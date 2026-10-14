package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"aiworkc/internal/config"
)

type Csrf struct {
	origins map[string]struct{}
}

func NewCsrf(cfg config.Config) *Csrf {
	m := &Csrf{origins: map[string]struct{}{}}
	for _, o := range cfg.CORS.AllowOrigins {
		o = strings.TrimSpace(o)
		if o == "" || o == "*" {
			continue
		}
		m.origins[o] = struct{}{}
	}
	return m
}

func (m *Csrf) Handler(ctx *gin.Context) {
	origin := strings.TrimSpace(ctx.GetHeader("Origin"))
	if origin != "" {
		if _, ok := m.origins[origin]; ok {
			ctx.Header("Access-Control-Allow-Origin", origin)
			ctx.Header("Access-Control-Allow-Credentials", "true")
			ctx.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			ctx.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			ctx.Header("Vary", "Origin")
		}
	}
	if ctx.Request.Method == http.MethodOptions {
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
	ctx.Next()
}
