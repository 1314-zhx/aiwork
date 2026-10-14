package agentx

import (
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/schema"
)

func TestGateAllowsFirstToolCall(t *testing.T) {
	allow, obs := gateToolCall(nil, schema.AgentAction{Tool: "todo_find", ToolInput: `{}`})
	if !allow || obs != "" {
		t.Fatalf("first call must run, got allow=%v obs=%q", allow, obs)
	}
}

func TestGateRefusesAfterSuccessAndAsksFinish(t *testing.T) {
	steps := []schema.AgentStep{{
		Action:      schema.AgentAction{Tool: "todo_find", ToolInput: `{"id":"1"}`},
		Observation: `{"chatType":1,"data":[]}`,
	}}
	allow, obs := gateToolCall(steps, schema.AgentAction{Tool: "todo_add", ToolInput: `{"title":"x"}`})
	if allow {
		t.Fatal("must refuse any tool after success")
	}
	if !IsGateObservation(obs) || !strings.Contains(obs, "最终答复") || !strings.Contains(obs, "上次结果") {
		t.Fatalf("obs=%q", obs)
	}
}

func TestGateAllowsFailedRetryWithDifferentInput(t *testing.T) {
	steps := []schema.AgentStep{{
		Action:      schema.AgentAction{Tool: "todo_add", ToolInput: `{"title":""}`},
		Observation: toolErrorObservation(errors.New("todo title parse failed")),
	}}
	allow, obs := gateToolCall(steps, schema.AgentAction{Tool: "todo_add", ToolInput: `{"title":"写周报"}`})
	if !allow || obs != "" {
		t.Fatalf("different input after failure must run, allow=%v obs=%q", allow, obs)
	}
}

func TestGateRefusesFailedRetryWithSameInput(t *testing.T) {
	steps := []schema.AgentStep{{
		Action:      schema.AgentAction{Tool: "todo_find", ToolInput: `{"id":"1","count":10}`},
		Observation: toolErrorPrefix + " timeout",
	}}
	allow, obs := gateToolCall(steps, schema.AgentAction{Tool: "TODO_FIND", ToolInput: `{ "count": 10, "id": "1" }`})
	if allow {
		t.Fatal("same input after failure must be refused")
	}
	if !IsGateObservation(obs) || !strings.Contains(obs, "必须与上次不同") {
		t.Fatalf("obs=%q", obs)
	}
}

func TestGateSkipsParserOnlySteps(t *testing.T) {
	steps := []schema.AgentStep{
		{Action: schema.AgentAction{Tool: "todo_find", ToolInput: `{}`}, Observation: "found"},
		{Observation: "格式错误"},
	}
	allow, _ := gateToolCall(steps, schema.AgentAction{Tool: "todo_find", ToolInput: `{"q":"2"}`})
	if allow {
		t.Fatal("parser-only step must not hide the successful tool result")
	}
}

func TestGateStillRefusesAfterFinishNudge(t *testing.T) {
	steps := []schema.AgentStep{
		{Action: schema.AgentAction{Tool: "todo_find", ToolInput: `{}`}, Observation: "found"},
		{Action: schema.AgentAction{Tool: "todo_add", ToolInput: `{"title":"x"}`}, Observation: gatePrefix + " 请 finish"},
	}
	allow, _ := gateToolCall(steps, schema.AgentAction{Tool: "todo_add", ToolInput: `{"title":"y"}`})
	if allow {
		t.Fatal("gate observation must not be treated as a failed tool that can be retried")
	}
}
