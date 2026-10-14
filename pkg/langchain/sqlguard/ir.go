package sqlguard

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrInvalidIR = errors.New("query plan is invalid")

const (
	OpEq      = "eq"
	OpNeq     = "neq"
	OpGt      = "gt"
	OpGte     = "gte"
	OpLt      = "lt"
	OpLte     = "lte"
	OpIn      = "in"
	OpBetween = "between"
	OpLike    = "like"

	FnSum   = "sum"
	FnCount = "count"
	FnAvg   = "avg"
	FnMax   = "max"
	FnMin   = "min"

	maxLikeRunes = 64
	maxInValues  = 20
	maxFilters   = 16
	maxSelect    = 12
)

// QueryIR is the only thing the model may output: slots, never SQL.
type QueryIR struct {
	Table      string      `json:"table"`
	Columns    []string    `json:"columns,omitempty"`
	Aggregates []Aggregate `json:"aggregates,omitempty"`
	Filters    []Filter    `json:"filters,omitempty"`
	GroupBy    []string    `json:"groupBy,omitempty"`
	OrderBy    []Order     `json:"orderBy,omitempty"`
	Limit      int         `json:"limit,omitempty"`
	Intent     string      `json:"intent,omitempty"`
}

type Aggregate struct {
	Fn     string `json:"fn"`
	Column string `json:"column,omitempty"`
	Alias  string `json:"alias,omitempty"`
}

type Filter struct {
	Column string `json:"column"`
	Op     string `json:"op"`
	Value  any    `json:"value,omitempty"`
	Value2 any    `json:"value2,omitempty"`
}

type Order struct {
	Column string `json:"column"`
	Dir    string `json:"dir,omitempty"`
}

func (q QueryIR) Empty() bool {
	return strings.TrimSpace(q.Table) == ""
}

func ParseIR(raw string) (QueryIR, error) {
	raw = extractJSONObject(raw)
	if raw == "" {
		return QueryIR{}, ErrInvalidIR
	}
	var ir QueryIR
	if err := json.Unmarshal([]byte(raw), &ir); err != nil {
		return QueryIR{}, ErrInvalidIR
	}
	return normalizeIR(ir)
}

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func normalizeIR(ir QueryIR) (QueryIR, error) {
	ir.Table = strings.ToLower(strings.TrimSpace(ir.Table))
	ir.Intent = strings.TrimSpace(ir.Intent)
	if utf8.RuneCountInString(ir.Intent) > 200 {
		ir.Intent = string([]rune(ir.Intent)[:200])
	}
	ir.Columns = normalizeIdents(ir.Columns)
	ir.GroupBy = normalizeIdents(ir.GroupBy)
	aggs := make([]Aggregate, 0, len(ir.Aggregates))
	for _, a := range ir.Aggregates {
		a.Fn = strings.ToLower(strings.TrimSpace(a.Fn))
		a.Column = strings.ToLower(strings.TrimSpace(a.Column))
		a.Alias = strings.ToLower(strings.TrimSpace(a.Alias))
		if a.Fn == "" {
			continue
		}
		aggs = append(aggs, a)
	}
	ir.Aggregates = aggs
	filters := make([]Filter, 0, len(ir.Filters))
	for _, f := range ir.Filters {
		f.Column = strings.ToLower(strings.TrimSpace(f.Column))
		f.Op = strings.ToLower(strings.TrimSpace(f.Op))
		if f.Column == "" || f.Op == "" {
			continue
		}
		filters = append(filters, f)
	}
	ir.Filters = filters
	orders := make([]Order, 0, len(ir.OrderBy))
	for _, o := range ir.OrderBy {
		o.Column = strings.ToLower(strings.TrimSpace(o.Column))
		o.Dir = strings.ToLower(strings.TrimSpace(o.Dir))
		if o.Dir != "desc" {
			o.Dir = "asc"
		}
		if o.Column == "" {
			continue
		}
		orders = append(orders, o)
	}
	ir.OrderBy = orders
	if ir.Table == "" {
		return QueryIR{}, ErrInvalidIR
	}
	if len(ir.Columns) == 0 && len(ir.Aggregates) == 0 {
		return QueryIR{}, ErrInvalidIR
	}
	if len(ir.Filters) > maxFilters || len(ir.Columns)+len(ir.Aggregates) > maxSelect {
		return QueryIR{}, ErrInvalidIR
	}
	return ir, nil
}

func normalizeIdents(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
