/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package chatinternal

import (
	"context"
	"strings"

	"aiworkc/internal/domain"
	"aiworkc/internal/logic/chatinternal/analysis"
	"aiworkc/internal/svc"
	"aiworkc/pkg/langchain/router"
	"aiworkc/pkg/langchain/sqlguard"
	"aiworkc/pkg/langchain/tokencost"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/schema"
)

type AnalysisHandle struct {
	svc   *svc.ServiceContext
	inner *router.Router
}

func NewAnalysisHandle(svc *svc.ServiceContext, memory schema.Memory) *AnalysisHandle {
	return &AnalysisHandle{
		svc: svc,
		inner: router.NewRouter(svc.LLMs, []router.Handler{
			analysis.NewDB(svc),
			analysis.NewFile(svc),
		}, router.WithMemory(memory)),
	}
}

func (a *AnalysisHandle) Name() string {
	return "analysis"
}

func (a *AnalysisHandle) Description() string {
	return "use when you need to analyze certain data"
}

func (a *AnalysisHandle) Chains() chains.Chain {
	return chains.NewTransform(func(ctx context.Context, inputs map[string]any, opts ...chains.ChainCallOption) (map[string]any, error) {
		if err := analysis.RequireAnalysisAccess(ctx, a.svc); err != nil {
			return nil, err
		}
		tokencost.RaiseLimit(ctx, tokencost.AnalysisTokenLimit)
		if err := tokencost.RequireDaily(ctx, tokencost.DailyEnterNeed(tokencost.AnalysisTokenLimit)); err != nil {
			return nil, err
		}
		return a.inner.Call(ctx, inputs, opts...)
	}, nil, nil)
}

// Resume runs the already-chosen analysis handler. It does not call the
// level-1 or level-2 routers.
func (a *AnalysisHandle) Resume(ctx context.Context, dest string, inputs map[string]any, opts ...chains.ChainCallOption) (map[string]any, error) {
	if err := analysis.RequireAnalysisAccess(ctx, a.svc); err != nil {
		return nil, err
	}
	tokencost.RaiseLimit(ctx, tokencost.AnalysisTokenLimit)
	if err := tokencost.RequireDaily(ctx, tokencost.DailyEnterNeed(tokencost.AnalysisTokenLimit)); err != nil {
		return nil, err
	}
	h := a.inner.Handler(strings.TrimSpace(dest))
	if h == nil {
		next, _ := inputs["input"].(string)
		return nil, router.NewUnknownIntent([]string{"db", "file"}, dest, next)
	}
	return chains.Call(ctx, h.Chains(), inputs, opts...)
}

func (a *AnalysisHandle) ExecuteSQLDraft(ctx context.Context, draft sqlguard.Draft) (*domain.ChatResp, error) {
	if err := analysis.RequireAnalysisAccess(ctx, a.svc); err != nil {
		return nil, err
	}
	tokencost.RaiseLimit(ctx, tokencost.AnalysisTokenLimit)
	if err := tokencost.RequireDaily(ctx, tokencost.DailyEnterNeed(tokencost.AnalysisTokenLimit)); err != nil {
		return nil, err
	}
	return analysis.NewDB(a.svc).ExecuteConfirmed(ctx, draft)
}

func (a *AnalysisHandle) ReviseSQLDraft(ctx context.Context, draft sqlguard.Draft, feedback string) (*domain.ChatResp, error) {
	if err := analysis.RequireAnalysisAccess(ctx, a.svc); err != nil {
		return nil, err
	}
	tokencost.RaiseLimit(ctx, tokencost.AnalysisTokenLimit)
	if err := tokencost.RequireDaily(ctx, tokencost.DailyEnterNeed(tokencost.AnalysisTokenLimit)); err != nil {
		return nil, err
	}
	return analysis.NewDB(a.svc).ReviseDraft(ctx, draft, feedback)
}
