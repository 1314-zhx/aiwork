package analysis

import (
	"context"
	"strings"
	"testing"

	"aiworkc/pkg/langchain/tokencost"
)

func TestFitFileBodyCapsToAnalysisLimit(t *testing.T) {
	ctx := tokencost.WithBudget(context.Background(), tokencost.NewBudget(tokencost.AnalysisTokenLimit))
	body := strings.Repeat("预算科目金额说明。", 2000)
	got, truncated, err := fitFileBody(ctx, "analyze the spreadsheet", "汇总待发生金额", body)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("long body should truncate")
	}
	if tokencost.CountTokens(fileEmbedModel, got) > tokencost.AnalysisTokenLimit {
		t.Fatalf("body tokens=%d", tokencost.CountTokens(fileEmbedModel, got))
	}
}

func TestFitFileBodyRejectsEmpty(t *testing.T) {
	if _, _, err := fitFileBody(context.Background(), "sys", "q", "   "); err == nil {
		t.Fatal("empty")
	}
}
