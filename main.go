package main

import (
	"context"
	"flag"
	"sync"

	"aiworkc/internal/config"
	"aiworkc/internal/domain"
	"aiworkc/internal/handler/api"
	"aiworkc/internal/handler/ws"
	"aiworkc/internal/svc"
	"aiworkc/pkg/conf"
)

type Serve interface {
	Run() error
}

const (
	Api = "api"

	// add other module
)

var (
	configFile = flag.String("f", "./etc/api.yaml", "the config file")
	modeType   = flag.String("m", "api", "server run mod")

	sw sync.WaitGroup
)

func main() {
	flag.Parse()

	var cfg config.Config
	conf.MustLoad(*configFile, &cfg)

	svc, err := svc.NewServiceContext(cfg)
	if err != nil {
		panic(err)
	}

	wsSrv := ws.NewWs(svc)
	svc.ApprovalPush = func(ctx context.Context, evt *domain.ApprovalEvent) error {
		return wsSrv.PushToUser(ctx, evt.ToUserId, evt)
	}

	sw.Add(1)
	go func() {
		defer sw.Done()
		srv := api.NewHandle(svc)
		srv.Run()
	}()
	sw.Add(1)
	go func() {
		defer sw.Done()
		wsSrv.Run()
	}()

	sw.Wait()
	//var srv Serve
	//switch *modeType {
	//case Api:
	//	srv = api.NewHandle(svc)
	//// add other module case
	//default:
	//	panic("请指定正确的服务")
	//}
	//
	//srv.Run()
}
