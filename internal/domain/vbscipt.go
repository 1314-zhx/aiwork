/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package domain

import "time"

type AiChatType int

const (
	DefaultHandler = iota
	TodoFind
	TodoAdd

	ApprovalFind

	ChatLog

	ImgAndText // 图片+文本
	File       //

	ConfirmTodo     // 待办草稿，等待用户点击提交后落库
	ConfirmApproval // 审批草稿，等待用户点击提交后落库
	ConfirmSQL      // SQL 查询草稿，等待用户确认后才执行
	TokenRetry      // 本次请求 token 不足，可带 draftId 重试
	AnalysisRetry   // 二级分析缺意图或必填字段，按 eventId 补全后恢复
)

type ChatFile struct {
	Path string
	Name string
	Time time.Time
}

type ImgAndTextResp struct {
	Text string `json:"text"`
	Url  string `json:"url"`
}

// SQLDraft is shown to the user before analysis.db runs Query.
// Confirm must send draftId only; the SQL stays on the server.
type SQLDraft struct {
	DraftId string   `json:"draftId"`
	Summary string   `json:"summary"`
	Tables  []string `json:"tables,omitempty"`
	Columns []string `json:"columns,omitempty"`
	SQL     string   `json:"sql,omitempty"` // 仅展示，确认/拒绝时回传会被忽略
	Limit   int      `json:"limit,omitempty"`
}

// TokenRetryResp asks the client whether to retry the same server ticket.
type TokenRetryResp struct {
	DraftId  string `json:"draftId"`
	Summary  string `json:"summary"`
	CanRetry bool   `json:"canRetry"`
}

// AnalysisRetryResp asks the client to fill missing analysis fields, then
// resume with the same eventId. DraftId equals EventId for the existing retry field.
type AnalysisRetryResp struct {
	EventId     string            `json:"eventId"`
	DraftId     string            `json:"draftId"`
	Reason      string            `json:"reason"`
	Hint        string            `json:"hint"`
	Need        []string          `json:"need,omitempty"`
	Accept      []string          `json:"accept,omitempty"`
	Destination string            `json:"destination,omitempty"`
	Got         map[string]string `json:"got,omitempty"`
	CanRetry    bool              `json:"canRetry"`
}

type ChatFileResp struct {
	Url string `json:"url"`
}
