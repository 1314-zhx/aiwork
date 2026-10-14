package tokencost

import (
	"context"
	"errors"
	"strings"
	"sync"
)

const (
	// RouterDailyNeed 进门预扣基数：够跑 Router（生成帽 256 + 问句和通道说明）。
	// 实际放行用 DailyEnterNeed，不是「3000 再加通道 8000」。
	RouterDailyNeed = 2000
	// ChatTokenLimit 闲聊单次预算（路由已花的 + 一次作答），不是日额度预扣。
	ChatTokenLimit = 3000
	// ReActDailyNeed 进门：多数 ReAct 一轮 2000–3000 就能结束；单次上限仍是 ReActTokenLimit。
	ReActDailyNeed = 3000
	// ReActTokenLimit 工具路径：第一跳与后续跳都带渐进记忆（办公 catalog / 闲聊 L0），后续跳另加 observation。
	ReActTokenLimit = 8000
	// AnalysisTokenLimit 数据分析/会话总结等多次 LLM、但无 ReAct 循环。
	AnalysisTokenLimit = 4000

	PerRequestTokenLimit = ChatTokenLimit
	DailyTokenLimit      = 40000
	MemoryTokenBudget    = 1500
	DefaultMaxTokens     = 1024
	RouterMaxTokens      = 256
	SummaryMaxTokens     = 400
	AgentFirstMaxTokens  = 512
	AgentNextMaxTokens   = 400
	RepairMaxTokens      = 256
	SQLCheckMaxTokens    = 256
	SQLIRMaxTokens       = 400
)

var (
	ErrRequestTokenLimit = errors.New("本次对话超出token上限")
	ErrDailyTokenLimit   = errors.New("今日用量已达上限")
	ErrDailyNotEnough    = errors.New("今日剩余额度不足")
)

type ctxKey struct{}

type Budget struct {
	mu    sync.Mutex
	limit int
	used  int
}

func NewBudget(limit int) *Budget {
	if limit <= 0 {
		limit = PerRequestTokenLimit
	}
	return &Budget{limit: limit}
}

func WithBudget(ctx context.Context, b *Budget) context.Context {
	return context.WithValue(ctx, ctxKey{}, b)
}

func FromContext(ctx context.Context) *Budget {
	b, _ := ctx.Value(ctxKey{}).(*Budget)
	return b
}

func (b *Budget) Used() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

func (b *Budget) Remain() int {
	if b == nil {
		return PerRequestTokenLimit
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	remain := b.limit - b.used
	if remain < 0 {
		return 0
	}
	return remain
}

func (b *Budget) Guard(estimatedPrompt int) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if estimatedPrompt < 0 {
		estimatedPrompt = 0
	}
	if b.used+estimatedPrompt >= b.limit {
		return ErrRequestTokenLimit
	}
	return nil
}

// CanAfford reports whether remaining budget can cover a hop of need tokens.
// Used before a ReAct next hop: leftover must still fit a first-hop sized call.
func (b *Budget) CanAfford(need int) error {
	if b == nil {
		return nil
	}
	if need < 0 {
		need = 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit-b.used < need {
		return ErrRequestTokenLimit
	}
	return nil
}

func IsRequestLimit(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRequestTokenLimit) {
		return true
	}
	return strings.Contains(err.Error(), ErrRequestTokenLimit.Error())
}

func (b *Budget) Add(promptTokens, completionTokens int) {
	if b == nil {
		return
	}
	total := promptTokens + completionTokens
	if total < 0 {
		return
	}
	b.mu.Lock()
	b.used += total
	b.mu.Unlock()
}

func (b *Budget) RaiseLimit(limit int) {
	if b == nil || limit <= 0 {
		return
	}
	b.mu.Lock()
	if limit > b.limit {
		b.limit = limit
	}
	b.mu.Unlock()
}

func RaiseLimit(ctx context.Context, limit int) {
	FromContext(ctx).RaiseLimit(limit)
}
