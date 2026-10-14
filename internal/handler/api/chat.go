package api

import (
	"github.com/gin-gonic/gin"

	"aiworkc/internal/domain"
	"aiworkc/internal/logic"
	"aiworkc/internal/svc"
	"aiworkc/pkg/httpx"
	"aiworkc/pkg/token"
)

type Chat struct {
	svcCtx *svc.ServiceContext
	chat   logic.Chat
}

func NewChat(svcCtx *svc.ServiceContext, chat logic.Chat) *Chat {
	return &Chat{
		svcCtx: svcCtx,
		chat:   chat,
	}
}

func (h *Chat) InitRegister(engine *gin.Engine) {
	g := engine.Group("v1/chat", h.svcCtx.Jwt.Handler)
	g.POST("/", h.Chat)
	g.POST("/stream", h.ChatStream)
	g.POST("/confirm", h.Confirm)
	g.POST("/summary", h.Summary) // 会话页时间表单总结，不走意图路由
	g.GET("/sync", h.Sync)        // 离线消息同步
}

func (h *Chat) Chat(ctx *gin.Context) {
	var req domain.ChatReq
	if err := httpx.BindAndValidate(ctx, &req); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	req.Prompts = httpx.LimitPrompt(req.Prompts)

	res, err := h.chat.AIChat(ctx.Request.Context(), &req)
	if err != nil {
		httpx.FailWithErr(ctx, err)
	} else {
		httpx.OkWithData(ctx, res)
	}
}

func (h *Chat) ChatStream(ctx *gin.Context) {
	var req domain.ChatReq
	if err := httpx.BindAndValidate(ctx, &req); err != nil {
		httpx.SSEError(ctx, err)
		return
	}
	req.Prompts = httpx.LimitPrompt(req.Prompts)

	httpx.SSE(ctx)

	// seq is per-connection only (sort / dedup). Streams are not resumed after disconnect.
	seq := 0

	err := h.chat.AIChatStream(ctx.Request.Context(), &req, func(content string) error {
		seq++
		httpx.SSEWriteWithSeq(ctx, seq, content)
		return nil
	})
	if err != nil {
		httpx.SSEError(ctx, err)
		return
	}

	httpx.SSEDoneWithSeq(ctx, seq)
}

func (h *Chat) Confirm(ctx *gin.Context) {
	var req domain.ChatConfirmReq
	if err := httpx.BindAndValidate(ctx, &req); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}

	res, err := h.chat.Confirm(ctx.Request.Context(), &req)
	if err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	httpx.OkWithData(ctx, res)
}

// Summary 当前登录用户与指定对方的私聊记录总结。
func (h *Chat) Summary(ctx *gin.Context) {
	var req domain.ChatLogSummaryReq
	if err := httpx.BindAndValidate(ctx, &req); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	req.Goal = httpx.LimitPrompt(req.Goal)

	res, err := h.chat.SummarizeLog(ctx.Request.Context(), &req)
	if err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	httpx.OkWithData(ctx, res)
}

// Sync 离线消息同步
func (h *Chat) Sync(ctx *gin.Context) {
	var req domain.ChatSyncReq
	if err := httpx.BindAndValidate(ctx, &req); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}

	uid := token.GetUId(ctx.Request.Context())

	res, err := h.chat.Sync(ctx.Request.Context(), &req, uid)
	if err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}

	httpx.OkWithData(ctx, res)
}
