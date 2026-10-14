package tokencost

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

type LLM struct {
	*openai.LLM
	modelName string
}

func Wrap(inner *openai.LLM, modelName string) *LLM {
	if modelName == "" {
		modelName = "qwen-turbo"
	}
	return &LLM{LLM: inner, modelName: modelName}
}

func (l *LLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, l, prompt, options...)
}

func (l *LLM) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	budget := FromContext(ctx)
	opts := llms.CallOptions{}
	for _, opt := range options {
		opt(&opts)
	}
	estimated := estimatePromptTokens(l.modelName, messages, opts)
	if err := budget.Guard(estimated); err != nil {
		return nil, err
	}
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	remain := budget.Remain() - estimated
	if remain < 1 {
		return nil, ErrRequestTokenLimit
	}
	// Next hop is cheaper than first hop, but leftover below first-hop size
	// is treated as "not enough for a full hop" so the caller can leave a retry ticket.
	if maxTokens == AgentNextMaxTokens && remain < AgentFirstMaxTokens {
		return nil, ErrRequestTokenLimit
	}
	if maxTokens > remain {
		maxTokens = remain
	}
	options = append(options, llms.WithMaxTokens(maxTokens))

	resp, err := l.LLM.GenerateContent(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	promptTokens, completionTokens := usageFrom(resp)
	if promptTokens == 0 && completionTokens == 0 {
		budget.Add(estimated, 0)
	} else {
		budget.Add(promptTokens, completionTokens)
	}
	return resp, nil
}

// CountTokens estimates prompt tokens for model. Empty text is 0.
func CountTokens(model, text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	if model == "" {
		model = "qwen-turbo"
	}
	n := llms.CountTokens(model, text)
	if n < 1 {
		return 1
	}
	return n
}

// TruncateToTokens keeps a prefix of text whose estimate is at most limit.
func TruncateToTokens(model, text string, limit int) string {
	if limit <= 0 || text == "" {
		return ""
	}
	if CountTokens(model, text) <= limit {
		return text
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	best := ""
	for lo <= hi {
		mid := (lo + hi) / 2
		cand := string(runes[:mid])
		if CountTokens(model, cand) <= limit {
			best = cand
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return best
}

func estimatePromptTokens(model string, messages []llms.MessageContent, opts llms.CallOptions) int {
	var b strings.Builder
	for _, msg := range messages {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case llms.TextContent:
				b.WriteString(p.Text)
				b.WriteByte('\n')
			case llms.ToolCall:
				if p.FunctionCall != nil {
					b.WriteString(p.FunctionCall.Name)
					b.WriteString(p.FunctionCall.Arguments)
					b.WriteByte('\n')
				}
			case llms.ToolCallResponse:
				b.WriteString(p.Name)
				b.WriteString(p.Content)
				b.WriteByte('\n')
			}
		}
	}
	for _, fn := range opts.Functions {
		b.WriteString(fn.Name)
		b.WriteString(fn.Description)
		if fn.Parameters != nil {
			if raw, err := jsonMarshal(fn.Parameters); err == nil {
				b.Write(raw)
			}
		}
	}
	for _, tool := range opts.Tools {
		if tool.Function == nil {
			continue
		}
		b.WriteString(tool.Function.Name)
		b.WriteString(tool.Function.Description)
		if tool.Function.Parameters != nil {
			if raw, err := jsonMarshal(tool.Function.Parameters); err == nil {
				b.Write(raw)
			}
		}
	}
	n := llms.CountTokens(model, b.String())
	if n < 1 {
		return 1
	}
	return n
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func usageFrom(resp *llms.ContentResponse) (prompt, completion int) {
	if resp == nil {
		return 0, 0
	}
	for _, c := range resp.Choices {
		if c.GenerationInfo == nil {
			continue
		}
		prompt += asInt(c.GenerationInfo["PromptTokens"])
		completion += asInt(c.GenerationInfo["CompletionTokens"])
		if prompt == 0 && completion == 0 {
			total := asInt(c.GenerationInfo["TotalTokens"])
			prompt += total
		}
	}
	return prompt, completion
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	default:
		return 0
	}
}
