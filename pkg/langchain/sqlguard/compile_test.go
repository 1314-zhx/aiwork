package sqlguard

import (
	"strings"
	"testing"
)

func TestCompileBindsValuesAndRejectsUnknown(t *testing.T) {
	p := DefaultPolicy()
	ir := QueryIR{
		Table:   "budget",
		Columns: []string{"title", "amount", "status"},
		Filters: []Filter{{Column: "status", Op: OpEq, Value: 1}},
		Limit:   20,
		Intent:  "进行中预算",
	}
	got, err := Compile(ir, p, CompileOpts{TopK: 100, Tenant: Tenant{UserName: "alice"}})
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(got.SQL)
	if strings.Contains(got.SQL, "1") && strings.Contains(got.SQL, "alice") {
		// limit 20 is expected in SQL; tenant/filter values must not be
	}
	if strings.Contains(got.SQL, "alice") {
		t.Fatalf("tenant value inlined: %s", got.SQL)
	}
	if !strings.Contains(sql, "where") || !strings.Contains(sql, "limit 20") {
		t.Fatalf("sql=%s", got.SQL)
	}
	if len(got.Args) < 2 {
		t.Fatalf("args=%v", got.Args)
	}
	if got.Args[0] != 1 {
		t.Fatalf("status arg=%v", got.Args[0])
	}
	foundAlice := false
	for _, a := range got.Args {
		if a == "alice" {
			foundAlice = true
		}
	}
	if !foundAlice {
		t.Fatalf("missing tenant arg: %v", got.Args)
	}
}

func TestCompileDoesNotInlineInjectionString(t *testing.T) {
	p := DefaultPolicy()
	evil := "'; DROP TABLE budget; --"
	got, err := Compile(QueryIR{
		Table:   "budget",
		Columns: []string{"title"},
		Filters: []Filter{{Column: "title", Op: OpEq, Value: evil}},
	}, p, CompileOpts{Tenant: Tenant{UserID: "u1"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.SQL, "DROP") || strings.Contains(got.SQL, evil) {
		t.Fatalf("value concatenated: %s", got.SQL)
	}
	if got.Args[0] != evil {
		t.Fatalf("arg=%v", got.Args[0])
	}
}

func TestCompileRejectsUnknownTableAndColumn(t *testing.T) {
	p := DefaultPolicy()
	if _, err := Compile(QueryIR{Table: "mysql", Columns: []string{"title"}}, p, CompileOpts{Tenant: Tenant{IsSystem: true}}); err != ErrUnknownTable {
		t.Fatalf("got %v", err)
	}
	if _, err := Compile(QueryIR{Table: "budget", Columns: []string{"password"}}, p, CompileOpts{Tenant: Tenant{IsSystem: true}}); err != ErrSensitiveCol {
		t.Fatalf("got %v", err)
	}
}

func TestCompileDropsModelTenantFilter(t *testing.T) {
	p := DefaultPolicy()
	got, err := Compile(QueryIR{
		Table:   "budget",
		Columns: []string{"title"},
		Filters: []Filter{{Column: "user", Op: OpEq, Value: "eve"}},
	}, p, CompileOpts{Tenant: Tenant{UserName: "alice"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got.Args {
		if a == "eve" {
			t.Fatal("model must not set tenant")
		}
	}
}

func TestCompileAggregateAndGroupBy(t *testing.T) {
	p := DefaultPolicy()
	got, err := Compile(QueryIR{
		Table:      "budget",
		Columns:    []string{"status"},
		Aggregates: []Aggregate{{Fn: FnSum, Column: "amount", Alias: "total"}},
		GroupBy:    []string{"status"},
	}, p, CompileOpts{Tenant: Tenant{IsSystem: true}, TopK: 50})
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(got.SQL)
	if !strings.Contains(sql, "SUM") || !strings.Contains(sql, "GROUP BY") {
		t.Fatalf("sql=%s", got.SQL)
	}
	if !strings.Contains(got.SQL, "/ 100") {
		t.Fatalf("amount should be scaled: %s", got.SQL)
	}
}

func TestCompileLikeEscapesWildcards(t *testing.T) {
	p := DefaultPolicy()
	got, err := Compile(QueryIR{
		Table:   "budget",
		Columns: []string{"title"},
		Filters: []Filter{{Column: "title", Op: OpLike, Value: "%"}},
	}, p, CompileOpts{Tenant: Tenant{UserID: "u1"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Args[0] != `%\%%` {
		t.Fatalf("like arg=%v sql=%s", got.Args[0], got.SQL)
	}
	if !strings.Contains(strings.ToUpper(got.SQL), "ESCAPE") {
		t.Fatalf("sql=%s", got.SQL)
	}
}

func TestCompileCapsOpenScanLimit(t *testing.T) {
	p := DefaultPolicy()
	got, err := Compile(QueryIR{
		Table:   "budget",
		Columns: []string{"title"},
		Limit:   100,
	}, p, CompileOpts{TopK: 100, Tenant: Tenant{UserName: "alice"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.SQL, "LIMIT 20") {
		t.Fatalf("open scan should cap at %d: %s", OpenScanMax, got.SQL)
	}
}

func TestCompileSystemUserStillGetsTenant(t *testing.T) {
	p := DefaultPolicy()
	got, err := Compile(QueryIR{
		Table:   "budget",
		Columns: []string{"title"},
		Filters: []Filter{{Column: "status", Op: OpEq, Value: 1}},
	}, p, CompileOpts{Tenant: Tenant{IsSystem: true, UserID: "admin1", UserName: "root"}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range got.Args {
		if a == "root" || a == "admin1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("system login must still bind tenant: sql=%s args=%v", got.SQL, got.Args)
	}
}

func TestParseIRFromMarkdown(t *testing.T) {
	raw := "```json\n{\"table\":\"budget\",\"columns\":[\"title\"],\"filters\":[{\"column\":\"status\",\"op\":\"eq\",\"value\":1}]}\n```"
	ir, err := ParseIR(raw)
	if err != nil || ir.Table != "budget" || ir.Columns[0] != "title" {
		t.Fatalf("%+v %v", ir, err)
	}
}

func TestExplainIRChinese(t *testing.T) {
	summary, tables, cols := ExplainIR(QueryIR{
		Table:   "budget",
		Columns: []string{"title", "amount"},
		Filters: []Filter{{Column: "status", Op: OpEq, Value: 1}},
		Intent:  "看预算标题",
		Limit:   10,
	}, 100)
	if tables[0] != "budget" || !containsAll(cols, "title", "amount") {
		t.Fatalf("%v %v", tables, cols)
	}
	for _, want := range []string{"预算", "标题", "金额", "状态", "10", "看预算标题"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("missing %q in %s", want, summary)
		}
	}
}
