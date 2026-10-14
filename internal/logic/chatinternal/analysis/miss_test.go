package analysis

import (
	"strings"
	"testing"
)

func TestInferDestFromSaved(t *testing.T) {
	if got := InferDest("file", "budget title", nil); got != "file" {
		t.Fatalf("saved dest should win, got %q", got)
	}
}

func TestInferDestFromSupplement(t *testing.T) {
	if got := InferDest("", "fileId: abc123", nil); got != "file" {
		t.Fatalf("file supplement: %q", got)
	}
	if got := InferDest("", "查 budget 的 title 和 amount", nil); got != "db" {
		t.Fatalf("db supplement: %q", got)
	}
	if got := InferDest("", "帮我看看", nil); got != "" {
		t.Fatalf("unknown: %q", got)
	}
}

func TestMergeResumeInputKeepsPartial(t *testing.T) {
	got := MergeResumeInput("分析这份表", "fileId xyz", map[string]string{
		"fileName": "预算.xlsx",
	})
	for _, part := range []string{"分析这份表", "fileName: 预算.xlsx", "用户补充：fileId xyz"} {
		if !strings.Contains(got, part) {
			t.Fatalf("merged=%q missing %q", got, part)
		}
	}
}

func TestFileIdentity(t *testing.T) {
	if _, _, _, ok := fileIdentity(map[string]string{FileId: "1", FileName: ""}); ok {
		t.Fatal("placeholder fileId is not identity")
	}
	if _, _, _, ok := fileIdentity(map[string]string{FileName: "a.xlsx"}); !ok {
		t.Fatal("fileName should count")
	}
}
