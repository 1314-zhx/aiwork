/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package memoryx

import "github.com/tmc/langchaingo/prompts"

const (
	_DEFAULT_SUMMARIZER_TEMPLATE = `Append a summary of the new dialogue onto the existing summary.

Rules:
1. Keep the existing summary unchanged.
2. Only append a summary of the new lines.
3. Keep names, numbers, dates, times, and identities in the original language.
4. Do not use generic pronouns like "user" or "AI"; use names from the dialogue.
5. The appended text must read naturally after the existing summary.

Example
Existing:
用户问AI什么是微服务。AI解释微服务是一种架构。

New:
Human: 我是物理
AI: 你好，物理！有什么问题吗？

Result:
用户问AI什么是微服务。AI解释微服务是一种架构。用户介绍自己叫物理，AI回应问候。

---

Existing:
{{.summary}}

New:
{{.new_lines}}

Result:`
)

func createSummaryPrompt() prompts.PromptTemplate {
	return prompts.NewPromptTemplate(_DEFAULT_SUMMARIZER_TEMPLATE, []string{
		"summary", "new_lines",
	})
}
