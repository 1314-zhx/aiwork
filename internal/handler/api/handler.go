package api

import (
	"gitee.com/dn-jinmin/tlog"
	"github.com/gin-gonic/gin"
	"aiworkc/internal/handler"
	"aiworkc/internal/middleware"
	"aiworkc/internal/svc"
	"aiworkc/pkg/httpx"
)

type Handler interface {
	InitRegister(*gin.Engine)
}

type handle struct {
	srv  *gin.Engine
	addr string
}

func NewHandle(svc *svc.ServiceContext) *handle {
	h := &handle{
		srv:  gin.Default(),
		addr: "0.0.0.0:8080",
	}
	if len(svc.Config.Addr) > 0 {
		h.addr = svc.Config.Addr
	}

	tlog.Init(
		tlog.WithLoggerWriter(tlog.NewLoggerWriter()),
		tlog.WithLabel(svc.Config.Tlog.Label),
		tlog.WithMode(svc.Config.Tlog.Mode),
	)

	h.srv.Use(middleware.NewLog().Handler, middleware.NewCsrf(svc.Config).Handler, middleware.NewRateLimit().Handler)
	httpx.SetErrorHandler(handler.ErrorHandler)

	handlers := initHandler(svc)
	for _, handler := range handlers {
		handler.InitRegister(h.srv)
	}

	return h
}

func (h *handle) Run() error {
	return h.srv.Run(h.addr)
}
