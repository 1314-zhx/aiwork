/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package router

import (
	"context"
	"os"
	"testing"

	"gitee.com/dn-jinmin/tlog"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
	"aiworkc/pkg/langchain/callbackx"
)

var (
	apiKey = os.Getenv("AIWORKC_API_KEY")
	url    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
)

func getLLmOpenaiClient(t *testing.T, opts ...openai.Option) *openai.LLM {
	if apiKey == "" {
		t.Skip("set AIWORKC_API_KEY to run live LLM tests")
	}
	opts = append(opts, openai.WithBaseURL(url), openai.WithToken(apiKey))
	llm, err := openai.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	return llm
}

func TestRouter(t *testing.T) {
	logger := tlog.NewLogger(tlog.WithLogWriteLimit(1))

	callback := callbacks.CombiningHandler{
		Callbacks: []callbacks.Handler{
			callbackx.NewLogHandler(logger),
			callbackx.NewTitTokenHandle(logger),
		},
	}

	llms := getLLmOpenaiClient(t, openai.WithCallback(callback))

	handlers := []Handler{
		NewSwimmingHandler(llms),
		NewBasketballHandler(llms),
	}

	router := NewRouter(llms, handlers, Withcallback(callback))

	res, err := chains.Call(tlog.TraceStart(context.Background()), router, map[string]any{
		"input": "请问游泳的类型有哪些",
	}, chains.WithCallback(callback))
	//res, err := chains.Call(context.Background(), router, map[string]any{
	//	"input": "乒乓球怎么打",
	//}, chains.WithCallback(callback))

	t.Log(res)
	t.Log(err)

}

type SwimmingHandler struct {
	c chains.Chain
}

func NewSwimmingHandler(llms llms.Model) *SwimmingHandler {
	return &SwimmingHandler{c: chains.NewLLMChain(
		llms,
		prompts.NewPromptTemplate(
			"你是一个资深游泳教练，精通游泳相关的所有知识, 请回答下面的问题：\n{{.input}}", []string{"input"},
		),
	)}
}

func (h SwimmingHandler) Name() string {
	return "swimming"
}

func (h SwimmingHandler) Description() string {
	return "适合回答游泳相关的知识"
}
func (h SwimmingHandler) Chains() chains.Chain {
	return h.c
}

type BasketballHandler struct {
	c chains.Chain
}

func NewBasketballHandler(llms llms.Model) *BasketballHandler {
	return &BasketballHandler{c: chains.NewLLMChain(
		llms,
		prompts.NewPromptTemplate(
			"你是一个资深篮球教练，精通篮球相关的所有知识, 请回答下面的问题：\n{{.input}}", []string{"input"},
		),
	)}
}

func (h BasketballHandler) Name() string {
	return "basketball"
}

func (h BasketballHandler) Description() string {
	return "适合回答篮球相关的知识"
}
func (h BasketballHandler) Chains() chains.Chain {
	return h.c
}
