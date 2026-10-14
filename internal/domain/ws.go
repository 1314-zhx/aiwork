/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package domain

type Message struct {
	ConversationId string `json:"conversationId"`

	RecvId string `json:"recvId"`
	SendId string `json:"sendId"`

	ChatType    int    `json:"chatType"`
	Content     string `json:"content"`
	ContentType int    `json:"contentType"`
}

type ApprovalEvent struct {
	ToUserId   string `json:"toUserId"`
	ApprovalId string `json:"approvalId"`
	ApprovalNo string `json:"approvalNo"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	EventType  string `json:"eventType"`
	CreateAt   int64  `json:"createAt"`
}
