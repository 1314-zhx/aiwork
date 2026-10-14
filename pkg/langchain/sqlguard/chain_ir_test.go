package sqlguard

import (
	"context"
	"strings"
	"testing"
)

func TestPrepareCompilesIRNotRawSQL(t *testing.T) {
	raw := `{"table":"budget","columns":["title","status"],"filters":[{"column":"status","op":"eq","value":1}],"intent":"进行中预算"}`
	c := NewChain(stubModel{answer: raw}, 50, nil, DefaultPolicy())
	ctx := WithTenant(context.Background(), Tenant{UserName: "alice"})
	prep, err := c.Prepare(ctx, map[string]any{
		"query":              "看进行中的预算",
		"table_names_to_use": []string{"budget"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prep.IR.Table != "budget" {
		t.Fatalf("ir=%+v", prep.IR)
	}
	if strings.Contains(prep.SQL, "alice") || strings.Contains(prep.SQL, "DROP") {
		t.Fatalf("sql=%s", prep.SQL)
	}
	if !strings.Contains(strings.ToUpper(prep.SQL), "FROM") {
		t.Fatalf("sql=%s", prep.SQL)
	}
	found := false
	for _, a := range prep.Args {
		if a == "alice" {
			found = true
		}
	}
	if !found {
		t.Fatalf("args=%v", prep.Args)
	}
}

func TestSecurePlanRejectsSQLShapedOutput(t *testing.T) {
	c := NewChain(stubModel{answer: "SELECT * FROM budget"}, 50, nil, DefaultPolicy())
	ctx := WithTenant(context.Background(), Tenant{IsSystem: true})
	_, err := c.Prepare(ctx, map[string]any{
		"query":              "预算",
		"table_names_to_use": []string{"budget"},
	})
	if err == nil {
		t.Fatal("expected invalid IR")
	}
}

func TestExecuteRejectsSQLWithoutIR(t *testing.T) {
	c := NewChain(stubModel{answer: "no"}, 50, nil, DefaultPolicy())
	ctx := WithTenant(context.Background(), Tenant{UserName: "alice"})
	_, err := c.Execute(ctx, &Prepared{
		SQL:    "SELECT title FROM budget WHERE id = 1 OR 1=1",
		Query:  "预算",
		Tables: []string{"budget"},
	})
	if err != ErrNeedQueryPlan {
		t.Fatalf("got %v", err)
	}
}
