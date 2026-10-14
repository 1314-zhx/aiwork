package logic

import (
	"context"
	"errors"
	"strings"

	"aiworkc/internal/domain"
	"aiworkc/internal/logic/chatinternal"
	"aiworkc/internal/logic/chatinternal/analysis"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/router"
	"aiworkc/pkg/langchain/sqlguard"
	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/langchain/tokenretry"
	"aiworkc/pkg/token"

	"github.com/segmentio/ksuid"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/schema"
)

const tokenRetrySummary = "本次请求 token 不足，是否重试？"

func tokenRetryChat(draftID string) *domain.ChatResp {
	return &domain.ChatResp{
		ChatType: int(domain.TokenRetry),
		Data: domain.TokenRetryResp{
			DraftId:  draftID,
			Summary:  tokenRetrySummary,
			CanRetry: true,
		},
	}
}

func tokenRetryConfirm(draftID string) *domain.ChatConfirmResp {
	resp := tokenRetryChat(draftID)
	return &domain.ChatConfirmResp{ChatType: resp.ChatType, Data: resp.Data}
}

func (l *chat) offerChatTokenRetry(ctx context.Context, req *domain.ChatReq, uid string) *domain.ChatResp {
	if l.svc == nil || l.svc.TokenRetries == nil || uid == "" || req == nil {
		return tokenRetryChat("")
	}
	sess := tokenretry.FromSession(ctx)
	kind := tokenretry.KindChat
	handler := ""
	var steps []schema.AgentStep
	inputs := map[string]string{}
	if sess != nil && len(sess.Steps) > 0 && sess.Handler != "" {
		kind = tokenretry.KindReact
		handler = sess.Handler
		steps = append([]schema.AgentStep(nil), sess.Steps...)
		inputs = sess.Inputs
	}
	id := strings.TrimSpace(req.DraftId)
	if id == "" {
		id = ksuid.New().String()
	}
	ticket := l.svc.TokenRetries.Put(tokenretry.Ticket{
		ID:          id,
		UserID:      uid,
		Kind:        kind,
		Handler:     handler,
		Steps:       steps,
		AgentInputs: inputs,
		Chat: tokenretry.ChatSnapshot{
			Prompts:    req.Prompts,
			RelationId: req.RelationId,
			StartTime:  req.StartTime,
			EndTime:    req.EndTime,
		},
	})
	return tokenRetryChat(ticket.ID)
}

func (l *chat) offerSQLTokenRetry(uid, draftID string, kind tokenretry.Kind, feedback string, draft *sqlguard.Draft) *domain.ChatConfirmResp {
	if draft != nil && l.svc != nil && l.svc.SQLDrafts != nil {
		l.svc.SQLDrafts.Put(*draft)
	}
	if l.svc != nil && l.svc.TokenRetries != nil && uid != "" && draftID != "" {
		l.svc.TokenRetries.Put(tokenretry.Ticket{
			ID:         draftID,
			UserID:     uid,
			Kind:       kind,
			SQLDraftID: draftID,
			Feedback:   feedback,
		})
	}
	return tokenRetryConfirm(draftID)
}

func (l *chat) retryToken(ctx context.Context, req *domain.ChatReq) (*domain.ChatResp, error) {
	uid := token.GetUId(ctx)
	if uid == "" {
		return nil, errors.New("请先登录后再重试")
	}
	draftID := ""
	if req != nil {
		draftID = strings.TrimSpace(req.DraftId)
	}
	if draftID == "" {
		return nil, errors.New("draftId is required")
	}
	if l.svc == nil || l.svc.TokenRetries == nil {
		return nil, errors.New("retry store is not configured")
	}
	ticket, err := l.svc.TokenRetries.Get(uid, draftID)
	if err != nil {
		if l.svc.SQLDrafts != nil {
			if _, sqlErr := l.svc.SQLDrafts.Get(uid, draftID); sqlErr == nil {
				confirm, cErr := l.confirmSQL(ctx, &domain.ChatConfirmReq{
					ChatType: int(domain.ConfirmSQL),
					DraftId:  draftID,
					Action:   sqlActionRetry,
				})
				if cErr != nil {
					return nil, cErr
				}
				return &domain.ChatResp{ChatType: confirm.ChatType, Data: confirm.Data}, nil
			}
		}
		return nil, err
	}

	switch ticket.Kind {
	case tokenretry.KindSQLExecute, tokenretry.KindSQLRevise:
		confirm, cErr := l.confirmSQL(ctx, &domain.ChatConfirmReq{
			ChatType: int(domain.ConfirmSQL),
			DraftId:  draftID,
			Action:   sqlActionRetry,
			Feedback: ticket.Feedback,
		})
		if cErr != nil {
			return nil, cErr
		}
		return &domain.ChatResp{ChatType: confirm.ChatType, Data: confirm.Data}, nil
	case tokenretry.KindReact:
		return l.retryReact(ctx, req, uid, ticket)
	case tokenretry.KindAnalysisFill:
		return l.retryAnalysis(ctx, req)
	default:
		replay := &domain.ChatReq{
			Prompts:    ticket.Chat.Prompts,
			RelationId: ticket.Chat.RelationId,
			StartTime:  ticket.Chat.StartTime,
			EndTime:    ticket.Chat.EndTime,
			DraftId:    ticket.ID,
		}
		resp, err := l.aiService(ctx, replay)
		if err != nil {
			return nil, err
		}
		if !keepRetryTicket(resp) {
			l.svc.TokenRetries.Delete(uid, draftID)
		}
		return resp, nil
	}
}

func (l *chat) retryReact(ctx context.Context, req *domain.ChatReq, uid string, ticket tokenretry.Ticket) (*domain.ChatResp, error) {
	h := l.router.Handler(ticket.Handler)
	if h == nil {
		replay := &domain.ChatReq{
			Prompts:    ticket.Chat.Prompts,
			RelationId: ticket.Chat.RelationId,
			StartTime:  ticket.Chat.StartTime,
			EndTime:    ticket.Chat.EndTime,
			DraftId:    ticket.ID,
		}
		return l.aiService(ctx, replay)
	}
	ctx, err := l.withAIBudget(ctx, tokencost.ReActTokenLimit)
	if err != nil {
		return nil, err
	}
	defer l.finishAITurn(ctx)
	ctx = tokenretry.WithResume(ctx, ticket.Handler, ticket.Steps, ticket.AgentInputs)
	inputs := map[string]any{}
	for k, v := range ticket.AgentInputs {
		inputs[k] = v
	}
	v, err := chains.Call(ctx, h.Chains(), inputs)
	if err != nil {
		if tokencost.IsDailyLimit(err) {
			return nil, err
		}
		if tokencost.IsRequestLimit(err) {
			return l.offerChatTokenRetry(ctx, req, uid), nil
		}
		return nil, err
	}
	resp, err := l.thoroughAnalysis(ctx, ticket.Chat.Prompts, v)
	if err != nil {
		return nil, err
	}
	if !keepRetryTicket(resp) {
		l.svc.TokenRetries.Delete(uid, ticket.ID)
	}
	return resp, nil
}

func keepRetryTicket(resp *domain.ChatResp) bool {
	if resp == nil {
		return false
	}
	switch domain.AiChatType(resp.ChatType) {
	case domain.TokenRetry, domain.AnalysisRetry:
		return true
	default:
		return false
	}
}

func analysisRetryChat(data domain.AnalysisRetryResp) *domain.ChatResp {
	return &domain.ChatResp{
		ChatType: int(domain.AnalysisRetry),
		Data:     data,
	}
}

func (l *chat) offerAnalysisRetry(ctx context.Context, req *domain.ChatReq, miss *router.NotHandlesError) *domain.ChatResp {
	if miss == nil {
		miss = router.NewUnknownIntent([]string{"db", "file"}, "", "")
	}
	uid := token.GetUId(ctx)
	id := ""
	prompts := ""
	relationID := 0
	var start, end int64
	if req != nil {
		id = strings.TrimSpace(req.DraftId)
		prompts = req.Prompts
		relationID = req.RelationId
		start = req.StartTime
		end = req.EndTime
	}
	if id == "" {
		id = ksuid.New().String()
	}
	nextInputs := strings.TrimSpace(miss.NextInputs)
	if nextInputs == "" {
		nextInputs = strings.TrimSpace(prompts)
	}
	dest := strings.TrimSpace(miss.Destination)
	partial := miss.Partial
	if l.svc != nil && l.svc.TokenRetries != nil && uid != "" {
		if prev, err := l.svc.TokenRetries.Get(uid, id); err == nil && prev.Kind == tokenretry.KindAnalysisFill {
			partial = analysis.MergePartial(prev.Partial, partial)
			if dest == "" {
				dest = prev.Destination
			}
			if nextInputs == "" {
				nextInputs = prev.NextInputs
			}
			if prompts == "" {
				prompts = prev.Chat.Prompts
				relationID = prev.Chat.RelationId
				start = prev.Chat.StartTime
				end = prev.Chat.EndTime
			}
		}
		dest = analysis.InferDest(dest, "", partial)
		l.svc.TokenRetries.Put(tokenretry.Ticket{
			ID:          id,
			UserID:      uid,
			Kind:        tokenretry.KindAnalysisFill,
			Destination: dest,
			NextInputs:  nextInputs,
			Partial:     analysis.MergePartial(partial, nil),
			Need:        append([]string(nil), miss.Need...),
			Accept:      append([]string(nil), miss.Accept...),
			Reason:      miss.Reason,
			Hint:        miss.Hint,
			Chat: tokenretry.ChatSnapshot{
				Prompts:    prompts,
				RelationId: relationID,
				StartTime:  start,
				EndTime:    end,
			},
		})
	}
	return analysisRetryChat(domain.AnalysisRetryResp{
		EventId:     id,
		DraftId:     id,
		Reason:      miss.Reason,
		Hint:        miss.Hint,
		Need:        miss.Need,
		Accept:      miss.Accept,
		Destination: dest,
		Got:         partial,
		CanRetry:    true,
	})
}

func (l *chat) retryAnalysis(ctx context.Context, req *domain.ChatReq) (*domain.ChatResp, error) {
	uid := token.GetUId(ctx)
	if uid == "" {
		return nil, errors.New("请先登录后再重试")
	}
	draftID := ""
	supplement := ""
	if req != nil {
		draftID = strings.TrimSpace(req.DraftId)
		supplement = req.Prompts
	}
	if draftID == "" {
		return nil, errors.New("eventId is required")
	}
	if l.svc == nil || l.svc.TokenRetries == nil {
		return nil, errors.New("retry store is not configured")
	}
	ticket, err := l.svc.TokenRetries.Get(uid, draftID)
	if err != nil {
		return nil, err
	}
	if ticket.Kind != tokenretry.KindAnalysisFill {
		return nil, errors.New("eventId is not an analysis retry ticket")
	}

	base := ticket.NextInputs
	if strings.TrimSpace(base) == "" {
		base = ticket.Chat.Prompts
	}
	merged := analysis.MergeResumeInput(base, supplement, ticket.Partial)
	dest := analysis.InferDest(ticket.Destination, supplement, ticket.Partial)
	if dest == "" {
		dest = analysis.InferDest(ticket.Destination, merged, ticket.Partial)
	}
	if dest == "" {
		miss := router.NewUnknownIntent([]string{"db", "file"}, "", merged)
		miss.Partial = analysis.MergePartial(ticket.Partial, miss.Partial)
		if req == nil {
			req = &domain.ChatReq{}
		}
		req.DraftId = ticket.ID
		if req.Prompts == "" {
			req.Prompts = ticket.Chat.Prompts
			req.RelationId = ticket.Chat.RelationId
			req.StartTime = ticket.Chat.StartTime
			req.EndTime = ticket.Chat.EndTime
		}
		return l.offerAnalysisRetry(ctx, req, miss), nil
	}

	h, _ := l.router.Handler("analysis").(*chatinternal.AnalysisHandle)
	if h == nil {
		return nil, errors.New("analysis handler is not configured")
	}

	ctx, err = l.withAIBudget(ctx, tokencost.AnalysisTokenLimit)
	if err != nil {
		return nil, err
	}
	defer l.finishAITurn(ctx)

	inputs := map[string]any{
		langchain.Input: defend.Sanitize(merged, defend.DefaultSanitizeConfig()),
		"history":       "",
		"relationId":    ticket.Chat.RelationId,
		"startTime":     ticket.Chat.StartTime,
		"endTime":       ticket.Chat.EndTime,
	}
	v, err := h.Resume(ctx, dest, inputs)
	if err != nil {
		if tokencost.IsDailyLimit(err) {
			return nil, err
		}
		if tokencost.IsRequestLimit(err) {
			return l.offerChatTokenRetry(ctx, req, uid), nil
		}
		if miss, ok := router.AsNotHandles(err); ok {
			if req == nil {
				req = &domain.ChatReq{}
			}
			req.DraftId = ticket.ID
			if req.Prompts == "" {
				req.Prompts = ticket.Chat.Prompts
			}
			req.RelationId = ticket.Chat.RelationId
			req.StartTime = ticket.Chat.StartTime
			req.EndTime = ticket.Chat.EndTime
			return l.offerAnalysisRetry(ctx, req, miss), nil
		}
		return nil, err
	}
	resp, err := l.thoroughAnalysis(ctx, merged, v)
	if err != nil {
		return nil, err
	}
	if !keepRetryTicket(resp) {
		l.svc.TokenRetries.Delete(uid, ticket.ID)
	}
	return resp, nil
}
