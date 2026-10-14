package audit

import (
	"context"
	"fmt"

	"gitee.com/dn-jinmin/tlog"
)

func Event(ctx context.Context, name string, kv ...any) {
	if ctx == nil {
		ctx = context.Background()
	}
	tlog.InfoCtx(ctx, "audit", fmt.Sprintf("%s %v", name, kv))
}
