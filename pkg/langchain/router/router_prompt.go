/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package router

import (
	"fmt"
	"strings"

	"aiworkc/pkg/langchain/defend"

	"github.com/tmc/langchaingo/outputparser"
	"github.com/tmc/langchaingo/prompts"
)

const (
	_formatting   = "formatting"
	_destinations = "destinations"
	_input        = "input"
	_nextInput    = "next_inputs"

	MULTI_PROMPT_ROUTER_TEMPLATE = defend.DataHandlingInstruction + `Given a raw text input to a language model select the model prompt best suited for 
the input. You will be given the names of the available prompts and a description of 
what the prompt is best suited for. You may also revise the original input if you 
think that revising it will ultimately lead to a better response from the language 
model.

<< FORMATTING >>
Return a markdown code snippet with a JSON object formatted to look like:
{{.formatting}}

REMEMBER: "destination" MUST be one of the candidate prompt names specified below OR 
it can be "DEFAULT" if the input is not well suited for any of the candidate prompts.
REMEMBER: "next_inputs" can just be the original input if you don't think any 
modifications are needed.

When destination is "knowledge" and the user is consulting company policy (leave, attendance, approval, handbook, office rules):
rewrite next_inputs into a short Chinese passage that LOOKS LIKE an official answer (2-4 declarative sentences, handbook style).
Do not keep it as a question. Include concrete policy keywords from the user text.
This rewrite is only a retrieval hint for vector search; do not present it as a real rule.
When destination is "knowledge" but the user is uploading or updating the knowledge base, keep next_inputs as the original input.

<< CANDIDATE PROMPTS >>
{{.destinations}}

<< INPUT >>
` + defend.UserInputPrefix + `{{.input}}` + defend.UserInputSuffix
)

var (
	// 定义输出解析器，你向大模型声明了：“我最终需要的是一个包含以下两个字段的字典/对象”
	_outputparser = outputparser.NewStructured([]outputparser.ResponseSchema{
		{
			// 目标路由
			// 告诉大模型，这里必须填入一个具体的系统名称（也就是我们在上一个问题中提到的候选菜单里的名字），或者如果都不匹配，就填 "DEFAULT"
			Name:        _destinations,
			Description: `name of the question answering system to use or "DEFAULT"`,
		}, {
			// 下一步输入
			// 告诉大模型，这里需要填入传给下一个处理链的指令。它可以是用户的原话，也可以是大模型帮忙润色、改写后的版本
			Name:        _nextInput,
			Description: `rewritten input for the next chain. For knowledge policy consults, a short Chinese answer-like passage (not a question). Otherwise the original or lightly revised input.`,
		},
	})
)

func createPrompt(handler []Handler) prompts.PromptTemplate {
	return prompts.PromptTemplate{
		Template:       MULTI_PROMPT_ROUTER_TEMPLATE,
		InputVariables: []string{_input},
		TemplateFormat: prompts.TemplateFormatGoTemplate,
		PartialVariables: map[string]any{
			//"destinations":
			_destinations: handlerDestinations(handler),
			_formatting:   _outputparser.GetFormatInstructions(),
		},
	}
}

func handlerDestinations(handler []Handler) string {
	var hs strings.Builder
	for _, h := range handler {
		hs.WriteString(fmt.Sprintf("- %s: %s\n", h.Name(), h.Description()))
	}

	return hs.String()
}
