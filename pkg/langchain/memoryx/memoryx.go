/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package memoryx

import (
	"context"
	"sync"

	"aiworkc/pkg/langchain"
	"github.com/tmc/langchaingo/schema"
)

type Memoryx struct {
	sync.Mutex
	getMemory     func() schema.Memory
	memorys       map[string]schema.Memory // 以 session id 为 key
	defaultMemory schema.Memory
	entityStore   *EntityStore // 实体存储
}

func NewMemoryx(handle func() schema.Memory) *Memoryx {
	return NewMemoryxWithStore(handle, NewEntityStore())
}

// NewMemoryxWithStore 创建使用指定实体存储的会话记忆管理器。
func NewMemoryxWithStore(handle func() schema.Memory, entityStore *EntityStore) *Memoryx {
	if entityStore == nil {
		entityStore = NewEntityStore()
	}
	return &Memoryx{
		getMemory:     handle,
		memorys:       make(map[string]schema.Memory),
		defaultMemory: handle(),
		entityStore:   entityStore,
	}
}

// NewMemoryxWithEntityStore 创建带实体注入的 Memoryx
func NewMemoryxWithEntityStore(handle func() schema.Memory) (*Memoryx, *EntityStore) {
	entityStore := NewEntityStore()

	mem := &Memoryx{
		getMemory:     handle,
		memorys:       make(map[string]schema.Memory),
		defaultMemory: handle(),
		entityStore:   entityStore,
	}

	return mem, entityStore
}

// GetEntityStore 获取实体存储
func (s *Memoryx) GetEntityStore() *EntityStore {
	return s.entityStore
}

// GetEntityInjector 获取实体注入器
func (s *Memoryx) GetEntityInjector(model string) *EntityInjector {
	return NewEntityInjector(s.entityStore, model)
}

// GetMemoryKey getter for memory key.
func (s *Memoryx) GetMemoryKey(ctx context.Context) string {
	return s.memory(ctx).GetMemoryKey(ctx)
}

// MemoryVariables Input keys this memory class will load dynamically.
func (s *Memoryx) MemoryVariables(ctx context.Context) []string {
	return s.memory(ctx).MemoryVariables(ctx)
}

// LoadMemoryVariables Return key-value pairs given the text input to the chain.
// If None, return all memories
func (s *Memoryx) LoadMemoryVariables(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return s.memory(ctx).LoadMemoryVariables(ctx, inputs)
}

// SaveContext Save the context of this model run to memory.
func (s *Memoryx) SaveContext(ctx context.Context, inputs map[string]any, outputs map[string]any) error {
	return s.memory(ctx).SaveContext(ctx, inputs, outputs)
}

// EnsureTokenBudget 压缩当前会话的旧对话，为本轮模型调用预留上下文空间。
func (s *Memoryx) EnsureTokenBudget(ctx context.Context, currentInput string, maxTokens int) error {
	memory := s.memory(ctx)
	if summary, ok := memory.(*SummaryBuffer); ok {
		return summary.EnsureTokenBudget(ctx, currentInput, maxTokens)
	}
	return nil
}

// Clear memory contents.
func (s *Memoryx) Clear(ctx context.Context) error {
	return s.memory(ctx).Clear(ctx)
}

func (s *Memoryx) memory(ctx context.Context) schema.Memory {
	s.Lock()
	defer s.Unlock()

	var chatId string
	v := ctx.Value(langchain.ChatId)
	if v == nil {
		return s.defaultMemory
	}

	chatId = v.(string)
	memory, ok := s.memorys[chatId]
	if !ok {
		memory = s.getMemory()
		s.memorys[chatId] = memory
	}

	return memory
}
