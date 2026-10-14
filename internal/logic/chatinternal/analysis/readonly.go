package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"aiworkc/pkg/langchain/sqlguard"

	_ "github.com/go-sql-driver/mysql"
	"github.com/tmc/langchaingo/tools/sqldatabase"
)

const readOnlyMaxExecMS = 8000

var identName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type readOnlyMySQL struct {
	db      *sql.DB
	maxRows int
}

func newReadOnlyMySQL(dsn string) (sqldatabase.Engine, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	maxRows := sqlguard.DefaultTopK
	if maxRows <= 0 {
		maxRows = 100
	}
	return &readOnlyMySQL{db: db, maxRows: maxRows}, nil
}

func (m *readOnlyMySQL) Dialect() string { return "mysql" }

func (m *readOnlyMySQL) Close() error {
	if m == nil || m.db == nil {
		return nil
	}
	return m.db.Close()
}

func (m *readOnlyMySQL) Query(ctx context.Context, query string, args ...any) ([]string, [][]string, error) {
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()

	tx, txErr := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if txErr != nil {
		if _, err := conn.ExecContext(ctx, "SET SESSION transaction_read_only = ON"); err != nil {
			return nil, nil, err
		}
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("SET SESSION max_execution_time = %d", readOnlyMaxExecMS))
		return scanQuery(ctx, conn, m.maxRows, query, args...)
	}
	defer tx.Rollback()
	_, _ = tx.ExecContext(ctx, fmt.Sprintf("SET SESSION max_execution_time = %d", readOnlyMaxExecMS))
	cols, rows, err := scanTxQuery(ctx, tx, m.maxRows, query, args...)
	if err != nil {
		return nil, nil, err
	}
	_ = tx.Rollback()
	return cols, rows, nil
}

func (m *readOnlyMySQL) TableNames(ctx context.Context) ([]string, error) {
	_, result, err := m.Query(ctx, "SHOW TABLES")
	if err != nil {
		return nil, err
	}
	ret := make([]string, 0, len(result))
	for _, row := range result {
		if len(row) > 0 {
			ret = append(ret, row[0])
		}
	}
	return ret, nil
}

func (m *readOnlyMySQL) TableInfo(ctx context.Context, table string) (string, error) {
	table = strings.TrimSpace(table)
	if !identName.MatchString(table) {
		return "", sqldatabase.ErrTableNotFound
	}
	_, result, err := m.Query(ctx, "SHOW CREATE TABLE `"+table+"`")
	if err != nil {
		return "", err
	}
	if len(result) == 0 {
		return "", sqldatabase.ErrTableNotFound
	}
	if len(result[0]) < 2 {
		return "", sqldatabase.ErrInvalidResult
	}
	return result[0][1], nil
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func scanQuery(ctx context.Context, q queryer, maxRows int, query string, args ...any) ([]string, [][]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	return collectRows(rows, maxRows)
}

func scanTxQuery(ctx context.Context, tx *sql.Tx, maxRows int, query string, args ...any) ([]string, [][]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	return collectRows(rows, maxRows)
}

func collectRows(rows *sql.Rows, maxRows int) ([]string, [][]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	if maxRows <= 0 {
		maxRows = 100
	}
	results := make([][]string, 0)
	for rows.Next() {
		if len(results) >= maxRows {
			break
		}
		rowNullable := make([]sql.NullString, len(cols))
		rowPtrs := make([]any, len(cols))
		for i := range rowNullable {
			rowPtrs[i] = &rowNullable[i]
		}
		if err := rows.Scan(rowPtrs...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(cols))
		for i := range rowNullable {
			row[i] = rowNullable[i].String
		}
		results = append(results, row)
	}
	return cols, results, rows.Err()
}
