package sqlguard

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

func TestParseSelect(t *testing.T) {
	ins, err := Parse("SELECT title, amount FROM budget WHERE status = 1")
	if err != nil {
		t.Fatal(err)
	}
	if ins.HasStar || len(ins.Tables) != 1 || ins.Tables[0] != "budget" {
		t.Fatalf("%+v", ins)
	}
}

func TestParseRejectsInsertAndMultiStmt(t *testing.T) {
	if _, err := Parse("DELETE FROM budget"); !errors.Is(err, ErrNotSelect) {
		t.Fatalf("got %v", err)
	}
	if _, err := Parse("SELECT 1; SELECT 2"); err == nil {
		t.Fatal("expected multi-statement reject")
	}
}

func TestCheckWhitelistAndBlockedColumn(t *testing.T) {
	p := DefaultPolicy()
	if err := Check("SELECT title FROM budget", p, []string{"budget"}); err != nil {
		t.Fatal(err)
	}
	if err := Check("SELECT title FROM admin", p, []string{"budget"}); !errors.Is(err, ErrUnknownTable) {
		t.Fatalf("got %v", err)
	}
	if err := Check("SELECT id, title FROM budget", p, []string{"budget"}); !errors.Is(err, ErrSensitiveCol) {
		t.Fatalf("got %v", err)
	}
	if err := Check("SELECT * FROM budget", p, []string{"budget"}); !errors.Is(err, ErrSensitiveCol) {
		t.Fatalf("got %v", err)
	}
}

func TestStripSensitiveRemovesIDAndStar(t *testing.T) {
	p := DefaultPolicy()
	out, err := StripSensitive("SELECT id, title FROM budget", p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(out), "id") && !strings.Contains(strings.ToLower(out), "budget_id") {
		t.Fatalf("id still present: %s", out)
	}
	star, err := StripSensitive("SELECT * FROM budget", p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(star, "*") {
		t.Fatalf("star remains: %s", star)
	}
	if err := Check(star, p, []string{"budget"}); err != nil {
		t.Fatal(err)
	}
}

func TestWhereIDIsAllowed(t *testing.T) {
	p := DefaultPolicy()
	if err := Check("SELECT title FROM budget WHERE id = 1", p, []string{"budget"}); err != nil {
		t.Fatal(err)
	}
}

type stubModel struct{ answer string }

func (s stubModel) GenerateContent(_ context.Context, _ []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: s.answer}}}, nil
}
func (s stubModel) Call(_ context.Context, _ string, _ ...llms.CallOption) (string, error) {
	return s.answer, nil
}

func TestReviewReject(t *testing.T) {
	_, err := Review(context.Background(), stubModel{answer: "REJECT"}, "分析预算", "SELECT * FROM mysql.user")
	if !errors.Is(err, ErrUnsafeSQL) {
		t.Fatalf("got %v", err)
	}
}

func TestReviewEchoesSafeSQL(t *testing.T) {
	sql := "SELECT title FROM budget"
	got, err := Review(context.Background(), stubModel{answer: sql}, "分析预算", sql)
	if err != nil {
		t.Fatal(err)
	}
	if got != sql {
		t.Fatalf("got %s", got)
	}
}
