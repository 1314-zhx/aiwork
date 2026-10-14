package sqlguard

import (
	"context"
	"strings"
	"unicode/utf8"

	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/tokencost"

	"github.com/tmc/langchaingo/llms"
)

const (
	maxSQLRunes = 1500
	sqlPrefix   = "\n━━━ ━━━ SQL START ━━━ ━━━\n"
	sqlSuffix   = "\n━━━ ━━━ SQL END ━━━ ━━━\n"
)

const reviewTemplate = defend.DataHandlingInstruction + `You are a SQL security checker. Output exactly one MySQL SELECT, no markdown.

Allowed tables: budget
Blocked columns (must not appear in SELECT): id, password, token, secret, phone, email, create_at, update_at
Blocked: comments, multiple statements, INSERT/UPDATE/DELETE/DDL, SLEEP/BENCHMARK/LOAD_FILE, other databases.

Rules:
1. If the SQL is already a safe SELECT on an allowed table, echo it unchanged.
2. Only rewrite security issues (blocked columns, SELECT *, comments, unsafe functions).
3. Do not change business filters, aggregations, grouping, sorting, or intent.
4. If it cannot be made safe without changing business meaning, output REJECT.

User question:
` + defend.UserInputPrefix + `{{.input}}` + defend.UserInputSuffix + `
SQL:
` + sqlPrefix + `{{.sql}}` + sqlSuffix

func Review(ctx context.Context, model llms.Model, userQuestion, sql string) (string, error) {
	if model == nil {
		return sql, nil
	}
	userQuestion = defend.Sanitize(cutRunes(userQuestion, maxSQLRunes), defend.DefaultSanitizeConfig())
	sql = defend.Sanitize(cutRunes(strings.TrimSpace(sql), maxSQLRunes), defend.SanitizeHistoryConfig())
	prompt := strings.ReplaceAll(reviewTemplate, "{{.input}}", userQuestion)
	prompt = strings.ReplaceAll(prompt, "{{.sql}}", sql)

	out, err := llms.GenerateFromSinglePrompt(ctx, model, prompt, llms.WithMaxTokens(tokencost.SQLCheckMaxTokens))
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "```sql")
	out = strings.TrimPrefix(out, "```")
	out = strings.TrimSuffix(out, "```")
	out = strings.TrimSpace(out)
	if out == "" || strings.EqualFold(out, "REJECT") {
		return "", ErrUnsafeSQL
	}
	return out, nil
}

func cutRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
