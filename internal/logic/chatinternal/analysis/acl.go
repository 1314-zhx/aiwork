package analysis

import (
	"context"
	"errors"
	"strings"

	"aiworkc/internal/model"
	"aiworkc/internal/svc"
	"aiworkc/pkg/token"
)

// ErrAnalysisForbidden is returned before extract / SQL work when the caller
// is not on the analysis department or named-user allowlist.
var ErrAnalysisForbidden = errors.New("无数据分析权限")

type analysisACL struct {
	Enabled func(ctx context.Context, kind, key string) (bool, error)
}

type analysisIdentity struct {
	UserID   string
	DepID    string
	IsSystem bool
}

func RequireAnalysisAccess(ctx context.Context, svcCtx *svc.ServiceContext) error {
	if svcCtx == nil || svcCtx.Whitelist == nil {
		return ErrAnalysisForbidden
	}
	uid := strings.TrimSpace(token.GetUId(ctx))
	if uid == "" {
		return errors.New("请先登录后再执行数据分析")
	}
	ident := analysisIdentity{UserID: uid}
	if svcCtx.UserModel != nil {
		user, err := svcCtx.UserModel.FindOne(ctx, uid)
		if err != nil || user == nil {
			return ErrAnalysisForbidden
		}
		ident.IsSystem = user.IsSystem
	}
	if svcCtx.DepartmentUserModel != nil {
		if dep, err := svcCtx.DepartmentUserModel.FindByUserId(ctx, uid); err == nil && dep != nil {
			ident.DepID = strings.TrimSpace(dep.DepId)
		}
	}
	return checkAnalysisACL(ctx, analysisACL{Enabled: svcCtx.Whitelist.Enabled}, ident)
}

func checkAnalysisACL(ctx context.Context, store analysisACL, ident analysisIdentity) error {
	if store.Enabled == nil {
		return ErrAnalysisForbidden
	}
	if ident.IsSystem {
		return nil
	}
	uid := strings.TrimSpace(ident.UserID)
	if uid == "" {
		return errors.New("请先登录后再执行数据分析")
	}
	ok, err := store.Enabled(ctx, model.WhitelistAnalysisUser, uid)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	depID := strings.TrimSpace(ident.DepID)
	if depID == "" {
		return ErrAnalysisForbidden
	}
	ok, err = store.Enabled(ctx, model.WhitelistAnalysisDep, depID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAnalysisForbidden
	}
	return nil
}
