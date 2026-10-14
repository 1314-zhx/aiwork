/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package callbackx

import (
	"context"
	"gitee.com/dn-jinmin/tlog"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms"
)

type TitTokenHandle struct {
	callbacks.SimpleHandler
	logger tlog.Logger
}

func NewTitTokenHandle(logger tlog.Logger) *TitTokenHandle {
	return &TitTokenHandle{
		SimpleHandler: callbacks.SimpleHandler{},
		logger:        logger,
	}
}

func (l *TitTokenHandle) HandleLLMGenerateContentEnd(ctx context.Context, res *llms.ContentResponse) {
	var count int
	for i, _ := range res.Choices {
		if v, ok := res.Choices[i].GenerationInfo["TotalTokens"]; ok {
			count += v.(int)
		}
	}

	if count == 0 {
		return
	}

	l.logger.InfofCtx(ctx, "TitTokenHandle:llm_generate_content_end", "count %d", count)
}
