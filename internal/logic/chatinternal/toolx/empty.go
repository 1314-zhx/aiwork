/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package toolx

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/prompts"
	"aiworkc/internal/svc"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
)

type Empty struct {
	chain chains.Chain
}

func NewDefaultHandler(svcCtx *svc.ServiceContext) *Empty {
	prompt := prompts.PromptTemplate{
		Template: defend.DataHandlingInstruction + `You are a helpful office assistant.
Answer the user directly. Do not repeat the question. Do not say "based on your input".
Be concise, useful, and natural.
If the question depends on context (name, what was just said, memory), use conversation history first; if history has nothing, say you do not know.
Reply in Chinese.

` + defend.HistoryPrefix + `{{.history}}` + defend.HistorySuffix + `
` + defend.UserInputPrefix + `{{.input}}` + defend.UserInputSuffix + `
Assistant:`,
		InputVariables: []string{langchain.Input, "history"},
		TemplateFormat: prompts.TemplateFormatGoTemplate,
	}
	return &Empty{
		chain: chains.NewLLMChain(svcCtx.LLMs, prompt),
	}
}

func (e *Empty) Name() string {
	return "default"
}

func (e *Empty) Description() string {
	return "Fallback handler for chitchat and memory questions. Answer directly."
}

func (e *Empty) Call(ctx context.Context, input string) (string, error) {
	return e.CallWithHistory(ctx, input, "")
}

func (e *Empty) CallWithHistory(ctx context.Context, input, history string) (string, error) {
	fmt.Println("empty -- call -- start --- ", input)

	question := input
	if strings.HasPrefix(strings.TrimSpace(input), "{") {
		var m map[string]any
		if err := json.Unmarshal([]byte(input), &m); err == nil {
			if data, ok := m["data"].(string); ok {
				question = data
			}
		}
	}

	safeQuestion := defend.Sanitize(question, defend.DefaultSanitizeConfig())
	safeHistory := defend.Sanitize(history, defend.SanitizeHistoryConfig())

	resp, err := chains.Predict(ctx, e.chain, map[string]any{
		langchain.Input: safeQuestion,
		"history":       safeHistory,
	})
	if err != nil {
		return "", err
	}

	answer := defend.RedactSecrets(strings.TrimSpace(resp))
	body, err := json.Marshal(map[string]any{
		"chatType": 0,
		"data":     answer,
	})
	if err != nil {
		return "", err
	}

	return "Final Answer: " + string(body) + "\n", nil
}
