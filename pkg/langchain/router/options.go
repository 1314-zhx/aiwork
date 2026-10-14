/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package router

import (
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/schema"
)

type Options struct {
	prompt       prompts.PromptTemplate
	memory       schema.Memory
	callback     callbacks.Handler
	emptyHandler Handler
}

type Option func(options *Options)

func executorDefaultOptions(handler []Handler) Options {
	return Options{
		prompt: createPrompt(handler),
		memory: memory.NewSimple(),
	}
}

func WithMemory(m schema.Memory) Option {
	return func(options *Options) {
		options.memory = m
	}
}

func WithEmptyHandler(emptyHandler Handler) Option {
	return func(options *Options) {
		options.emptyHandler = emptyHandler
	}
}
func Withcallback(callback callbacks.Handler) Option {
	return func(options *Options) {
		options.callback = callback
	}
}
