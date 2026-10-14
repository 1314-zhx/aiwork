package agentx

import (
	"fmt"
	"strings"

	"aiworkc/pkg/langchain/defend"

	"github.com/tmc/langchaingo/schema"
)

const (
	toolErrorPrefix = "ERROR:"
	gatePrefix      = "GATE:"
)

func lastToolStep(steps []schema.AgentStep) (schema.AgentStep, bool) {
	for i := len(steps) - 1; i >= 0; i-- {
		if strings.TrimSpace(steps[i].Action.Tool) == "" {
			continue
		}
		if IsGateObservation(steps[i].Observation) {
			continue
		}
		return steps[i], true
	}
	return schema.AgentStep{}, false
}

func toolObservationFailed(obs string) bool {
	s := strings.TrimSpace(obs)
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, toolErrorPrefix) {
		return true
	}
	return strings.Contains(s, "is not a valid tool")
}

func sameToolInput(prev schema.AgentAction, toolName, input string) bool {
	return strings.EqualFold(strings.TrimSpace(prev.Tool), toolName) &&
		normalizeJSON(prev.ToolInput) == normalizeJSON(input)
}

// gateToolCall decides whether a planned tool may run.
// After a successful tool result, any further call is refused and the model must finish.
// After a failed result, the same tool may retry only with a different input.
func gateToolCall(steps []schema.AgentStep, action schema.AgentAction) (allow bool, observation string) {
	last, ok := lastToolStep(steps)
	if !ok {
		return true, ""
	}
	if !toolObservationFailed(last.Observation) {
		preview := defend.Truncate(last.Observation, 240)
		return false, gatePrefix + " 上一工具已成功，禁止再次调用。请立即给出最终答复；若上次结果已是 ChatResp JSON，请原样输出。\n上次结果摘要:\n" + preview
	}
	if sameToolInput(last.Action, action.Tool, action.ToolInput) {
		preview := defend.Truncate(last.Observation, 240)
		return false, gatePrefix + " 上次调用失败，可用同一工具重试，但 input 必须与上次不同。\n上次结果摘要:\n" + preview
	}
	return true, ""
}

func toolErrorObservation(err error) string {
	if err == nil {
		return toolErrorPrefix + " unknown error"
	}
	return fmt.Sprintf("%s %v", toolErrorPrefix, err)
}

func IsGateObservation(obs string) bool {
	return strings.HasPrefix(strings.TrimSpace(obs), gatePrefix)
}
