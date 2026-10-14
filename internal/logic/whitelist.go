package logic

import (
	"context"
	"errors"
	"strings"

	"aiworkc/internal/domain"
	"aiworkc/internal/model"
	"aiworkc/internal/svc"
)

type Whitelist interface {
	List(ctx context.Context, req *domain.WhitelistListReq) (*domain.WhitelistListResp, error)
	Upsert(ctx context.Context, req *domain.WhitelistItem) error
	Delete(ctx context.Context, req *domain.WhitelistItem) error
}

type whitelist struct {
	svc *svc.ServiceContext
}

func NewWhitelist(svc *svc.ServiceContext) Whitelist {
	return &whitelist{svc: svc}
}

func (l *whitelist) List(ctx context.Context, req *domain.WhitelistListReq) (*domain.WhitelistListResp, error) {
	if err := l.svc.Auth(ctx); err != nil {
		return nil, err
	}
	kind := ""
	if req != nil {
		kind = req.Kind
	}
	rows, err := l.svc.Whitelist.List(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.WhitelistItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toWhitelistItem(row))
	}
	return &domain.WhitelistListResp{List: out}, nil
}

func (l *whitelist) Upsert(ctx context.Context, req *domain.WhitelistItem) error {
	if err := l.svc.Auth(ctx); err != nil {
		return err
	}
	row, err := fromWhitelistItem(req)
	if err != nil {
		return err
	}
	return l.svc.Whitelist.Upsert(ctx, row)
}

func (l *whitelist) Delete(ctx context.Context, req *domain.WhitelistItem) error {
	if err := l.svc.Auth(ctx); err != nil {
		return err
	}
	if req == nil || strings.TrimSpace(req.Kind) == "" || strings.TrimSpace(req.Key) == "" {
		return errors.New("kind and key are required")
	}
	return l.svc.Whitelist.Delete(ctx, req.Kind, req.Key)
}

func toWhitelistItem(row *model.Whitelist) *domain.WhitelistItem {
	if row == nil {
		return nil
	}
	enabled := row.Enabled
	return &domain.WhitelistItem{
		Kind:          row.Kind,
		Key:           row.Key,
		Magic:         row.Magic,
		Columns:       row.Columns,
		TenantColumn:  row.TenantColumn,
		ScaleDivisors: row.ScaleDivisors,
		Enabled:       &enabled,
	}
}

func fromWhitelistItem(req *domain.WhitelistItem) (*model.Whitelist, error) {
	if req == nil {
		return nil, errors.New("whitelist is required")
	}
	kind := strings.TrimSpace(req.Kind)
	key := strings.TrimSpace(req.Key)
	if kind == "" || key == "" {
		return nil, errors.New("kind and key are required")
	}
	switch kind {
	case model.WhitelistFileExt, model.WhitelistSQLTable, model.WhitelistSQLBlockedCol, model.WhitelistSQLBlockedFunc,
		model.WhitelistAnalysisDep, model.WhitelistAnalysisUser:
	default:
		return nil, errors.New("unsupported whitelist kind")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if kind == model.WhitelistFileExt && strings.TrimSpace(req.Magic) == "" {
		return nil, errors.New("file_ext 需要填写 magic：pdf、png、jpeg、text、zip 或 any")
	}
	if kind == model.WhitelistSQLTable && len(req.Columns) == 0 {
		return nil, errors.New("sql_table 需要填写 columns")
	}
	return &model.Whitelist{
		Kind:          kind,
		Key:           key,
		Magic:         req.Magic,
		Columns:       req.Columns,
		TenantColumn:  req.TenantColumn,
		ScaleDivisors: req.ScaleDivisors,
		Enabled:       enabled,
	}, nil
}
