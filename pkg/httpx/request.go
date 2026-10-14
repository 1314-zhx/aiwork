package httpx

import (
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const MaxPromptRunes = 500

// LimitPrompt keeps at most MaxPromptRunes. Voice is capped after transcription;
// the client is expected to stop recording at 30s.
func LimitPrompt(prompt string) string {
	if utf8.RuneCountInString(prompt) <= MaxPromptRunes {
		return prompt
	}
	return string([]rune(prompt)[:MaxPromptRunes])
}

func BindAndValidate(ctx *gin.Context, v any) error {
	if err := ctx.ShouldBind(v); err != nil {
		return err
	}

	if err := ctx.ShouldBindUri(v); err != nil {
		return err
	}

	return nil
}
