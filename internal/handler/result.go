/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package handler

import (
	"gitee.com/dn-jinmin/tlog"
	"github.com/gin-gonic/gin"
	"aiworkc/pkg/xerr"
)

func ErrorHandler(ctx *gin.Context, err error) (int, error) {
	var e error
	if ce, ok := err.(xerr.Cause); ok {
		e = ce.Cause()
		tlog.ErrorCtx(ctx.Request.Context(), "err", err.Error())
	} else {
		e = err
		tlog.ErrorCtx(ctx.Request.Context(), "err", err.Error())
	}

	return 500, e
}
