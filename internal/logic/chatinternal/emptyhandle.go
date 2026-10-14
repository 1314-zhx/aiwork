package chatinternal

import (
	"context"

	"github.com/tmc/langchaingo/chains"
	"aiworkc/internal/logic/chatinternal/toolx"
	"aiworkc/internal/svc"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/tokencost"
)

type DefaultHandler struct {
	svc *svc.ServiceContext
	c   chains.Chain
}

func NewDefaultHandler(svc *svc.ServiceContext) *DefaultHandler {
	d := &DefaultHandler{svc: svc}
	d.c = chains.NewTransform(func(ctx context.Context, inputs map[string]any, _ ...chains.ChainCallOption) (map[string]any, error) {
		if err := tokencost.RequireDaily(ctx, tokencost.DailyEnterNeed(tokencost.RouterDailyNeed)); err != nil {
			return nil, err
		}
		tool := toolx.NewDefaultHandler(d.svc)
		input, _ := inputs[langchain.Input].(string)
		history, _ := inputs["history"].(string)
		out, err := tool.CallWithHistory(ctx, input, history)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			langchain.OutPut: out,
		}, nil
	}, []string{"history"}, []string{langchain.OutPut})
	return d
}

func (d *DefaultHandler) Name() string {
	return "default"
}

func (d *DefaultHandler) Description() string {
	return "Default handler for chitchat and memory questions."
}

func (d *DefaultHandler) Chains() chains.Chain {
	return d.c
}
