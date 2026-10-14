/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package analysis

import (
	"github.com/tmc/langchaingo/outputparser"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
)

const (
	_destinations = "destinations"

	_defaultDBQuery = defend.DataHandlingInstruction + `
	# Role
	Rewrite the user question, pick a database, then answer from that database.

	## Tasks
	1. Rewrite the user prompt so the query is precise.
	2. Choose the database name.
	3. If the prompt involves money, append: divide final amounts by 100
	4. Always append: Answer in Chinese

	## Databases
	- budget: financial budget questions. Append the money hint when amounts appear.

	## Output
	{{.formatting}}

	## Input
` + defend.UserInputPrefix + `{{.input}}` + defend.UserInputSuffix
	FilePath = "filePath"
	FileName = "fileName"
	FileId   = "fileId"
	System   = "system"

	_defaultFileQuery = defend.DataHandlingInstruction + `
	## History
` + defend.HistoryPrefix + `{{.history}}` + defend.HistorySuffix + `

	## Role
	File analysis assistant. Use the user question and files mentioned in history.

	## Tasks
	1. Understand the question and check history.
	2. Pick the one related file.
	3. Write a short English system role for this question. Downstream answer must be Chinese.

	## Output
	{{.formatting}}

	## Input
` + defend.UserInputPrefix + `{{.input}}` + defend.UserInputSuffix
)

var (
	_outputParser = outputparser.NewStructured([]outputparser.ResponseSchema{
		{
			Name:        _destinations,
			Description: "name of the prompt to use or \"DEFAULT\"",
		},
		{
			Name:        langchain.Input,
			Description: "rewritten user prompt",
		},
	})

	_defaultFileOutputParser = outputparser.NewStructured([]outputparser.ResponseSchema{
		{
			Name:        FilePath,
			Description: "path of the matching file",
		}, {
			Name:        FileName,
			Description: "file name",
		}, {
			Name:        FileId,
			Description: `file id; output "1" if missing`,
		}, {
			Name:        System,
			Description: "system role and job for this question",
		},
	})
)
