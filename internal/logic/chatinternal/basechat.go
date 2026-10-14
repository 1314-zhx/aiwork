/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package chatinternal

import (
	"context"
	"encoding/json"
	"strings"

	"aiworkc/internal/domain"
	"aiworkc/internal/svc"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/agentx"
	"aiworkc/pkg/langchain/outputrepair"
	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/langchain/tokenretry"

	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/tools"
)

type baseChat struct {
	agent        *agentx.StrictReActAgent
	errorHandler *agents.ParserErrorHandler
	maxIter      int
	llm          llms.Model
}

func NewBaseChat(svc *svc.ServiceContext, toolsList []tools.Tool) *baseChat {
	return newBaseChat(svc, toolsList, 4)
}

// NewOneShotChat 待办/审批：第一跳全量上下文选工具；第二跳只用 observation 和已用工具描述，产出格式化 Final Answer。
func NewOneShotChat(svc *svc.ServiceContext, toolsList []tools.Tool) *baseChat {
	return newBaseChat(svc, toolsList, 2)
}

func newBaseChat(svc *svc.ServiceContext, toolsList []tools.Tool, maxIterations int) *baseChat {
	// 创建错误处理器，当 agent 无法解析输出时，告诉它正确的格式
	errorHandler := &agents.ParserErrorHandler{
		Formatter: func(err string) string {
			if idx := strings.LastIndex(err, ": "); idx >= 0 {
				output := err[idx+2:]
				if len(output) > 10 && (strings.Contains(output, "是") || strings.Contains(output, "。") || strings.Contains(output, ".")) {
					return "请直接输出最终中文答复。若已有 ChatResp JSON（chatType + data），请原样输出，不要再调用工具。"
				}
			}
			return "请通过 function calling 调用一个工具，或直接输出最终中文答复 / ChatResp JSON。不要输出 Thought、Action 文本。"
		},
	}

	strictAgent := agentx.NewStrictReActAgent(svc.LLMs, toolsList, svc.Callbacks)
	return &baseChat{
		llm:          svc.LLMs,
		agent:        strictAgent,
		errorHandler: errorHandler,
		maxIter:      maxIterations,
	}
}

func (t *baseChat) Chains() chains.Chain {
	return chains.NewTransform(t.transform, nil, nil)
}

func (t *baseChat) transform(ctx context.Context, inputs map[string]any,
	opts ...chains.ChainCallOption) (map[string]any,
	error) {

	for s, a := range inputs {
		if _, ok := a.(string); !ok {
			delete(inputs, s)
		}
	}

	tokencost.RaiseLimit(ctx, tokencost.ReActTokenLimit)
	if err := tokencost.RequireDaily(ctx, tokencost.DailyEnterNeed(tokencost.ReActTokenLimit)); err != nil {
		return nil, err
	}
	strInputs := map[string]string{}
	for k, v := range inputs {
		if s, ok := v.(string); ok {
			strInputs[k] = s
		}
	}
	outPut, err := agentx.Run(ctx, t.agent, t.maxIter, strInputs, tokenretry.ResumeSteps(ctx), t.errorHandler)
	if tokencost.IsRequestLimit(err) {
		return nil, err
	}

	// 创建类工具只产出草稿；后续 Plan 不得覆盖待确认 JSON
	if jsonStr := extractConfirmDraft(outPut); jsonStr != "" {
		return map[string]any{
			langchain.OutPut: jsonStr,
		}, nil
	}

	// 尝试从输出中提取文本并解析
	if text := extractTextFromOutput(outPut); text != "" {
		if jsonStr := tryParseToJSON(text); jsonStr != "" {
			return map[string]any{
				langchain.OutPut: jsonStr,
			}, nil
		}
		userInput, _ := inputs[langchain.Input].(string)
		if res, ok := outputrepair.Repair(ctx, t.llm, userInput, text); ok {
			if jsonStr := outputrepair.EncodeChatResp(res); jsonStr != "" {
				return map[string]any{
					langchain.OutPut: jsonStr,
				}, nil
			}
		}
		if err != nil {
			return map[string]any{
				langchain.OutPut: wrapToJSON(text),
			}, nil
		}
	}

	// 如果没有文本但有错误
	if err != nil {
		return nil, err
	}

	return outPut, nil
}

func isConfirmDraft(res domain.ChatResp) bool {
	return res.ChatType == int(domain.ConfirmTodo) || res.ChatType == int(domain.ConfirmApproval) || res.ChatType == int(domain.ConfirmSQL)
}

// extractConfirmDraft 从任意中间步骤中取出待确认草稿，避免被后续 Plan 覆盖。
func extractConfirmDraft(outPut map[string]any) string {
	if outPut == nil {
		return ""
	}

	var found string
	if v, ok := outPut["intermediateSteps"]; ok {
		if steps, ok := v.([]schema.AgentStep); ok {
			for _, step := range steps {
				if jsonStr := tryParseToJSON(step.Observation); jsonStr == "" {
					continue
				} else {
					var res domain.ChatResp
					if err := json.Unmarshal([]byte(jsonStr), &res); err == nil && isConfirmDraft(res) {
						found = jsonStr
					}
				}
			}
		}
	}
	if found != "" {
		return found
	}

	if v, ok := outPut["output"]; ok {
		if text, ok := v.(string); ok {
			if jsonStr := tryParseToJSON(text); jsonStr != "" {
				var res domain.ChatResp
				if err := json.Unmarshal([]byte(jsonStr), &res); err == nil && isConfirmDraft(res) {
					return jsonStr
				}
			}
		}
	}
	return ""
}

// extractTextFromOutput 从 agent 输出中提取文本
func extractTextFromOutput(outPut map[string]any) string {
	// 优先尝试 output 字段
	if v, ok := outPut["output"]; ok {
		if text, ok := v.(string); ok && text != "" && text != "agent not finished before max iterations" {
			return text
		}
	}
	// 尝试 intermediateSteps
	if v, ok := outPut["intermediateSteps"]; ok {
		if steps, ok := v.([]schema.AgentStep); ok {
			for i := len(steps) - 1; i >= 0; i-- {
				if steps[i].Observation != "" && !agentx.IsGateObservation(steps[i].Observation) {
					return steps[i].Observation
				}
			}
		}
	}
	return ""
}

// tryParseToJSON 第一层：markdown / 转义 JSON 清洗后解析为 ChatResp。
func tryParseToJSON(text string) string {
	res, ok := outputrepair.ParseChatResp(text)
	if !ok {
		return ""
	}
	return outputrepair.EncodeChatResp(res)
}

// wrapToJSON 将纯文本包装成 ChatResp JSON 格式
func wrapToJSON(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	resp := domain.ChatResp{
		ChatType: 0,
		Data:     text,
	}
	d, _ := json.Marshal(resp)
	return string(d)
}

func (t *baseChat) wrapOutput(v any) (map[string]any, error) {
	text, ok := v.(string)
	if !ok {
		return nil, nil
	}
	return map[string]any{
		langchain.OutPut: wrapToJSON(text),
	}, nil
}
