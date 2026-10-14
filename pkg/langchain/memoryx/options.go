/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package memoryx

import "github.com/tmc/langchaingo/callbacks"

type Options struct {
	outParser
	callback    callbacks.Handler
	model       string
	entityStore *EntityStore // 实体存储

	// SummaryBuffer 压缩策略
	SummaryMaxTokens     int // 摘要本身的 token 上限，超过则对摘要再次压缩；<=0 表示不限制
	CompressCooldown     int // 相邻两次压缩至少间隔的对话轮数；<=0 表示不冷却
	ProtectedRecentTurns int // 压缩时保留最近 N 轮对话不参与摘要，减少近轮精度损失；<=0 表示不保护
	Level0MaxTokens      int // Level 0 完整对话上限，超限后按层一次压进 Level 1
	Level1MaxTokens      int // Level 1 近期摘要上限，超限后整层压进 Level 2
	Level2MaxTokens      int // Level 2 长期事件摘要上限
	TotalMemoryMaxTokens int // 三级记忆总上限
	Level0BatchTurns     int // 已弃用：压缩改为按层整批，不再按轮次批量迁移
}

type Option func(options *Options)

func newOption(opts ...Option) *Options {
	opt := &Options{
		callback:             nil,
		outParser:            nil,
		SummaryMaxTokens:     550,
		CompressCooldown:     0,
		ProtectedRecentTurns: 1,
		Level0MaxTokens:      700,
		Level1MaxTokens:      550,
		Level2MaxTokens:      300,
		TotalMemoryMaxTokens: 1550,
		Level0BatchTurns:     2,
	}

	for _, o := range opts {
		o(opt)
	}
	return opt
}

func WithCallback(handler callbacks.Handler) Option {
	return func(options *Options) {
		options.callback = handler
	}
}

func WithModel(model string) Option {
	return func(options *Options) {
		options.model = model
	}
}

func WithOutParser(outParser outParser) Option {
	return func(options *Options) {
		options.outParser = outParser
	}
}

// WithSummaryMaxTokens 设置摘要的 token 上限，超过则对摘要再次压缩
func WithSummaryMaxTokens(n int) Option {
	return func(options *Options) {
		options.SummaryMaxTokens = n
	}
}

// WithCompressCooldown 设置相邻两次压缩之间至少间隔的对话轮数
func WithCompressCooldown(n int) Option {
	return func(options *Options) {
		options.CompressCooldown = n
	}
}

// WithProtectedRecentTurns 设置压缩时保留最近 N 轮对话不参与摘要，减少近轮精度损失
func WithProtectedRecentTurns(n int) Option {
	return func(options *Options) {
		options.ProtectedRecentTurns = n
	}
}

// WithTieredMemoryLimits 设置 Level 0、Level 1、Level 2 及三级总 Token 上限。
func WithTieredMemoryLimits(level0, level1, level2, total int) Option {
	return func(options *Options) {
		options.Level0MaxTokens = level0
		options.Level1MaxTokens = level1
		options.Level2MaxTokens = level2
		options.TotalMemoryMaxTokens = total
	}
}

// WithLevel0BatchTurns 设置每次从完整对话迁入近期摘要的轮数。
func WithLevel0BatchTurns(n int) Option {
	return func(options *Options) {
		options.Level0BatchTurns = n
	}
}
