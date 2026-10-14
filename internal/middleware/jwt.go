package middleware

import (
	"github.com/gin-gonic/gin"
	"aiworkc/pkg/httpx"
	"aiworkc/pkg/token"
)

type Jwt struct {
	tokenParser *token.Parse
}

func NewJwt(secret string) *Jwt {
	return &Jwt{
		tokenParser: token.NewTokenParse(secret),
	}
}

func (m *Jwt) Handler(ctx *gin.Context) {
	r, err := m.tokenParser.ParseWithContext(ctx.Request)
	if err != nil {
		httpx.FailWithErr(ctx, err)
		ctx.Abort()
		return
	}

	ctx.Request = r
	ctx.Next()
}
