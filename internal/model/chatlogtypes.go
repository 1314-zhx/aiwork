package model

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ChatType int

const (
	GroupChatType ChatType = iota + 1
	SingleChatType
)

// MsgType 消息发送方类型
type MsgType int

const (
	MsgTypeUser MsgType = iota + 1 // 用户发送
	MsgTypeAI                      // AI 回复
)

type Chatlog struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`

	RequestId     string   `bson:"requestId,omitempty"`     // 请求ID，用于 AI 回复的幂等保存
	ConversationId string   `bson:"conversationId"`
	SendId         string   `bson:"sendId"`      // 发送者ID（用户ID 或 "ai"）
	RecvId         string   `bson:"recvId"`      // 接收者ID
	ChatType       ChatType `bson:"chatType"`
	MsgType        MsgType  `bson:"msgType"`     // 消息类型：用户/AI
	MsgContent     string   `bson:"msgContent"`
	SendTime       int64    `bson:"sendTime"`

	UpdateAt int64 `bson:"updateAt,omitempty" json:"updateAt,omitempty"`
	CreateAt int64 `bson:"createAt,omitempty" json:"createAt,omitempty"`
}
