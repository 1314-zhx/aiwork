/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package router

import (
	"aiworkc/pkg/audit"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/memoryx"
	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/langchain/tokenretry"
	"context"
	"errors"
	"strings"

	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/outputparser"
	"github.com/tmc/langchaingo/schema"
)

const Empty = "DEFAULT"

var ErrNotHandles = errors.New("不存在合适的handler")

type Router struct {
	handlers     map[string]Handler
	chain        chains.Chain
	callbacks    callbacks.Handler
	memory       schema.Memory
	outputparser outputparser.Structured
	emptyHandle  Handler
}

func NewRouter(llm llms.Model, handler []Handler, opts ...Option) *Router {
	opt := executorDefaultOptions(handler)
	for _, o := range opts {
		o(&opt)
	}

	hs := make(map[string]Handler, len(handler))
	for _, h := range handler {
		hs[h.Name()] = h
	}

	return &Router{
		handlers:     hs,
		chain:        chains.NewLLMChain(llm, opt.prompt),
		callbacks:    opt.callback,
		memory:       opt.memory,
		emptyHandle:  opt.emptyHandler,
		outputparser: _outputparser,
	}
}

func (r *Router) Call(ctx context.Context, inputs map[string]any, options ...chains.ChainCallOption) (map[string]any, error) {
	if r.callbacks != nil {
		r.callbacks.HandleChainStart(ctx, inputs)
	}

	// loadHistory 闲聊只带 Level 0；办公 ReAct 带 Level 0 与 L1/L2 目录，命中目录时再展开正文。
	loadHistory := func(mode string) string {
		if r.memory == nil {
			return ""
		}
		loadIn := map[string]any{memoryx.LoadModeKey: mode}
		for k, v := range inputs {
			loadIn[k] = v
		}
		loadIn[memoryx.LoadModeKey] = mode
		vars, err := r.memory.LoadMemoryVariables(ctx, loadIn)
		if err != nil {
			return ""
		}
		key := r.memory.GetMemoryKey(ctx)
		if h, ok := vars[key].(string); ok {
			return defend.Sanitize(h, defend.SanitizeHistoryConfig())
		}
		return ""
	}

	if len(r.handlers) == 0 {
		if r.emptyHandle != nil {
			return r.callEmpty(ctx, inputs, loadHistory)
		}
		return nil, NewUnknownIntent(nil, "", inputString(inputs))
	}
	// 意图识别
	// Prompt injection 防护：清洗用户输入（最高优先入口）
	if rawInput, ok := inputs[langchain.Input].(string); ok {
		inputs[langchain.Input] = defend.Sanitize(rawInput, defend.DefaultSanitizeConfig())
	}

	result, err := chains.Call(ctx, r.chain, inputs, chains.WithMaxTokens(tokencost.RouterMaxTokens))
	if err != nil {
		return nil, err
	}

	text, ok := result["text"]
	if !ok {
		if r.emptyHandle != nil {
			return nil, chains.ErrNotFound
		}
		return nil, NewUnknownIntent(handlerNames(r.handlers), "", inputString(inputs))
	}
	// 解析输出
	out, err := r.outputparser.Parse(text.(string))
	if err != nil {
		if r.emptyHandle != nil {
			return nil, err
		}
		return nil, NewUnknownIntent(handlerNames(r.handlers), "", inputString(inputs))
	}

	if r.callbacks != nil {
		r.callbacks.HandleChainEnd(ctx, map[string]any{
			"out": out,
		})
	}

	// 获取路由结果并调用单个 Handler。
	data := out.(map[string]string)
	next, ok := data[_destinations]
	nextInputs := strings.TrimSpace(data[_nextInput])
	if !ok || next == Empty || r.handlers[next] == nil {
		if r.emptyHandle != nil {
			return r.callEmpty(ctx, inputs, loadHistory)
		}
		if nextInputs == "" {
			nextInputs = inputString(inputs)
		}
		return nil, NewUnknownIntent(handlerNames(r.handlers), next, nextInputs)
	}

	// 使用路由 LLM 重写后的问题（如果有）
	// 更新输入，同时注入从 memory 加载的历史对话
	handlerInputs := map[string]any{
		langchain.Input: defend.Sanitize(nextInputs, defend.DefaultSanitizeConfig()),
		"history":       loadHistory(memoryx.LoadModeCatalog),
		// 关联的会话 ID
		"relationId": inputs["relationId"],
		"startTime":  inputs["startTime"],
		"endTime":    inputs["endTime"],
	}

	if err := tokencost.RequireDaily(ctx, tokencost.HandlerDailyNeed(next)); err != nil {
		return nil, err
	}
	tokenretry.SetHandler(ctx, next)
	audit.Event(ctx, "router.dest", "handler", next)
	return chains.Call(ctx, r.handlers[next].Chains(), handlerInputs)
}

func (r *Router) callEmpty(ctx context.Context, inputs map[string]any, loadHistory func(string) string) (map[string]any, error) {
	if err := tokencost.RequireDaily(ctx, tokencost.DailyEnterNeed(tokencost.RouterDailyNeed)); err != nil {
		return nil, err
	}
	inputs["history"] = loadHistory(memoryx.LoadModeL0)
	return chains.Call(ctx, r.emptyHandle.Chains(), inputs)
}

func (r *Router) Handler(name string) Handler {
	if r == nil {
		return nil
	}
	return r.handlers[name]
}

func (r *Router) GetMemory() schema.Memory {
	if r.memory == nil {
		return memory.NewSimple()
	}
	return r.memory
}

func (r *Router) GetInputKeys() []string {
	return nil
}

func (r *Router) GetOutputKeys() []string {
	return nil
}
