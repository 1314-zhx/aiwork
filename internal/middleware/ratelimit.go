package middleware

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"aiworkc/pkg/httpx"
	"aiworkc/pkg/ratelimit"
	"aiworkc/pkg/token"
)

type RateLimit struct {
	lim *ratelimit.Limiter
}

func NewRateLimit() *RateLimit {
	return &RateLimit{lim: ratelimit.New(60, time.Minute)}
}

func (m *RateLimit) Handler(ctx *gin.Context) {
	key := ctx.ClientIP()
	if uid := token.GetUId(ctx.Request.Context()); uid != "" {
		key = uid
	}
	if !m.lim.Allow(key) {
		httpx.FailWithErr(ctx, errRateLimited)
		ctx.Abort()
		return
	}
	ctx.Next()
}

var errRateLimited = errors.New("请求过于频繁，请稍后再试")
