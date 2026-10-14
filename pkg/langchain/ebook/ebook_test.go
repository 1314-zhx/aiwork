/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package ebook

import (
	"os"
	"testing"
)

import (
	"context"

	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms/openai"
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

func TestNewEbook(t *testing.T) {
	llm := getLLmOpenaiClient(t, openai.WithModel("qwen-turbo"), openai.WithCallback(callbacks.LogHandler{}))
	e := NewEbook(llm, "./upload/")
	res, err := chains.Call(context.Background(), e, map[string]any{
		"input": "我想写一篇关于上班族如何在上班的时候健身的文章",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 只打印关键信息，不要打印完整文章
	t.Logf("Ebook generated successfully, output type: %T", res["output"])
	// 如果需要查看完整内容，可以读取生成的临时文件
	t.Log("Check ./upload/ directory for generated files")
}
