/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package logic

import (
	"aiworkc/internal/domain"
	"aiworkc/internal/logic/chatinternal"
	"aiworkc/internal/model"
	"aiworkc/internal/svc"
	"aiworkc/pkg/audit"
	"aiworkc/pkg/httpx"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/memoryx"
	"aiworkc/pkg/langchain/outputrepair"
	"aiworkc/pkg/langchain/router"
	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/langchain/tokenretry"
	"aiworkc/pkg/langchain/voice"
	"aiworkc/pkg/token"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gitee.com/dn-jinmin/tlog"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/schema"
)

type Chat interface {
	PrivateChat(ctx context.Context, req *domain.Message) error
	GroupChat(ctx context.Context, req *domain.Message) (uids []string, err error)
	AIChat(ctx context.Context, req *domain.ChatReq) (resp *domain.ChatResp, err error)
	AIChatStream(ctx context.Context, req *domain.ChatReq, writeFn func(content string) error) error
	Confirm(ctx context.Context, req *domain.ChatConfirmReq) (resp *domain.ChatConfirmResp, err error)
	// SummarizeLog 会话页时间表单直达聊天总结，不走意图路由。
	SummarizeLog(ctx context.Context, req *domain.ChatLogSummaryReq) (*domain.ChatResp, error)
	File(ctx context.Context, req []*domain.FileResp) (err error)
	// Sync 离线消息同步
	Sync(ctx context.Context, req *domain.ChatSyncReq, userId string) (*domain.ChatSyncResp, error)
	// SyncAll 全量离线消息同步(连接时调用)
	SyncAll(ctx context.Context, lastSyncTime int64, userId string) (*domain.ChatSyncResp, error)
}

type chat struct {
	svc     *svc.ServiceContext
	memory  schema.Memory
	router  *router.Router
	voice   *voice.Voice
	chatLog *chatinternal.ChatLogHandle
}

func NewChat(svc *svc.ServiceContext) Chat {
	entityStore := memoryx.NewEntityStore()
	memory := memoryx.NewMemoryxWithStore(func() schema.Memory {
		m := memoryx.NewSummaryBuffer(svc.LLMs, 1550, memoryx.WithCallback(svc.Callbacks),
			memoryx.WithOutParser(memoryOutput),
			memoryx.WithModel("qwen-turbo"),
			memoryx.WithProtectedRecentTurns(1),
			memoryx.WithTieredMemoryLimits(700, 550, 300, 1550),
			memoryx.WithEntityStore(entityStore))

		m.InputKey = langchain.Input
		return m
	}, entityStore)

	handlers := []router.Handler{
		chatinternal.NewTodoHandle(svc),
		chatinternal.NewKnowledge(svc),
		chatinternal.NewApprovalHandle(svc),
		chatinternal.NewAnalysisHandle(svc, memory),
	}

	return &chat{
		svc:     svc,
		memory:  memory,
		voice:   voice.NewVoice(svc.OpenaiClient),
		chatLog: chatinternal.NewChatLogHandle(svc),
		router: router.NewRouter(svc.LLMs, handlers,
			router.WithMemory(memory),
			router.Withcallback(svc.Callbacks),
			router.WithEmptyHandler(chatinternal.NewDefaultHandler(svc)),
		),
	}
}

func (l *chat) PrivateChat(ctx context.Context, req *domain.Message) error {
	return l.chatlog(ctx, req)
}

func (l *chat) GroupChat(ctx context.Context, req *domain.Message) (uids []string, err error) {
	req.ConversationId = "all"
	if err := l.chatlog(ctx, req); err != nil {
		return nil, err
	}

	return nil, err
}

func (l *chat) chatlog(ctx context.Context, req *domain.Message) error {
	sendId := req.SendId

	chatlog := model.Chatlog{
		ConversationId: req.ConversationId,
		SendId:         sendId,
		RecvId:         req.RecvId,
		ChatType:       model.ChatType(req.ChatType),
		MsgContent:     req.Content,
		SendTime:       time.Now().Unix(),
	}

	if chatlog.ConversationId == "" {
		chatlog.ConversationId = GenerateUniqueID(sendId, req.RecvId)
	}

	return l.svc.ChatlogModel.Insert(ctx, &chatlog)
}

func (l *chat) AIChat(ctx context.Context, req *domain.ChatReq) (resp *domain.ChatResp, err error) {
	uid := token.GetUId(ctx)
	ctx = context.WithValue(ctx, langchain.ChatId, uid)
	l.seedFactEntities(ctx)

	if req.ChatType > 0 {
		switch domain.AiChatType(req.ChatType) {
		case domain.TokenRetry:
			return l.retryToken(ctx, req)
		case domain.AnalysisRetry:
			if err := l.resolvePrompt(ctx, req); err != nil {
				return nil, err
			}
			return l.retryAnalysis(ctx, req)
		default:
			return l.basicService(ctx, req)
		}
	}

	if err := l.resolvePrompt(ctx, req); err != nil {
		return nil, err
	}

	return l.aiService(ctx, req)
}

func (l *chat) resolvePrompt(ctx context.Context, req *domain.ChatReq) error {
	if len(req.Prompts) == 0 && len(req.Voice) > 0 {
		prompts, err := l.voice.Transcriptions(ctx, req.Voice, "")
		if err != nil {
			return err
		}
		req.Prompts = prompts
	}
	req.Prompts = httpx.LimitPrompt(req.Prompts)
	return nil
}

func (l *chat) seedFactEntities(ctx context.Context) {
	uid := token.GetUId(ctx)
	if uid == "" {
		return
	}
	mx, ok := l.memory.(*memoryx.Memoryx)
	if !ok || mx.GetEntityStore() == nil {
		return
	}
	fact := memoryx.FactProfile{UserID: uid}
	if l.svc != nil && l.svc.UserModel != nil {
		if u, err := l.svc.UserModel.FindOne(ctx, uid); err == nil && u != nil {
			fact.Name = u.Name
		}
	}
	if l.svc != nil && l.svc.DepartmentUserModel != nil {
		if du, err := l.svc.DepartmentUserModel.FindByUserId(ctx, uid); err == nil && du != nil && du.DepId != "" {
			fact.DeptID = du.DepId
			if l.svc.DepartmentModel != nil {
				if d, err := l.svc.DepartmentModel.FindOne(ctx, du.DepId); err == nil && d != nil {
					fact.DeptName = d.Name
				}
			}
		}
	}
	mx.GetEntityStore().SetFacts(uid, fact)
}

func (l *chat) withAIBudget(ctx context.Context, dailyNeed int) (context.Context, error) {
	uid := token.GetUId(ctx)
	if dailyNeed <= 0 {
		dailyNeed = tokencost.RouterDailyNeed
	}
	ctx = tokencost.WithDaily(ctx, l.svc.TokenDaily, uid)
	if err := tokencost.RequireDaily(ctx, tokencost.DailyEnterNeed(dailyNeed)); err != nil {
		return ctx, err
	}
	return tokencost.WithBudget(ctx, tokencost.NewBudget(tokencost.ChatTokenLimit)), nil
}

func (l *chat) finishAITurn(ctx context.Context) {
	if b := tokencost.FromContext(ctx); b != nil {
		l.svc.TokenDaily.Add(token.GetUId(ctx), b.Used())
	}
}

func (l *chat) preparePrompt(ctx context.Context, prompt string) error {
	if mx, ok := l.memory.(*memoryx.Memoryx); ok {
		return mx.EnsureTokenBudget(ctx, prompt, tokencost.MemoryTokenBudget)
	}
	return nil
}

func (l *chat) aiService(ctx context.Context, req *domain.ChatReq) (resp *domain.ChatResp, err error) {
	safePrompt := defend.Sanitize(req.Prompts, defend.DefaultSanitizeConfig())
	ctx, err = l.withAIBudget(ctx, tokencost.RouterDailyNeed)
	if err != nil {
		return nil, err
	}
	defer l.finishAITurn(ctx)
	if err := l.preparePrompt(ctx, safePrompt); err != nil {
		return nil, err
	}

	ctx = tokenretry.WithSession(ctx)
	v, err := chains.Call(ctx, l.router, map[string]any{
		langchain.Input: safePrompt,
		"relationId":    req.RelationId,
		"startTime":     req.StartTime,
		"endTime":       req.EndTime,
	}, chains.WithCallback(l.svc.Callbacks))
	if err != nil {
		fmt.Println("\n aiService chains ---- err : ", err.Error())
		if tokencost.IsDailyLimit(err) {
			return nil, err
		}
		if tokencost.IsRequestLimit(err) {
			return l.offerChatTokenRetry(ctx, req, token.GetUId(ctx)), nil
		}
		if miss, ok := router.AsNotHandles(err); ok {
			resp := l.offerAnalysisRetry(ctx, req, miss)
			redactChatResp(resp)
			return resp, nil
		}
		if res, ok := parseChatResp(err.Error()); ok {
			return &res, nil
		}
		if res, ok := outputrepair.Repair(ctx, l.svc.LLMs, safePrompt, err.Error()); ok {
			return &res, nil
		}
		return nil, err
	}
	fmt.Println("\n aiService chains ---- thoroughAnalysis ")

	resp, err = l.thoroughAnalysis(ctx, safePrompt, v)
	if err != nil {
		return nil, err
	}
	redactChatResp(resp)
	if resp != nil {
		audit.Event(ctx, "chat.complete", "uid", token.GetUId(ctx), "chatType", resp.ChatType)
	}

	// 保存 AI 回复到 MongoDB
	go l.saveAIResponse(context.Background(), req, resp)

	return resp, nil
}

// saveAIResponse 保存 AI 回复到 MongoDB
func (l *chat) saveAIResponse(ctx context.Context, req *domain.ChatReq, resp *domain.ChatResp) {
	uid := token.GetUId(ctx)
	if uid == "" {
		uid = fmt.Sprintf("%d", req.RelationId)
	}

	// 构建 AI 回复内容
	var aiContent string
	switch d := resp.Data.(type) {
	case string:
		aiContent = d
	default:
		b, _ := json.Marshal(d)
		aiContent = string(b)
	}

	// 构建消息列表：用户消息 + AI 回复
	now := time.Now().Unix()

	// 用户消息直接插入
	userMsg := &model.Chatlog{
		ConversationId: fmt.Sprintf("%d", req.RelationId),
		SendId:         uid,
		RecvId:         "ai",
		ChatType:       model.SingleChatType,
		MsgType:        model.MsgTypeUser,
		MsgContent:     req.Prompts,
		SendTime:       now - 1,
	}
	if err := l.svc.ChatlogModel.Insert(ctx, userMsg); err != nil {
		tlog.ErrorfCtx(ctx, "saveAIResponse", "insert user msg failed: %v", err)
	}

	// AI 回复使用 RequestId 幂等插入（避免重复保存）
	aiMsg := &model.Chatlog{
		RequestId:      resp.RequestId, // 由调用方传入
		ConversationId: fmt.Sprintf("%d", req.RelationId),
		SendId:         "ai",
		RecvId:         uid,
		ChatType:       model.SingleChatType,
		MsgType:        model.MsgTypeAI,
		MsgContent:     aiContent,
		SendTime:       now,
	}
	if err := l.svc.ChatlogModel.UpsertByRequestId(ctx, aiMsg); err != nil {
		tlog.ErrorfCtx(ctx, "saveAIResponse", "upsert ai msg failed: %v", err)
	}
}

func (l *chat) AIChatStream(ctx context.Context, req *domain.ChatReq, writeFn func(content string) error) error {
	uid := token.GetUId(ctx)
	ctx = context.WithValue(ctx, langchain.ChatId, uid)

	if err := l.resolvePrompt(ctx, req); err != nil {
		return err
	}

	safePrompt := defend.Sanitize(req.Prompts, defend.DefaultSanitizeConfig())
	ctx, err := l.withAIBudget(ctx, tokencost.RouterDailyNeed)
	if err != nil {
		return err
	}
	defer l.finishAITurn(ctx)
	if err := l.preparePrompt(ctx, safePrompt); err != nil {
		return err
	}

	// Buffer the full reply for Mongo. Chunks are not stored; disconnect is not resumed.
	var fullResponse strings.Builder
	requestId := fmt.Sprintf("%s-%d-%d", uid, req.RelationId, time.Now().UnixNano())

	input := map[string]any{
		langchain.Input: safePrompt,
		"history":       "",
		"relationId":    req.RelationId,
		"startTime":     req.StartTime,
		"endTime":       req.EndTime,
	}

	_, err = chains.Call(ctx, l.router, input,
		chains.WithCallback(l.svc.Callbacks),
		chains.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			if len(chunk) > 0 {
				fullResponse.Write(chunk)
				if err := writeFn(string(chunk)); err != nil {
					return err
				}
			}
			return nil
		}),
	)
	if err != nil {
		return err
	}

	go l.saveAIResponse(context.Background(), req, &domain.ChatResp{
		ChatType:  domain.DefaultHandler,
		Data:      fullResponse.String(),
		RequestId: requestId,
	})

	return nil
}

func (l *chat) thoroughAnalysis(ctx context.Context, userInput string, out map[string]any) (*domain.ChatResp, error) {
	var str string
	if _, ok := out[langchain.OutPut]; !ok {
		return &domain.ChatResp{
			ChatType: domain.DefaultHandler,
			Data:     str,
		}, nil
	}
	str = out[langchain.OutPut].(string)
	if res, ok := parseChatResp(str); ok {
		return &res, nil
	}
	if res, ok := outputrepair.Repair(ctx, l.svc.LLMs, userInput, str); ok {
		return &res, nil
	}

	return &domain.ChatResp{
		ChatType: domain.DefaultHandler,
		Data:     str,
	}, nil
}

func parseChatResp(text string) (domain.ChatResp, bool) {
	return outputrepair.ParseChatResp(text)
}

func redactChatResp(res *domain.ChatResp) {
	if res == nil {
		return
	}
	if res.ChatType == int(domain.ConfirmSQL) {
		return
	}
	switch d := res.Data.(type) {
	case string:
		res.Data = defend.RedactSecrets(d)
	case domain.ImgAndTextResp:
		d.Text = defend.RedactSecrets(d.Text)
		res.Data = d
	case domain.ChatLogSummaryData:
		d.Goal = defend.RedactSecrets(d.Goal)
		d.Text = defend.RedactSecrets(d.Text)
		d.TruncateNote = defend.RedactSecrets(d.TruncateNote)
		res.Data = d
	case *domain.ChatLogSummaryData:
		d.Goal = defend.RedactSecrets(d.Goal)
		d.Text = defend.RedactSecrets(d.Text)
		d.TruncateNote = defend.RedactSecrets(d.TruncateNote)
		res.Data = d
	case domain.AnalysisRetryResp:
		d.Reason = defend.RedactSecrets(d.Reason)
		d.Hint = defend.RedactSecrets(d.Hint)
		if d.Got != nil {
			got := make(map[string]string, len(d.Got))
			for k, v := range d.Got {
				got[k] = defend.RedactSecrets(v)
			}
			d.Got = got
		}
		res.Data = d
	}
}

func (l *chat) basicService(ctx context.Context, req *domain.ChatReq) (resp *domain.ChatResp, err error) {
	return nil, err
}

func (l *chat) Confirm(ctx context.Context, req *domain.ChatConfirmReq) (*domain.ChatConfirmResp, error) {
	if req == nil {
		return nil, errors.New("confirm request is required")
	}

	switch domain.AiChatType(req.ChatType) {
	case domain.ConfirmTodo:
		extracted, err := draftAs(req.Todo, req.Data)
		if err != nil {
			return nil, err
		}
		todo, err := buildTodoFromExtracted(extracted)
		if err != nil {
			return nil, err
		}
		idResp, err := NewTodo(l.svc).Create(ctx, todo)
		if err != nil {
			return nil, err
		}
		return idConfirmResp(idResp), nil
	case domain.ConfirmApproval:
		extracted, err := draftAs(req.Approval, req.Data)
		if err != nil {
			return nil, err
		}
		approval, err := buildApprovalFromExtracted(extracted)
		if err != nil {
			return nil, err
		}
		idResp, err := NewApproval(l.svc).Create(ctx, approval)
		if err != nil {
			return nil, err
		}
		return idConfirmResp(idResp), nil
	case domain.ConfirmSQL, domain.TokenRetry:
		return l.confirmSQL(ctx, req)
	default:
		return nil, errors.New("unsupported chatType")
	}
}

func (l *chat) SummarizeLog(ctx context.Context, req *domain.ChatLogSummaryReq) (*domain.ChatResp, error) {
	uid := token.GetUId(ctx)
	peer, cid, err := resolveChatLogSummary(uid, req)
	if err != nil {
		return nil, err
	}
	if _, err := l.svc.UserModel.FindOne(ctx, peer); err != nil {
		return nil, errors.New("会话对象不存在")
	}

	ctx = context.WithValue(ctx, langchain.ChatId, uid)
	l.seedFactEntities(ctx)
	ctx, err = l.withAIBudget(ctx, tokencost.AnalysisTokenLimit)
	if err != nil {
		return nil, err
	}
	defer l.finishAITurn(ctx)

	out, err := chains.Call(ctx, l.chatLog.Chains(), map[string]any{
		"conversationId": cid,
		"startTime":      req.StartTime,
		"endTime":        req.EndTime,
		"goal":           req.Goal,
	})
	if err != nil {
		return nil, err
	}
	resp, err := chatRespFromChainOut(out)
	if err != nil {
		return nil, err
	}
	redactChatResp(resp)
	if resp != nil {
		audit.Event(ctx, "chat.summary", "uid", uid, "peer", peer)
	}
	return resp, nil
}

func chatRespFromChainOut(out map[string]any) (*domain.ChatResp, error) {
	if out == nil {
		return nil, errors.New("总结结果为空")
	}
	switch v := out[langchain.OutPut].(type) {
	case *domain.ChatResp:
		return v, nil
	case domain.ChatResp:
		return &v, nil
	default:
		return nil, errors.New("总结结果格式异常")
	}
}

func resolveChatLogSummary(uid string, req *domain.ChatLogSummaryReq) (peer string, cid string, err error) {
	if req == nil {
		return "", "", errors.New("请填写总结表单")
	}
	if strings.TrimSpace(uid) == "" {
		return "", "", errors.New("请先登录后再总结会话")
	}
	peer = strings.TrimSpace(req.PeerUserId)
	if peer == "" {
		return "", "", errors.New("请选择要总结的会话对象")
	}
	if peer == uid {
		return "", "", errors.New("不能总结与自己的会话")
	}
	if _, err := chatinternal.NormalizeChatLogGoal(req.Goal); err != nil {
		return "", "", err
	}
	if err := chatinternal.ValidateChatLogWindow(req.StartTime, req.EndTime); err != nil {
		return "", "", err
	}
	return peer, GenerateUniqueID(uid, peer), nil
}

func (l *chat) confirmSQL(ctx context.Context, req *domain.ChatConfirmReq) (*domain.ChatConfirmResp, error) {
	draftID, err := sqlDraftID(req)
	if err != nil {
		return nil, err
	}
	uid := token.GetUId(ctx)
	if uid == "" {
		return nil, errors.New("请先登录后再执行数据分析")
	}
	action, err := sqlConfirmAction(req)
	if err != nil {
		return nil, err
	}

	ctx, err = l.withAIBudget(ctx, tokencost.AnalysisTokenLimit)
	if err != nil {
		return nil, err
	}
	defer l.finishAITurn(ctx)
	tokencost.RaiseLimit(ctx, tokencost.AnalysisTokenLimit)

	if action == sqlActionRetry && l.svc.TokenRetries != nil {
		if t, tErr := l.svc.TokenRetries.Get(uid, draftID); tErr == nil && t.Kind == tokenretry.KindSQLRevise {
			if strings.TrimSpace(req.Feedback) == "" {
				req.Feedback = t.Feedback
			}
			return l.rejectSQL(ctx, uid, draftID, req)
		}
	}
	if action == sqlActionReject {
		return l.rejectSQL(ctx, uid, draftID, req)
	}

	if err := tokencost.FromContext(ctx).CanAfford(tokencost.AgentFirstMaxTokens); err != nil {
		return l.offerSQLTokenRetry(uid, draftID, tokenretry.KindSQLExecute, "", nil), nil
	}
	draft, err := l.svc.SQLDrafts.Take(uid, draftID)
	if err != nil {
		return nil, err
	}
	resp, err := chatinternal.NewAnalysisHandle(l.svc, l.memory).ExecuteSQLDraft(ctx, draft)
	if err != nil {
		if l.svc.SQLDrafts != nil {
			l.svc.SQLDrafts.Restore(draft)
		}
		if tokencost.IsRequestLimit(err) {
			return l.offerSQLTokenRetry(uid, draftID, tokenretry.KindSQLExecute, "", &draft), nil
		}
		return nil, err
	}
	if l.svc.TokenRetries != nil {
		l.svc.TokenRetries.Delete(uid, draftID)
	}
	return &domain.ChatConfirmResp{
		ChatType: resp.ChatType,
		Data:     resp.Data,
	}, nil
}

func (l *chat) rejectSQL(ctx context.Context, uid, draftID string, req *domain.ChatConfirmReq) (*domain.ChatConfirmResp, error) {
	if err := tokencost.FromContext(ctx).CanAfford(tokencost.AgentFirstMaxTokens); err != nil {
		return l.offerSQLTokenRetry(uid, draftID, tokenretry.KindSQLRevise, sqlFeedback(req), nil), nil
	}
	draft, err := l.svc.SQLDrafts.Get(uid, draftID)
	if err != nil {
		return nil, err
	}
	resp, err := chatinternal.NewAnalysisHandle(l.svc, l.memory).ReviseSQLDraft(ctx, draft, sqlFeedback(req))
	if err != nil {
		if tokencost.IsRequestLimit(err) {
			return l.offerSQLTokenRetry(uid, draftID, tokenretry.KindSQLRevise, sqlFeedback(req), nil), nil
		}
		return nil, err
	}
	_, _ = l.svc.SQLDrafts.Take(uid, draftID)
	if l.svc.TokenRetries != nil {
		l.svc.TokenRetries.Delete(uid, draftID)
	}
	return &domain.ChatConfirmResp{
		ChatType: resp.ChatType,
		Data:     resp.Data,
	}, nil
}

func idConfirmResp(idResp *domain.IdResp) *domain.ChatConfirmResp {
	out := &domain.ChatConfirmResp{}
	if idResp != nil {
		out.Id = idResp.Id
	}
	return out
}

func memoryOutput(ctx context.Context, v string) string {
	// 如果不是JSON格式，尝试从 "Final Answer: {...}" 中提取
	if !strings.HasPrefix(strings.TrimSpace(v), "{") {
		re := regexp.MustCompile(`Final Answer:\s*(\{[\s\S]*\})`)
		matches := re.FindStringSubmatch(v)
		if len(matches) > 1 {
			v = matches[1]
		}
	}

	var res domain.ChatResp
	if err := json.Unmarshal([]byte(v), &res); err != nil {
		tlog.ErrorfCtx(ctx, "memoryOutput", "v %s, err %s", v, err.Error())
		return v
	}

	tlog.InfoCtx(ctx, "memoryOutput", v)

	switch res.Data.(type) {
	case string:
		return res.Data.(string)
	default:
		return v
	}
}

func (l *chat) File(ctx context.Context, files []*domain.FileResp) (err error) {
	uid := token.GetUId(ctx)
	ctx = context.WithValue(ctx, langchain.ChatId, uid)

	data := make([]*domain.ChatFile, 0, len(files))
	for _, file := range files {
		data = append(data, &domain.ChatFile{
			Path: file.File,
			Name: file.Filename,
			Time: time.Now(),
		})
	}

	b, err := json.Marshal(data)
	if err != nil {
		return err
	}

	err = l.memory.SaveContext(ctx, map[string]any{
		langchain.Input: string(b),
	}, map[string]any{
		langchain.Input: "uploaded files",
	})

	// test
	memoryContent, err := l.memory.LoadMemoryVariables(ctx, map[string]any{})
	if err != nil {
		return err
	}
	fmt.Println(memoryContent)

	return
}

// GenerateUniqueID 根据传递的两个字符串 ID 生成唯一的 ID
func GenerateUniqueID(id1, id2 string) string {
	// 将两个 ID 放入切片中
	ids := []string{id1, id2}

	// 对 IDs 切片进行排序
	sort.Strings(ids)

	// 将排序后的 ID 组合起来
	combined := ids[0] + ids[1]

	// 创建 SHA-256 哈希对象
	hasher := sha256.New()

	// 写入合并后的字符串
	hasher.Write([]byte(combined))

	// 计算哈希值
	hash := hasher.Sum(nil)

	// 返回哈希值的十六进制字符串表示
	return base64.RawStdEncoding.EncodeToString(hash)[:22] // 可以选择更短的长度
}

// Sync 离线消息同步，按会话拉取指定时间之后的消息
func (l *chat) Sync(ctx context.Context, req *domain.ChatSyncReq, userId string) (*domain.ChatSyncResp, error) {
	var messages []*domain.ChatMessage

	// 如果没有指定会话ID，则返回用户所有会话的离线消息
	if req.ConversationId == "" {
		return l.SyncAll(ctx, req.LastSyncTime, userId)
	}

	// 按指定会话拉取
	logs, err := l.svc.ChatlogModel.Sync(ctx, req.ConversationId, req.LastSyncTime, req.Limit)
	if err != nil {
		return nil, err
	}

	for _, log := range logs {
		messages = append(messages, &domain.ChatMessage{
			Id:             log.ID.Hex(),
			ConversationId: log.ConversationId,
			SendId:         log.SendId,
			RecvId:         log.RecvId,
			ChatType:       int(log.ChatType),
			MsgType:        int(log.MsgType),
			Content:        log.MsgContent,
			SendTime:       log.SendTime,
		})
	}

	return &domain.ChatSyncResp{
		Messages:   messages,
		ServerTime: time.Now().Unix(),
		HasMore:    len(logs) >= req.Limit,
	}, nil
}

// SyncAll 全量离线消息同步，获取用户所有会话的离线消息
func (l *chat) SyncAll(ctx context.Context, lastSyncTime int64, userId string) (*domain.ChatSyncResp, error) {
	// 获取用户参与的所有会话
	conversationIds, err := l.svc.ChatlogModel.GetUserConversations(ctx, userId)
	if err != nil {
		return nil, err
	}

	var allMessages []*domain.ChatMessage
	const defaultLimit = 100

	for _, cid := range conversationIds {
		logs, err := l.svc.ChatlogModel.Sync(ctx, cid, lastSyncTime, defaultLimit)
		if err != nil {
			continue // 单个会话失败不影响其他会话
		}

		for _, log := range logs {
			allMessages = append(allMessages, &domain.ChatMessage{
				Id:             log.ID.Hex(),
				ConversationId: log.ConversationId,
				SendId:         log.SendId,
				RecvId:         log.RecvId,
				ChatType:       int(log.ChatType),
				Content:        log.MsgContent,
				SendTime:       log.SendTime,
			})
		}
	}

	// 按发送时间排序
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].SendTime < allMessages[j].SendTime
	})

	return &domain.ChatSyncResp{
		Messages:   allMessages,
		ServerTime: time.Now().Unix(),
		HasMore:    len(allMessages) >= len(conversationIds)*defaultLimit,
	}, nil
}
