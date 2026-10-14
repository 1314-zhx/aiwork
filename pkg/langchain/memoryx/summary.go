/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package memoryx

import (
	"context"
	"fmt"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/memory"
)

type Summary struct {
	*memory.ConversationBuffer
	callback callbacks.Handler

	chain chains.Chain
}

func NewSummary(llm llms.Model, opts ...Option) *Summary {
	opt := newOption(opts...)
	return &Summary{
		callback:           opt.callback,
		ConversationBuffer: memory.NewConversationBuffer(),
		chain:              chains.NewLLMChain(llm, createSummaryPrompt(), chains.WithCallback(opt.callback)),
	}
}

// GetMemoryKey getter for memory key.
func (s *Summary) GetMemoryKey(ctx context.Context) string {
	return s.ConversationBuffer.GetMemoryKey(ctx)
}

// MemoryVariables Input keys this memory class will load dynamically.
func (s *Summary) MemoryVariables(ctx context.Context) []string {
	return s.ConversationBuffer.MemoryVariables(ctx)
}

// LoadMemoryVariables Return key-value pairs given the text input to the chain.
// If None, return all memories
func (s *Summary) LoadMemoryVariables(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return s.ConversationBuffer.LoadMemoryVariables(ctx, inputs)
}

// SaveContext Save the context of this model run to memory.
func (s *Summary) SaveContext(ctx context.Context, inputs map[string]any, outputs map[string]any) error {
	// get memory
	message, err := s.LoadMemoryVariables(ctx, inputs)
	if err != nil {
		return err
	}
	summary := message[s.MemoryKey]

	// new lines

	userInputValue, err := memory.GetInputValue(inputs, s.InputKey)
	if err != nil {
		return err
	}
	userOutPutValue, err := memory.GetInputValue(outputs, s.OutputKey)
	if err != nil {
		return err
	}
	newLines := fmt.Sprintf("Homan: %s\nAi: %s", userInputValue, userOutPutValue)
	// summary

	newSummary, err := chains.Predict(ctx, s.chain, map[string]any{
		"summary":   summary,
		"new_lines": newLines,
	})
	if err != nil {
		return err
	}

	// save
	return s.ChatHistory.SetMessages(ctx, []llms.ChatMessage{
		llms.SystemChatMessage{Content: newSummary},
	})
}

// Clear memory contents.
func (s *Summary) Clear(ctx context.Context) error {
	return s.ConversationBuffer.Clear(ctx)
}
