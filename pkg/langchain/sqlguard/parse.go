package sqlguard

import (
	"fmt"
	"strings"

	"github.com/xwb1989/sqlparser"
)

type Inspected struct {
	Stmt    sqlparser.Statement
	SQL     string
	Tables  []string
	Columns []string
	HasStar bool
}

func Parse(sql string) (*Inspected, error) {
	sql = strings.TrimSpace(sql)
	sql = strings.TrimSuffix(sql, ";")
	if sql == "" {
		return nil, ErrEmptySQL
	}
	if strings.Count(sql, ";") > 0 {
		return nil, fmt.Errorf("%w: multiple statements", ErrSyntax)
	}
	stmt, err := sqlparser.Parse(sql)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSyntax, err)
	}
	if _, ok := stmt.(*sqlparser.Union); ok {
		return nil, fmt.Errorf("%w: UNION is not allowed", ErrUnsafeSQL)
	}
	sel, ok := stmt.(*sqlparser.Select)
	if !ok {
		return nil, ErrNotSelect
	}
	if len(sel.Comments) > 0 || sel.Lock != "" {
		return nil, ErrUnsafeSQL
	}

	ins := &Inspected{Stmt: stmt, SQL: sqlparser.String(sel)}
	if err := sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		switch n := node.(type) {
		case *sqlparser.AliasedTableExpr:
			if tn, ok := n.Expr.(sqlparser.TableName); ok {
				if q := strings.ToLower(tn.Qualifier.String()); q != "" {
					return false, fmt.Errorf("%w: qualified table", ErrUnknownTable)
				}
				ins.Tables = append(ins.Tables, strings.ToLower(tn.Name.String()))
			}
		case *sqlparser.FuncExpr:
			if DefaultPolicy().IsBlockedFunc(n.Name.String()) {
				return false, ErrUnsafeSQL
			}
		}
		return true, nil
	}, sel); err != nil {
		return nil, err
	}
	for _, expr := range sel.SelectExprs {
		if _, ok := expr.(*sqlparser.StarExpr); ok {
			ins.HasStar = true
		}
		_ = sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
			if col, ok := node.(*sqlparser.ColName); ok {
				ins.Columns = append(ins.Columns, strings.ToLower(col.Name.String()))
			}
			return true, nil
		}, expr)
	}
	return ins, nil
}

func Check(sql string, p Policy, allowedFromInput []string) error {
	ins, err := Parse(sql)
	if err != nil {
		return err
	}
	allow := map[string]struct{}{}
	for t := range p.AllowedTables {
		allow[t] = struct{}{}
	}
	if len(allowedFromInput) > 0 {
		narrow := map[string]struct{}{}
		for _, t := range allowedFromInput {
			name := strings.ToLower(strings.TrimSpace(t))
			if _, ok := allow[name]; !ok {
				return ErrUnknownTable
			}
			narrow[name] = struct{}{}
		}
		allow = narrow
	}
	if len(ins.Tables) == 0 {
		return ErrUnknownTable
	}
	for _, t := range ins.Tables {
		if _, ok := allow[t]; !ok {
			return ErrUnknownTable
		}
	}
	if ins.HasStar {
		return ErrSensitiveCol
	}
	allowCols := map[string]struct{}{}
	for t := range allow {
		for _, col := range p.AllowedColumns[t] {
			allowCols[strings.ToLower(col)] = struct{}{}
		}
	}
	for _, col := range ins.Columns {
		if p.IsBlockedColumn(col) {
			return ErrSensitiveCol
		}
		if len(allowCols) > 0 {
			if _, ok := allowCols[col]; !ok {
				return ErrSensitiveCol
			}
		}
	}
	return nil
}

func StripSensitive(sql string, p Policy) (string, error) {
	ins, err := Parse(sql)
	if err != nil {
		return "", err
	}
	sel := ins.Stmt.(*sqlparser.Select)
	if ins.HasStar {
		var exprs sqlparser.SelectExprs
		seen := map[string]struct{}{}
		for _, t := range ins.Tables {
			for _, col := range p.AllowedColumns[t] {
				if _, ok := seen[col]; ok {
					continue
				}
				seen[col] = struct{}{}
				exprs = append(exprs, &sqlparser.AliasedExpr{
					Expr: &sqlparser.ColName{Name: sqlparser.NewColIdent(col)},
				})
			}
		}
		if len(exprs) == 0 {
			return "", ErrSensitiveCol
		}
		sel.SelectExprs = exprs
		return sqlparser.String(sel), nil
	}

	var exprs sqlparser.SelectExprs
	for _, expr := range sel.SelectExprs {
		aliased, ok := expr.(*sqlparser.AliasedExpr)
		if !ok {
			exprs = append(exprs, expr)
			continue
		}
		col, ok := aliased.Expr.(*sqlparser.ColName)
		if ok && p.IsBlockedColumn(col.Name.String()) {
			continue
		}
		exprs = append(exprs, expr)
	}
	if len(exprs) == 0 {
		return "", ErrSensitiveCol
	}
	sel.SelectExprs = exprs
	return sqlparser.String(sel), nil
}
