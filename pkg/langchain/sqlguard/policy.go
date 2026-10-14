package sqlguard

import (
	"errors"
	"strings"
)

var (
	ErrNotSelect     = errors.New("only SELECT is allowed")
	ErrUnknownTable  = errors.New("table is not in the whitelist")
	ErrSensitiveCol  = errors.New("query selects a blocked sensitive column")
	ErrUnsafeSQL     = errors.New("sql failed the security check")
	ErrSyntax        = errors.New("sql syntax is invalid")
	ErrEmptySQL      = errors.New("sql is empty")
	ErrNeedQueryPlan = errors.New("缺少查询计划，拒绝执行")
)

// Policy is the input whitelist: tables the model may touch, columns it may not.
type Policy struct {
	AllowedTables   map[string]struct{}
	AllowedColumns  map[string][]string
	BlockedColumns  map[string]struct{}
	BlockedFuncs    map[string]struct{}
	TenantColumns   map[string]string
	ScaleDivisors   map[string]int // SELECT/聚合时金额列除以该数，例如分→元
}

func DefaultPolicy() Policy {
	return Policy{
		AllowedTables: map[string]struct{}{
			"budget": {},
		},
		AllowedColumns: map[string][]string{
			"budget": {
				"budget_id", "budget_no", "title", "occurred_time", "finish_time",
				"amount", "amount_bat", "tras", "user", "status", "day",
			},
		},
		BlockedColumns: map[string]struct{}{
			"id": {}, "password": {}, "passwd": {}, "token": {}, "secret": {},
			"phone": {}, "mobile": {}, "email": {}, "id_card": {},
			"create_at": {}, "update_at": {}, "access_key": {}, "api_key": {},
		},
		BlockedFuncs: map[string]struct{}{
			"sleep": {}, "benchmark": {}, "load_file": {}, "outfile": {},
			"dumpfile": {}, "updatexml": {}, "extractvalue": {},
		},
		TenantColumns: map[string]string{
			"budget": "user",
		},
		ScaleDivisors: map[string]int{
			"amount":     100,
			"amount_bat": 100,
		},
	}
}

func (p Policy) AllowColumn(table, col string) bool {
	table = strings.ToLower(strings.TrimSpace(table))
	col = strings.ToLower(strings.TrimSpace(col))
	if col == "" || p.IsBlockedColumn(col) {
		return false
	}
	for _, c := range p.AllowedColumns[table] {
		if c == col {
			return true
		}
	}
	return false
}

func (p Policy) ScaleDivisor(col string) int {
	if p.ScaleDivisors == nil {
		return 0
	}
	return p.ScaleDivisors[strings.ToLower(strings.TrimSpace(col))]
}

func (p Policy) SchemaPrompt(tables []string) string {
	var b strings.Builder
	seen := map[string]struct{}{}
	list := tables
	if len(list) == 0 {
		for t := range p.AllowedTables {
			list = append(list, t)
		}
	}
	for _, t := range list {
		t = strings.ToLower(strings.TrimSpace(t))
		if _, ok := p.AllowedTables[t]; !ok {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		cols := p.AllowedColumns[t]
		b.WriteString("- ")
		b.WriteString(t)
		b.WriteString(" columns: ")
		b.WriteString(strings.Join(cols, ", "))
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func (p Policy) TenantColumn(table string) (string, bool) {
	col, ok := p.TenantColumns[strings.ToLower(strings.TrimSpace(table))]
	return col, ok && col != ""
}

func (p Policy) AllowTable(name string) bool {
	_, ok := p.AllowedTables[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func (p Policy) IsBlockedColumn(name string) bool {
	_, ok := p.BlockedColumns[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func (p Policy) IsBlockedFunc(name string) bool {
	_, ok := p.BlockedFuncs[strings.ToLower(strings.TrimSpace(name))]
	return ok
}
