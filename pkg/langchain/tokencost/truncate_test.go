package tokencost

import (
	"strings"
	"testing"
)

func TestTruncateToTokens(t *testing.T) {
	short := "请假需提前一天"
	if TruncateToTokens("qwen-turbo", short, AnalysisTokenLimit) != short {
		t.Fatal("short text")
	}
	if TruncateToTokens("qwen-turbo", short, 0) != "" {
		t.Fatal("zero limit")
	}

	long := strings.Repeat("员工手册第九章考勤 ", 400)
	got := TruncateToTokens("qwen-turbo", long, 50)
	if got == "" || CountTokens("qwen-turbo", got) > 50 {
		t.Fatalf("got tokens=%d text=%q", CountTokens("qwen-turbo", got), got)
	}
	if CountTokens("qwen-turbo", long) <= 50 {
		t.Fatal("fixture too short")
	}
}
