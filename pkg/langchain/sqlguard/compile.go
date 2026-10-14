package sqlguard

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type CompileOpts struct {
	TopK   int
	Tenant Tenant
}

type Compiled struct {
	SQL     string
	Args    []any
	Tables  []string
	Columns []string
}

var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

var sqlOps = map[string]string{
	OpEq:      "=",
	OpNeq:     "<>",
	OpGt:      ">",
	OpGte:     ">=",
	OpLt:      "<",
	OpLte:     "<=",
	OpIn:      "IN",
	OpBetween: "BETWEEN",
	OpLike:    "LIKE",
}

var sqlFns = map[string]string{
	FnSum:   "SUM",
	FnCount: "COUNT",
	FnAvg:   "AVG",
	FnMax:   "MAX",
	FnMin:   "MIN",
}

// Compile turns a QueryIR into parameterized SQL. Identifiers come from the
// whitelist; values are bound as ? and never concatenated.
func Compile(ir QueryIR, p Policy, opt CompileOpts) (*Compiled, error) {
	ir, err := normalizeIR(ir)
	if err != nil {
		return nil, err
	}
	if opt.TopK <= 0 {
		opt.TopK = DefaultTopK
	}
	table := ir.Table
	if !p.AllowTable(table) {
		return nil, ErrUnknownTable
	}
	tenantCol, hasTenant := p.TenantColumn(table)
	if hasTenant && !opt.Tenant.Skip() && len(opt.Tenant.Values()) == 0 {
		return nil, ErrNoTenant
	}

	limit := ir.Limit
	if limit <= 0 || limit > opt.TopK {
		limit = opt.TopK
	}

	groupBy := ir.GroupBy
	if len(ir.Aggregates) > 0 && len(groupBy) == 0 && len(ir.Columns) > 0 {
		groupBy = append([]string(nil), ir.Columns...)
	}

	type selectExpr struct {
		sql   string
		alias string
		label string
	}
	var selects []selectExpr
	seenAlias := map[string]struct{}{}

	addSelect := func(expr, alias, label string) error {
		if alias == "" {
			alias = label
		}
		if !identRe.MatchString(alias) {
			return ErrInvalidIR
		}
		if _, ok := seenAlias[alias]; ok {
			return nil
		}
		seenAlias[alias] = struct{}{}
		selects = append(selects, selectExpr{sql: expr, alias: alias, label: label})
		return nil
	}

	colsForGroup := groupBy
	if len(ir.Aggregates) == 0 {
		colsForGroup = ir.Columns
	}
	for _, col := range colsForGroup {
		if !p.AllowColumn(table, col) {
			return nil, ErrSensitiveCol
		}
		ident, err := quoteIdent(col)
		if err != nil {
			return nil, err
		}
		expr := scaleExpr(ident, p.ScaleDivisor(col))
		if err := addSelect(expr, col, col); err != nil {
			return nil, err
		}
	}
	for _, agg := range ir.Aggregates {
		fn, ok := sqlFns[agg.Fn]
		if !ok {
			return nil, ErrInvalidIR
		}
		var inner string
		label := agg.Fn
		if agg.Fn == FnCount && agg.Column == "" {
			inner = "*"
			label = "count"
		} else {
			if !p.AllowColumn(table, agg.Column) {
				return nil, ErrSensitiveCol
			}
			ident, err := quoteIdent(agg.Column)
			if err != nil {
				return nil, err
			}
			inner = scaleExpr(ident, p.ScaleDivisor(agg.Column))
			label = agg.Fn + "_" + agg.Column
		}
		alias := agg.Alias
		if alias == "" {
			alias = label
		}
		if err := addSelect(fmt.Sprintf("%s(%s)", fn, inner), alias, label); err != nil {
			return nil, err
		}
	}
	if len(selects) == 0 {
		return nil, ErrInvalidIR
	}

	tbl, err := quoteIdent(table)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("SELECT ")
	for i, s := range selects {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(s.sql)
		if s.alias != "" {
			b.WriteString(" AS ")
			quoted, err := quoteIdent(s.alias)
			if err != nil {
				return nil, err
			}
			b.WriteString(quoted)
		}
	}
	b.WriteString(" FROM ")
	b.WriteString(tbl)

	var args []any
	var wheres []string
	businessFilters := 0
	for _, f := range ir.Filters {
		if hasTenant && f.Column == tenantCol {
			continue
		}
		clause, vals, err := compileFilter(table, f, p)
		if err != nil {
			return nil, err
		}
		businessFilters++
		wheres = append(wheres, clause)
		args = append(args, vals...)
	}
	if businessFilters == 0 && limit > OpenScanMax {
		limit = OpenScanMax
	}
	if hasTenant && !opt.Tenant.Skip() {
		ident, err := quoteIdent(tenantCol)
		if err != nil {
			return nil, err
		}
		vals := opt.Tenant.Values()
		if len(vals) == 1 {
			wheres = append(wheres, ident+" = ?")
			args = append(args, vals[0])
		} else {
			placeholders := strings.TrimRight(strings.Repeat("?,", len(vals)), ",")
			wheres = append(wheres, ident+" IN ("+placeholders+")")
			for _, v := range vals {
				args = append(args, v)
			}
		}
	}
	if len(wheres) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(wheres, " AND "))
	}

	if len(ir.Aggregates) > 0 && len(groupBy) > 0 {
		b.WriteString(" GROUP BY ")
		for i, col := range groupBy {
			if !p.AllowColumn(table, col) {
				return nil, ErrSensitiveCol
			}
			ident, err := quoteIdent(col)
			if err != nil {
				return nil, err
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(ident)
		}
	}

	if len(ir.OrderBy) > 0 {
		b.WriteString(" ORDER BY ")
		for i, o := range ir.OrderBy {
			if _, ok := seenAlias[o.Column]; !ok && !p.AllowColumn(table, o.Column) {
				return nil, ErrSensitiveCol
			}
			ident, err := quoteIdent(o.Column)
			if err != nil {
				return nil, err
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(ident)
			if o.Dir == "desc" {
				b.WriteString(" DESC")
			} else {
				b.WriteString(" ASC")
			}
		}
	}

	b.WriteString(" LIMIT ")
	b.WriteString(strconv.Itoa(limit))

	labels := make([]string, 0, len(selects))
	for _, s := range selects {
		labels = append(labels, s.label)
	}
	return &Compiled{
		SQL:     b.String(),
		Args:    args,
		Tables:  []string{table},
		Columns: labels,
	}, nil
}

func compileFilter(table string, f Filter, p Policy) (string, []any, error) {
	if !p.AllowColumn(table, f.Column) {
		return "", nil, ErrSensitiveCol
	}
	op, ok := sqlOps[f.Op]
	if !ok {
		return "", nil, ErrInvalidIR
	}
	ident, err := quoteIdent(f.Column)
	if err != nil {
		return "", nil, err
	}
	switch f.Op {
	case OpIn:
		vals := flattenList(f.Value)
		if len(vals) == 0 || len(vals) > maxInValues {
			return "", nil, ErrInvalidIR
		}
		ph := strings.TrimRight(strings.Repeat("?,", len(vals)), ",")
		return ident + " IN (" + ph + ")", vals, nil
	case OpBetween:
		if f.Value == nil || f.Value2 == nil {
			return "", nil, ErrInvalidIR
		}
		return ident + " BETWEEN ? AND ?", []any{f.Value, f.Value2}, nil
	case OpLike:
		s, ok := f.Value.(string)
		if !ok {
			return "", nil, ErrInvalidIR
		}
		s = strings.TrimSpace(s)
		if s == "" || utf8.RuneCountInString(s) > maxLikeRunes {
			return "", nil, ErrInvalidIR
		}
		return ident + ` LIKE ? ESCAPE '\\'`, []any{"%" + escapeLike(s) + "%"}, nil
	default:
		if f.Value == nil {
			return "", nil, ErrInvalidIR
		}
		return ident + " " + op + " ?", []any{f.Value}, nil
	}
}

func quoteIdent(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !identRe.MatchString(name) {
		return "", ErrInvalidIR
	}
	return "`" + name + "`", nil
}

func escapeLike(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\\' || r == '%' || r == '_' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func scaleExpr(ident string, div int) string {
	if div <= 1 {
		return ident
	}
	return fmt.Sprintf("(%s / %d)", ident, div)
}

func flattenList(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			if item == nil {
				continue
			}
			out = append(out, item)
		}
		return out
	default:
		return []any{v}
	}
}
