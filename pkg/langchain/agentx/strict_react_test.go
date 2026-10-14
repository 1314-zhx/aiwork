package agentx

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/tools"
)

func TestDecodeStrictJSONRejectsUnknownFields(t *testing.T) {
	var result response
	err := decodeStrictJSON(`{"type":"finish","answer":"ok","thought":"hidden"}`, &result)
	if err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestDecodeStrictJSONAcceptsMarkdownFence(t *testing.T) {
	var result response
	err := decodeStrictJSON("```json\n{\"type\":\"finish\",\"answer\":\"完成\"}\n```", &result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "finish" || result.Answer != "完成" {
		t.Fatalf("unexpected response: %#v", result)
	}
}

func TestRepeatedActionNormalizesJSON(t *testing.T) {
	prev := schema.AgentAction{Tool: "todo_find", ToolInput: `{"id":"1","count":10}`}
	if !sameToolInput(prev, "TODO_FIND", `{ "count": 10, "id": "1" }`) {
		t.Fatal("expected semantically identical action to be detected")
	}
}

func TestUsedToolDescriptionsIncludesOnlyUsedSpecs(t *testing.T) {
	agent := &StrictReActAgent{
		toolNames: map[string]string{
			"TODO_ADD":  "todo_add",
			"TODO_FIND": "todo_find",
		},
		toolSpecs: map[string]string{
			"todo_add":  "extract todo fields",
			"todo_find": "find todos by filter",
		},
	}
	steps := []schema.AgentStep{
		{Action: schema.AgentAction{Tool: "todo_add"}, Observation: "draft"},
		{Action: schema.AgentAction{Tool: "TODO_ADD"}, Observation: "draft-2"},
	}
	got := agent.usedToolDescriptions(steps)
	if got != "- todo_add: extract todo fields" {
		t.Fatalf("unexpected used specs: %s", got)
	}
	if strings.Contains(got, "todo_find") {
		t.Fatal("follow-up must not reload unused tool descriptions")
	}
}

func TestMarshalObservationsOmitsToolInput(t *testing.T) {
	steps := []schema.AgentStep{{
		Action:      schema.AgentAction{Tool: "todo_find", ToolInput: `{"secret":"x"}`, Log: "private"},
		Observation: "found",
	}}
	result := marshalObservations(steps)
	if !strings.Contains(result, `"tool":"todo_find"`) || !strings.Contains(result, "found") {
		t.Fatalf("unexpected observations: %s", result)
	}
	if !strings.Contains(result, "UNTRUSTED") {
		t.Fatal("observations must be isolated as untrusted frames")
	}
	if strings.Contains(result, "secret") || strings.Contains(result, "private") {
		t.Fatal("follow-up prompt must not replay tool input or thought log")
	}
}

func TestMarshalStepsDoesNotExposeThoughtLog(t *testing.T) {
	steps := []schema.AgentStep{{
		Action:      schema.AgentAction{Tool: "todo_find", ToolInput: `{}`, Log: "private thought"},
		Observation: "found",
	}}
	result := marshalSteps(steps)
	if result != `[{"tool":"todo_find","input":"{}","observation":"found"}]` {
		t.Fatalf("unexpected structured scratchpad: %s", result)
	}
}

func TestParseChoiceNativeToolCall(t *testing.T) {
	agent := &StrictReActAgent{toolNames: map[string]string{"TODO_ADD": "todo_add"}}
	actions, finish, err := agent.parseChoice(&llms.ContentChoice{
		Content: "calling",
		ToolCalls: []llms.ToolCall{{
			ID: "call_1",
			FunctionCall: &llms.FunctionCall{
				Name:      "todo_add",
				Arguments: `{"title":"写周报"}`,
			},
		}},
	})
	if err != nil || finish != nil || len(actions) != 1 {
		t.Fatalf("err=%v finish=%v actions=%v", err, finish, actions)
	}
	if actions[0].Tool != "todo_add" || actions[0].ToolInput != `{"title":"写周报"}` || actions[0].ToolID != "call_1" {
		t.Fatalf("action=%#v", actions[0])
	}
}

func TestParseChoiceContentFinish(t *testing.T) {
	agent := &StrictReActAgent{}
	_, finish, err := agent.parseChoice(&llms.ContentChoice{Content: `{"chatType":0,"data":"好的"}`})
	if err != nil || finish == nil {
		t.Fatalf("err=%v finish=%v", err, finish)
	}
	if finish.ReturnValues[outputKey] != `{"chatType":0,"data":"好的"}` {
		t.Fatalf("output=%v", finish.ReturnValues[outputKey])
	}
}

func TestParseChoiceLegacyJSONActionFallback(t *testing.T) {
	agent := &StrictReActAgent{toolNames: map[string]string{"TODO_FIND": "todo_find"}}
	actions, finish, err := agent.parseChoice(&llms.ContentChoice{
		Content: `{"type":"action","tool":"todo_find","input":{}}`,
	})
	if err != nil || finish != nil || len(actions) != 1 || actions[0].Tool != "todo_find" {
		t.Fatalf("err=%v finish=%v actions=%v", err, finish, actions)
	}
}

func TestPlanFirstHopSendsToolsAndParsesCall(t *testing.T) {
	stub := &stubModel{resp: &llms.ContentResponse{Choices: []*llms.ContentChoice{{
		ToolCalls: []llms.ToolCall{{
			ID: "c1",
			FunctionCall: &llms.FunctionCall{
				Name:      "todo_find",
				Arguments: `{}`,
			},
		}},
	}}}}
	agent := NewStrictReActAgent(stub, []tools.Tool{toolsAdapter{name: "todo_find", desc: "find todos"}}, nil)
	actions, finish, err := agent.Plan(context.Background(), nil, map[string]string{"input": "查待办", "history": ""})
	if err != nil || finish != nil || len(actions) != 1 {
		t.Fatalf("err=%v finish=%v actions=%v", err, finish, actions)
	}
	if len(stub.lastOpts.Tools) != 1 || stub.lastOpts.Tools[0].Function.Name != "todo_find" {
		t.Fatalf("tools=%#v", stub.lastOpts.Tools)
	}
	if stub.lastOpts.ToolChoice != "auto" {
		t.Fatalf("tool_choice=%v", stub.lastOpts.ToolChoice)
	}
}

func TestPlanNextHopAfterSuccessDisablesTools(t *testing.T) {
	stub := &stubModel{resp: &llms.ContentResponse{Choices: []*llms.ContentChoice{{
		Content: `{"chatType":1,"data":[]}`,
	}}}}
	agent := NewStrictReActAgent(stub, []tools.Tool{toolsAdapter{name: "todo_find", desc: "find todos"}}, nil)
	steps := []schema.AgentStep{{
		Action:      schema.AgentAction{Tool: "todo_find", ToolInput: `{}`, ToolID: "c1"},
		Observation: `{"chatType":1,"data":[]}`,
	}}
	_, finish, err := agent.Plan(context.Background(), steps, map[string]string{"input": "查待办"})
	if err != nil || finish == nil {
		t.Fatalf("err=%v finish=%v", err, finish)
	}
	if stub.lastOpts.ToolChoice != "none" {
		t.Fatalf("expected tool_choice none, got %v", stub.lastOpts.ToolChoice)
	}
}

func TestPlanNextHopIncludesProgressiveHistory(t *testing.T) {
	stub := &stubModel{resp: &llms.ContentResponse{Choices: []*llms.ContentChoice{{
		Content: `{"chatType":0,"data":"好的"}`,
	}}}}
	agent := NewStrictReActAgent(stub, []tools.Tool{toolsAdapter{name: "todo_find", desc: "find todos"}}, nil)
	steps := []schema.AgentStep{{
		Action:      schema.AgentAction{Tool: "todo_find", ToolInput: `{}`, ToolID: "c1"},
		Observation: `{"chatType":1,"data":[]}`,
	}}
	_, _, err := agent.Plan(context.Background(), steps, map[string]string{
		"input":   "把结果画成柱状图",
		"history": "## 近期记忆目录\n- 请假草稿 · 明天\n## 近期记忆\n用户要画图",
	})
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(stub.lastMsgs)
	got := string(blob)
	if !strings.Contains(got, "把结果画成柱状图") || !strings.Contains(got, "用户要画图") {
		t.Fatalf("next hop missing memory/user: %s", got)
	}
}

type stubModel struct {
	lastMsgs []llms.MessageContent
	lastOpts llms.CallOptions
	resp     *llms.ContentResponse
	err      error
}

func (s *stubModel) GenerateContent(_ context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	s.lastMsgs = messages
	opts := llms.CallOptions{}
	for _, o := range options {
		o(&opts)
	}
	s.lastOpts = opts
	return s.resp, s.err
}

func (s *stubModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", errors.New("not used")
}

type toolsAdapter struct {
	name, desc string
}

func (t toolsAdapter) Name() string                                 { return t.name }
func (t toolsAdapter) Description() string                          { return t.desc }
func (t toolsAdapter) Call(context.Context, string) (string, error) { return "", nil }
func (t toolsAdapter) ParameterSchema() any {
	return map[string]any{"type": "object", "additionalProperties": true}
}
