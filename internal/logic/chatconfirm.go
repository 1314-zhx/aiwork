package logic

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aiworkc/internal/domain"
	leavetime "aiworkc/internal/logic/chatinternal/toolx/approval"
	"aiworkc/internal/model"
	"aiworkc/pkg/httpx"
	"aiworkc/pkg/langchain/defend"
)

func draftAs[T any](direct *T, data any) (*T, error) {
	if direct != nil {
		return direct, nil
	}
	if data == nil {
		return nil, errors.New("draft is required")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// buildTodoFromExtracted 用 LLM 抽出的业务字段组装待办结构体，不信任 id/创建人等身份字段。
func buildTodoFromExtracted(src *domain.Todo) (*domain.Todo, error) {
	if src == nil {
		return nil, errors.New("todo draft is required")
	}
	title := strings.TrimSpace(src.Title)
	if title == "" {
		return nil, errors.New("待办标题解析失败")
	}
	return &domain.Todo{
		Title:      title,
		DeadlineAt: src.DeadlineAt,
		Desc:       src.Desc,
		ExecuteIds: src.ExecuteIds,
	}, nil
}

// buildApprovalFromExtracted 用 LLM 抽出的业务字段组装审批结构体，审批人/单号等由 Create 写入。
func buildApprovalFromExtracted(src *domain.Approval) (*domain.Approval, error) {
	if src == nil {
		return nil, errors.New("approval draft is required")
	}

	out := &domain.Approval{Type: src.Type}
	switch model.ApprovalType(src.Type) {
	case model.LeaveApproval:
		if src.Leave == nil {
			return nil, errors.New("请假信息解析失败")
		}
		leave := *src.Leave
		leavetime.ApplyDefaultLeaveTimes(&leave, time.Now())
		if leave.EndTime <= leave.StartTime {
			return nil, errors.New("请假结束时间必须晚于开始时间")
		}
		out.Leave = &domain.Leave{
			Type:      leave.Type,
			StartTime: leave.StartTime,
			EndTime:   leave.EndTime,
			Reason:    leave.Reason,
			TimeType:  leave.TimeType,
		}
	case model.MakeCardApproval:
		if src.MakeCard == nil || src.MakeCard.Day == 0 {
			return nil, errors.New("补卡信息解析失败")
		}
		out.MakeCard = &domain.MakeCard{
			Date:      src.MakeCard.Date,
			Reason:    src.MakeCard.Reason,
			Day:       src.MakeCard.Day,
			CheckType: src.MakeCard.CheckType,
		}
	case model.GoOutApproval:
		if src.GoOut == nil || src.GoOut.StartTime <= 0 || src.GoOut.EndTime <= 0 {
			return nil, errors.New("外出时间解析失败")
		}
		if src.GoOut.EndTime <= src.GoOut.StartTime {
			return nil, errors.New("外出结束时间必须晚于开始时间")
		}
		out.GoOut = &domain.GoOut{
			StartTime: src.GoOut.StartTime,
			EndTime:   src.GoOut.EndTime,
			Reason:    src.GoOut.Reason,
		}
	default:
		return nil, errors.New("不支持的审批类型")
	}
	return out, nil
}

func sqlDraftID(req *domain.ChatConfirmReq) (string, error) {
	if req == nil {
		return "", errors.New("draftId is required")
	}
	if id := strings.TrimSpace(req.DraftId); id != "" {
		return id, nil
	}
	if req.Data == nil {
		return "", errors.New("draftId is required")
	}
	b, err := json.Marshal(req.Data)
	if err != nil {
		return "", err
	}
	var payload struct {
		DraftId string `json:"draftId"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return "", errors.New("draftId is required")
	}
	if id := strings.TrimSpace(payload.DraftId); id != "" {
		return id, nil
	}
	return "", errors.New("draftId is required")
}

const (
	sqlActionConfirm = "confirm"
	sqlActionReject  = "reject"
	sqlActionRetry   = "retry"
)

func sqlConfirmAction(req *domain.ChatConfirmReq) (string, error) {
	if req == nil {
		return sqlActionConfirm, nil
	}
	if domain.AiChatType(req.ChatType) == domain.TokenRetry {
		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "", sqlActionRetry, sqlActionConfirm, "ok", "accept":
			return sqlActionRetry, nil
		case sqlActionReject, "deny":
			return sqlActionReject, nil
		default:
			return "", errors.New("token 不足后只能重试或拒绝")
		}
	}
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "", sqlActionConfirm, "ok", "accept":
		return sqlActionConfirm, nil
	case sqlActionReject, "deny":
		return sqlActionReject, nil
	case sqlActionRetry:
		return sqlActionRetry, nil
	default:
		return "", errors.New("SQL 只能确认、拒绝或重试，不能修改")
	}
}

func sqlFeedback(req *domain.ChatConfirmReq) string {
	if req == nil {
		return ""
	}
	return defend.Sanitize(httpx.LimitPrompt(req.Feedback), defend.DefaultSanitizeConfig())
}
