/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package visual

import (
	"github.com/tmc/langchaingo/outputparser"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
)

// 获取分析的数据字段、数据值 选择的工具
const (
	ImgUrl = "imgUrl"
	X      = "x-title"
	Y      = "y-title"
	Title  = "title"

	_destinations = "destinations"
	_formatting   = "formatting"

	_defaultVisualParsePrompts = defend.DataHandlingInstruction + `
	# Role
	Extract chart fields from the user text. Do not rewrite the input or compute analytics.

	## Tasks
	1. Understand the user text.
	2. Identify the main analysis subject.
	3. Pick one output type from [bar].
	4. Decide x-axis and y-axis titles.

	## Types
	- bar: single category values on the x-axis (bar chart)

	## Output
	{{.formatting}}

	## Input
` + defend.UserInputPrefix + `{{.input}}` + defend.UserInputSuffix + `
`
	_defaultVBarPrompts = defend.DataHandlingInstruction + `
	# Role
	Extract numeric series from the user text. Do not rewrite the input or compute analytics.

	## Tasks
	1. Understand the user text.
	2. Extract values using the given x-axis and y-axis titles.

	## Output
	1. JSON object: title -> number
	2. Titles must be short and unique
	3. Convert titles to English
	4. Example: {"chinese": 100, "math": 90, "physics": 90}

	## Example
	User: 在这次考试里木兮的语文有80分、数学有90分、物理有90分
	Result: {"chinese": 80, "math": 90, "physics": 90}

	## Input
` + defend.UserInputPrefix + `{{.input}}` + defend.UserInputSuffix + `
`
)

var (
	_outputParserVisual = outputparser.NewStructured([]outputparser.ResponseSchema{
		{
			Name:        _destinations,
			Description: "name of the prompt to use or \"DEFAULT\"",
		}, {
			Name:        X,
			Description: "x-axis title",
		}, {
			Name:        Y,
			Description: "y-axis title",
		}, {
			Name:        Title,
			Description: "chart title",
		}, {
			Name:        langchain.Input,
			Description: "original user input",
		},
	})
)
