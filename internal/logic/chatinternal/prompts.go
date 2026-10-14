/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package chatinternal

import "aiworkc/pkg/langchain/defend"

const (
	BASE_PROMPAT_TEMPLATE = `Current conversation:
` + defend.HistoryPrefix + `{{.history}}` + defend.HistorySuffix + `

`

	_defaultMrklFormatInstructions = `Use the following format:

Question: the input question you must answer
Thought: you should always think about what to do
Action: the action to take, should be one of [ {{.tool_names}} ]
Action Input: the input to the action
Observation: the result of the action
... (this Thought/Action/Action Input/Observation can repeat N times)
Thought: I now know the final answer
Final Answer: {"chatType": 0, "data": "the answer in Chinese, answer only, no tool output or file paths"}`

	_defaultMrklSuffix = `Begin!

Question: ` + defend.UserInputPrefix + `{{.input}}` + defend.UserInputSuffix + `
Thought:{{.agent_scratchpad}}`

	_defaultMrklPrefix = defend.DataHandlingInstruction + `Today is {{.today}}.
Answer the following questions as best you can. You have access to the following tools:

{{.tool_descriptions}}` + BASE_PROMPAT_TEMPLATE

	OUT_PROMPT_TEMPLATE = `Use the following format:

Question: the input question you must answer
Thought: you should always think about what to do
Action: the action to take, should be one of [ {{.tool_names}} ]
Action Input: the input to the action
Observation: the result of the action
... (this Thought/Action/Action Input/Observation can repeat N times)
Thought: I now know the final answer
Final Answer: {"chatType": 0, "data": "the answer in Chinese, answer only, no tool output or file paths"}`

	_defaultChatLogPrompts = defend.DataHandlingInstruction + `## Task
Summarize the conversation according to the user's goal.
Use only the conversation memory. Reply in Chinese.

## User goal
` + defend.UserInputPrefix + `{{.goal}}` + defend.UserInputSuffix + `

## Conversation memory
` + defend.HistoryPrefix + `{{.history}}` + defend.HistorySuffix + `

## Output
Plain Chinese text. No markdown fences. No tool names or file paths.
If the memory is empty, say there is nothing to summarize.`
)
