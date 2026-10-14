package sqlguard

import (
	"errors"
	"strconv"

	"github.com/xwb1989/sqlparser"
)

var ErrNoTenant = errors.New("缺少租户条件，拒绝查询")

type RewriteOpts struct {
	TopK   int
	Tables []string
	Tenant Tenant
}

// Rewrite turns model SQL into the statement that may be executed:
// strip sensitive columns, cap LIMIT, inject tenant predicate, then re-check.
func Rewrite(sql string, p Policy, opt RewriteOpts) (string, error) {
	if opt.TopK <= 0 {
		opt.TopK = DefaultTopK
	}
	stripped, err := StripSensitive(sql, p)
	if err != nil {
		return "", err
	}
	limited, err := ForceLimit(stripped, opt.TopK)
	if err != nil {
		return "", err
	}
	withTenant, err := InjectTenant(limited, p, opt.Tenant)
	if err != nil {
		return "", err
	}
	if err := Check(withTenant, p, opt.Tables); err != nil {
		return "", err
	}
	return withTenant, nil
}

func ForceLimit(sql string, topK int) (string, error) {
	if topK <= 0 {
		topK = DefaultTopK
	}
	ins, err := Parse(sql)
	if err != nil {
		return "", err
	}
	sel := ins.Stmt.(*sqlparser.Select)
	want := strconv.Itoa(topK)
	if sel.Limit == nil {
		sel.Limit = &sqlparser.Limit{Rowcount: sqlparser.NewIntVal([]byte(want))}
		return sqlparser.String(sel), nil
	}
	n := limitValue(sel.Limit.Rowcount)
	if n <= 0 || n > topK {
		sel.Limit.Rowcount = sqlparser.NewIntVal([]byte(want))
	}
	return sqlparser.String(sel), nil
}

func limitValue(expr sqlparser.Expr) int {
	v, ok := expr.(*sqlparser.SQLVal)
	if !ok || v.Type != sqlparser.IntVal {
		return 0
	}
	n, err := strconv.Atoi(string(v.Val))
	if err != nil {
		return 0
	}
	return n
}

func InjectTenant(sql string, p Policy, tenant Tenant) (string, error) {
	ins, err := Parse(sql)
	if err != nil {
		return "", err
	}
	if tenant.Skip() {
		return sqlparser.String(ins.Stmt), nil
	}
	sel := ins.Stmt.(*sqlparser.Select)
	pred, err := tenantPredicate(ins.Tables, p, tenant)
	if err != nil {
		return "", err
	}
	if pred == nil {
		return sqlparser.String(sel), nil
	}
	if sel.Where == nil {
		sel.Where = &sqlparser.Where{Type: sqlparser.WhereStr, Expr: pred}
	} else {
		sel.Where.Expr = &sqlparser.AndExpr{Left: sel.Where.Expr, Right: pred}
	}
	return sqlparser.String(sel), nil
}

func tenantPredicate(tables []string, p Policy, tenant Tenant) (sqlparser.Expr, error) {
	vals := tenant.Values()
	var pred sqlparser.Expr
	need := false
	for _, table := range tables {
		col, ok := p.TenantColumn(table)
		if !ok {
			continue
		}
		need = true
		if len(vals) == 0 {
			return nil, ErrNoTenant
		}
		left := &sqlparser.ColName{Name: sqlparser.NewColIdent(col)}
		var cmp sqlparser.Expr
		if len(vals) == 1 {
			cmp = &sqlparser.ComparisonExpr{
				Operator: sqlparser.EqualStr,
				Left:     left,
				Right:    sqlparser.NewStrVal([]byte(vals[0])),
			}
		} else {
			tuple := sqlparser.ValTuple{}
			for _, v := range vals {
				tuple = append(tuple, sqlparser.NewStrVal([]byte(v)))
			}
			cmp = &sqlparser.ComparisonExpr{
				Operator: sqlparser.InStr,
				Left:     left,
				Right:    tuple,
			}
		}
		if pred == nil {
			pred = cmp
		} else {
			pred = &sqlparser.AndExpr{Left: pred, Right: cmp}
		}
	}
	if need && pred == nil {
		return nil, ErrNoTenant
	}
	return pred, nil
}
