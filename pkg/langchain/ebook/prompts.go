/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package ebook

import (
	"github.com/tmc/langchaingo/prompts"
)

const (
	EbookTools           = "tools"
	EbookToolsDesc       = "tools_desc"
	_finalAnswerAction   = "文章生成成功"
	EbookWritePlanPrompt = `# Role
Article writing assistant.

## Process
1. Read the article content and chapter abstracts.
2. Follow chapter order.
3. Plan only the current section and its word count.
4. Use the write log to decide if the article is done.
5. If done, output exactly: 文章生成成功
6. Output the plan only. No extra text. Do not plan future sections.

## Output
计划：current write plan, e.g. 撰写 1. 引言 字数：600字以上

## Article
{{.input}}

## Write log
{{.history}}`

	SubjectToolPrompt = `# Role
Article writing assistant. From the user text, write theme, chapters, and abstract in Chinese.

## Output labels (exact)
主题：theme title
章节：chapter list with word counts
摘要：abstract from theme and chapters

## Example
1. 介绍 - 500字
2. Go语言简介 - 600字
3. 智能办公助手的概念 - 600字
4. 实现语音识别功能 - 600字
5. 结论 - 400字

## Input
{{.input}}`

	ContentPrompt = `# Role
Article writing assistant. Write the requested chapter only, in Chinese.

## Tasks
1. Read theme, chapter title, and abstract.
2. Write only the requested section.

## Output
章节序号: 章节标题
章节内容

## Example
章节1. 引言
在当今快节奏的工作环境中，智能办公助手的需求日益增长。它们不仅能够提高工作效率，还能通过自动化任务和提供智能建议来简化日常工作流程。

## Input
{{.input}}
`
)

func CreateEbookWritePlan() prompts.PromptTemplate {
	return prompts.PromptTemplate{
		Template:         EbookWritePlanPrompt,
		InputVariables:   []string{"history"},
		TemplateFormat:   prompts.TemplateFormatGoTemplate,
		PartialVariables: map[string]any{},
	}
}
