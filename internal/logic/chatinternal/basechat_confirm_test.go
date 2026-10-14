package chatinternal

import (
	"encoding/json"
	"testing"

	"aiworkc/internal/domain"

	"github.com/tmc/langchaingo/schema"
)

func TestExtractConfirmDraftPrefersToolObservation(t *testing.T) {
	draft, err := json.Marshal(domain.ChatResp{
		ChatType: int(domain.ConfirmTodo),
		Data: map[string]any{
			"title": "写周报",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := extractConfirmDraft(map[string]any{
		"output": "已帮你创建待办",
		"intermediateSteps": []schema.AgentStep{
			{
				Observation: "Final Answer: " + string(draft) + "\n单据尚未写入数据库。",
			},
		},
	})
	if got == "" {
		t.Fatal("expected confirm draft from intermediateSteps")
	}

	var res domain.ChatResp
	if err := json.Unmarshal([]byte(got), &res); err != nil {
		t.Fatal(err)
	}
	if res.ChatType != int(domain.ConfirmTodo) {
		t.Fatalf("chatType = %d, want %d", res.ChatType, domain.ConfirmTodo)
	}
}
