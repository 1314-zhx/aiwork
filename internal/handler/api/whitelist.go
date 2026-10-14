package api

import (
	"github.com/gin-gonic/gin"

	"aiworkc/internal/domain"
	"aiworkc/internal/logic"
	"aiworkc/internal/svc"
	"aiworkc/pkg/httpx"
)

type Whitelist struct {
	svcCtx    *svc.ServiceContext
	whitelist logic.Whitelist
}

func NewWhitelist(svcCtx *svc.ServiceContext, whitelist logic.Whitelist) *Whitelist {
	return &Whitelist{svcCtx: svcCtx, whitelist: whitelist}
}

func (h *Whitelist) InitRegister(engine *gin.Engine) {
	g := engine.Group("v1/whitelist", h.svcCtx.Jwt.Handler)
	g.GET("", h.List)
	g.POST("", h.Upsert)
	g.DELETE("", h.Delete)
}

func (h *Whitelist) List(ctx *gin.Context) {
	var req domain.WhitelistListReq
	if err := httpx.BindAndValidate(ctx, &req); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	res, err := h.whitelist.List(ctx.Request.Context(), &req)
	if err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	httpx.OkWithData(ctx, res)
}

func (h *Whitelist) Upsert(ctx *gin.Context) {
	var req domain.WhitelistItem
	if err := httpx.BindAndValidate(ctx, &req); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	if err := h.whitelist.Upsert(ctx.Request.Context(), &req); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	httpx.Ok(ctx)
}

func (h *Whitelist) Delete(ctx *gin.Context) {
	var req domain.WhitelistItem
	if err := httpx.BindAndValidate(ctx, &req); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	if err := h.whitelist.Delete(ctx.Request.Context(), &req); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	httpx.Ok(ctx)
}
