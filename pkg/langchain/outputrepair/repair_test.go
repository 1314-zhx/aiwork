package outputrepair

import (
	"context"
	"encoding/json"
	"testing"

	"aiworkc/internal/domain"

	"github.com/tmc/langchaingo/llms"
)

func TestParseChatRespMarkdownFence(t *testing.T) {
	raw := "```json\n{\"chatType\":0,\"data\":\"明天请假\"}\n```"
	res, ok := ParseChatResp(raw)
	if !ok {
		t.Fatal("expected markdown JSON to parse")
	}
	if res.ChatType != 0 {
		t.Fatalf("chatType=%d", res.ChatType)
	}
	if res.Data != "明天请假" {
		t.Fatalf("data=%v", res.Data)
	}
}

func TestParseChatRespEscapedObject(t *testing.T) {
	raw := `Final Answer: {\"chatType\":0,\"data\":\"已完成\"}`
	res, ok := ParseChatResp(raw)
	if !ok {
		t.Fatal("expected escaped JSON to parse")
	}
	if res.Data != "已完成" {
		t.Fatalf("data=%v", res.Data)
	}
}

func TestParseChatRespRejectsEmptyData(t *testing.T) {
	if _, ok := ParseChatResp(`{"chatType":0,"data":""}`); ok {
		t.Fatal("empty data is not a usable answer")
	}
}

func TestParseChatRespConfirmSQLDraft(t *testing.T) {
	raw, _ := json.Marshal(domain.ChatResp{
		ChatType: int(domain.ConfirmSQL),
		Data:     domain.SQLDraft{DraftId: "d1", Summary: "拟执行只读查询"},
	})
	res, ok := ParseChatResp(string(raw))
	if !ok {
		t.Fatal("expected SQL confirm draft")
	}
	if res.ChatType != int(domain.ConfirmSQL) {
		t.Fatalf("chatType=%d", res.ChatType)
	}
}

func TestParseChatRespConfirmDraft(t *testing.T) {
	raw, _ := json.Marshal(domain.ChatResp{
		ChatType: int(domain.ConfirmTodo),
		Data:     map[string]any{"title": "写周报"},
	})
	res, ok := ParseChatResp("Final Answer: " + string(raw))
	if !ok {
		t.Fatal("expected confirm draft")
	}
	if res.ChatType != int(domain.ConfirmTodo) {
		t.Fatalf("chatType=%d", res.ChatType)
	}
}

func TestRepairSkipsEmptyAndNilModel(t *testing.T) {
	if _, ok := Repair(nil, nil, "请假", ""); ok {
		t.Fatal("empty output must not repair")
	}
	if _, ok := Repair(nil, nil, "请假", "随便说一句"); ok {
		t.Fatal("nil model must not repair")
	}
}

type stubModel struct {
	answer string
}

func (s stubModel) GenerateContent(_ context.Context, _ []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: s.answer}}}, nil
}

func (s stubModel) Call(_ context.Context, _ string, _ ...llms.CallOption) (string, error) {
	return s.answer, nil
}

func TestRepairExtractsPlainAnswer(t *testing.T) {
	model := stubModel{answer: `{"chatType":0,"data":"明天可以请假"}`}
	res, ok := Repair(context.Background(), model, "明天请假吗", "当然可以，明天请一天假就行。")
	if !ok {
		t.Fatal("expected repair to rebuild JSON")
	}
	if res.Data != "明天可以请假" {
		t.Fatalf("data=%v", res.Data)
	}
}
