package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"aiworkc/pkg/langchain/sqlguard"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	WhitelistFileExt        = "file_ext"
	WhitelistSQLTable       = "sql_table"
	WhitelistSQLBlockedCol  = "sql_blocked_col"
	WhitelistSQLBlockedFunc = "sql_blocked_func"
	WhitelistAnalysisDep    = "analysis_dep"
	WhitelistAnalysisUser   = "analysis_user"
)

type Whitelist struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Kind          string             `bson:"kind" json:"kind"`
	Key           string             `bson:"key" json:"key"`
	Magic         string             `bson:"magic,omitempty" json:"magic,omitempty"`
	Columns       []string           `bson:"columns,omitempty" json:"columns,omitempty"`
	TenantColumn  string             `bson:"tenantColumn,omitempty" json:"tenantColumn,omitempty"`
	ScaleDivisors map[string]int     `bson:"scaleDivisors,omitempty" json:"scaleDivisors,omitempty"`
	Enabled       bool               `bson:"enabled" json:"enabled"`
	CreateAt      int64              `bson:"createAt,omitempty" json:"createAt,omitempty"`
	UpdateAt      int64              `bson:"updateAt,omitempty" json:"updateAt,omitempty"`
}

type WhitelistModel interface {
	SeedDefaults(ctx context.Context) error
	FindFileExt(ctx context.Context, ext string) (*Whitelist, error)
	List(ctx context.Context, kind string) ([]*Whitelist, error)
	Upsert(ctx context.Context, data *Whitelist) error
	Delete(ctx context.Context, kind, key string) error
	SQLPolicy(ctx context.Context) (sqlguard.Policy, error)
	Enabled(ctx context.Context, kind, key string) (bool, error)
}

type defaultWhitelistModel struct {
	col *mongo.Collection
}

func NewWhitelistModel(db *mongo.Database) WhitelistModel {
	col := db.Collection("whitelist")
	_, _ = col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "kind", Value: 1}, {Key: "key", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return &defaultWhitelistModel{col: col}
}

func (m *defaultWhitelistModel) SeedDefaults(ctx context.Context) error {
	now := time.Now().Unix()
	for _, row := range defaultWhitelistRows() {
		doc := bson.M{
			"kind":          row.Kind,
			"key":           row.Key,
			"magic":         row.Magic,
			"columns":       row.Columns,
			"tenantColumn":  row.TenantColumn,
			"scaleDivisors": row.ScaleDivisors,
			"enabled":       true,
			"createAt":      now,
			"updateAt":      now,
		}
		_, err := m.col.UpdateOne(ctx, bson.M{"kind": row.Kind, "key": row.Key}, bson.M{
			"$setOnInsert": doc,
		}, options.Update().SetUpsert(true))
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *defaultWhitelistModel) FindFileExt(ctx context.Context, ext string) (*Whitelist, error) {
	ext = normalizeFileExt(ext)
	if ext == "" {
		return nil, mongo.ErrNoDocuments
	}
	var row Whitelist
	err := m.col.FindOne(ctx, bson.M{
		"kind":    WhitelistFileExt,
		"key":     ext,
		"enabled": true,
	}).Decode(&row)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (m *defaultWhitelistModel) Enabled(ctx context.Context, kind, key string) (bool, error) {
	kind = strings.TrimSpace(kind)
	key = strings.ToLower(strings.TrimSpace(key))
	if kind == "" || key == "" {
		return false, nil
	}
	var row Whitelist
	err := m.col.FindOne(ctx, bson.M{
		"kind":    kind,
		"key":     key,
		"enabled": true,
	}).Decode(&row)
	switch err {
	case nil:
		return true, nil
	case mongo.ErrNoDocuments:
		return false, nil
	default:
		return false, err
	}
}

func (m *defaultWhitelistModel) List(ctx context.Context, kind string) ([]*Whitelist, error) {
	filter := bson.M{}
	if k := strings.TrimSpace(kind); k != "" {
		filter["kind"] = k
	}
	var rows []*Whitelist
	if err := entityList(ctx, m.col, filter, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (m *defaultWhitelistModel) Upsert(ctx context.Context, data *Whitelist) error {
	if data == nil {
		return errors.New("whitelist is required")
	}
	normalizeWhitelist(data)
	now := time.Now().Unix()
	data.UpdateAt = now
	if data.CreateAt == 0 {
		data.CreateAt = now
	}
	return entityUpdateOrInsert(ctx, m.col, bson.M{"kind": data.Kind, "key": data.Key}, bson.M{
		"$set": bson.M{
			"magic":         data.Magic,
			"columns":       data.Columns,
			"tenantColumn":  data.TenantColumn,
			"scaleDivisors": data.ScaleDivisors,
			"enabled":       data.Enabled,
			"updateAt":      now,
		},
		"$setOnInsert": bson.M{
			"kind":     data.Kind,
			"key":      data.Key,
			"createAt": now,
		},
	})
}

func (m *defaultWhitelistModel) Delete(ctx context.Context, kind, key string) error {
	kind = strings.TrimSpace(kind)
	if kind == WhitelistFileExt {
		key = normalizeFileExt(key)
	} else {
		key = strings.ToLower(strings.TrimSpace(key))
	}
	_, err := m.col.DeleteOne(ctx, bson.M{"kind": kind, "key": key})
	return err
}

func (m *defaultWhitelistModel) SQLPolicy(ctx context.Context) (sqlguard.Policy, error) {
	rows, err := m.List(ctx, WhitelistSQLTable)
	if err != nil {
		return sqlguard.Policy{}, err
	}
	blockedCols, err := m.List(ctx, WhitelistSQLBlockedCol)
	if err != nil {
		return sqlguard.Policy{}, err
	}
	blockedFuncs, err := m.List(ctx, WhitelistSQLBlockedFunc)
	if err != nil {
		return sqlguard.Policy{}, err
	}
	return BuildSQLPolicy(rows, blockedCols, blockedFuncs), nil
}

func BuildSQLPolicy(tables, blockedCols, blockedFuncs []*Whitelist) sqlguard.Policy {
	p := sqlguard.DefaultPolicy()
	if len(tables) > 0 {
		p.AllowedTables = map[string]struct{}{}
		p.AllowedColumns = map[string][]string{}
		p.TenantColumns = map[string]string{}
		p.ScaleDivisors = map[string]int{}
		for _, row := range tables {
			if row == nil || !row.Enabled {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(row.Key))
			if key == "" {
				continue
			}
			p.AllowedTables[key] = struct{}{}
			p.AllowedColumns[key] = append([]string(nil), row.Columns...)
			if col := strings.TrimSpace(row.TenantColumn); col != "" {
				p.TenantColumns[key] = col
			}
			for col, n := range row.ScaleDivisors {
				p.ScaleDivisors[strings.ToLower(strings.TrimSpace(col))] = n
			}
		}
	}
	if len(blockedCols) > 0 {
		p.BlockedColumns = map[string]struct{}{}
		for _, row := range blockedCols {
			if row == nil || !row.Enabled {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(row.Key))
			if key != "" {
				p.BlockedColumns[key] = struct{}{}
			}
		}
	}
	if len(blockedFuncs) > 0 {
		p.BlockedFuncs = map[string]struct{}{}
		for _, row := range blockedFuncs {
			if row == nil || !row.Enabled {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(row.Key))
			if key != "" {
				p.BlockedFuncs[key] = struct{}{}
			}
		}
	}
	return p
}

func defaultWhitelistRows() []*Whitelist {
	budgetCols := []string{
		"budget_id", "budget_no", "title", "occurred_time", "finish_time",
		"amount", "amount_bat", "tras", "user", "status", "day",
	}
	rows := []*Whitelist{
		{Kind: WhitelistFileExt, Key: ".pdf", Magic: "pdf"},
		{Kind: WhitelistFileExt, Key: ".png", Magic: "png"},
		{Kind: WhitelistFileExt, Key: ".jpg", Magic: "jpeg"},
		{Kind: WhitelistFileExt, Key: ".jpeg", Magic: "jpeg"},
		{Kind: WhitelistFileExt, Key: ".txt", Magic: "text"},
		{Kind: WhitelistFileExt, Key: ".csv", Magic: "text"},
		{
			Kind:          WhitelistSQLTable,
			Key:           "budget",
			Columns:       budgetCols,
			TenantColumn:  "user",
			ScaleDivisors: map[string]int{"amount": 100, "amount_bat": 100},
		},
	}
	for _, col := range []string{
		"id", "password", "passwd", "token", "secret", "phone", "mobile", "email",
		"id_card", "create_at", "update_at", "access_key", "api_key",
	} {
		rows = append(rows, &Whitelist{Kind: WhitelistSQLBlockedCol, Key: col})
	}
	for _, fn := range []string{
		"sleep", "benchmark", "load_file", "outfile", "dumpfile", "updatexml", "extractvalue",
	} {
		rows = append(rows, &Whitelist{Kind: WhitelistSQLBlockedFunc, Key: fn})
	}
	return rows
}

func normalizeWhitelist(data *Whitelist) {
	data.Kind = strings.TrimSpace(data.Kind)
	if data.Kind == WhitelistFileExt {
		data.Key = normalizeFileExt(data.Key)
		data.Magic = strings.ToLower(strings.TrimSpace(data.Magic))
	} else {
		data.Key = strings.ToLower(strings.TrimSpace(data.Key))
	}
}

func normalizeFileExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}
