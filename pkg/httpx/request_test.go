package httpx

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLimitPrompt(t *testing.T) {
	if got := LimitPrompt(""); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got := LimitPrompt("你好"); got != "你好" {
		t.Fatalf("short: %q", got)
	}

	over := strings.Repeat("字", MaxPromptRunes+20)
	got := LimitPrompt(over)
	if n := utf8.RuneCountInString(got); n != MaxPromptRunes {
		t.Fatalf("truncated runes=%d want=%d", n, MaxPromptRunes)
	}
	if got != strings.Repeat("字", MaxPromptRunes) {
		t.Fatalf("truncated content mismatch")
	}
}
