package httpx

import (
	"errors"
	"sync"

	"github.com/gin-gonic/gin"
)

const (
	ERROR   = 500
	SUCCESS = 200
)

var (
	ERRORMSG   = errors.New("fail")
	SUCCESSMSG = "success"
)

var (
	errorHandler func(ctx *gin.Context, err error) (int, error)
	errorLock    sync.RWMutex
	okHandler    func(ctx *gin.Context, data any) any
	okLock       sync.RWMutex
)

var NULL = map[string]interface{}{}

func SetOkHandler(handler func(ctx *gin.Context, data any) any) {
	okHandler = handler
}

func SetErrorHandler(handler func(ctx *gin.Context, err error) (int, error)) {
	errorHandler = handler
}

type Response struct {
	Code int         `json:"code"`
	Data interface{} `json:"data"`
	Msg  string      `json:"msg"`
}

func Result(ctx *gin.Context, code int, data interface{}, msg string) {
	ctx.JSON(200, &Response{
		code,
		data,
		msg,
	})
}

func Ok(ctx *gin.Context) {
	OkWithData(ctx, NULL)
}

func OkWithData(ctx *gin.Context, data interface{}) {
	okLock.RLock()
	handler := okHandler
	okLock.RUnlock()
	if handler != nil {
		data = handler(ctx, data)
	}
	Result(ctx, SUCCESS, data, SUCCESSMSG)
}

func Fail(ctx *gin.Context) {
	FailWithErr(ctx, ERRORMSG)
}

func FailWithErr(ctx *gin.Context, err error) {
	errorLock.RLock()
	handler := errorHandler
	errorLock.RUnlock()

	code := ERROR
	if handler != nil {
		code, err = handler(ctx, err)
	}
	Result(ctx, code, NULL, err.Error())
}

// StreamChunk is one SSE payload. Seq is monotonic in this connection for
// client-side sort and dedup; it is not used to resume a dropped stream.
type StreamChunk struct {
	Seq     int    `json:"seq"`
	Content string `json:"content"`
	Done    bool   `json:"done"`
	Error   string `json:"error,omitempty"`
}

// SSE stream response helpers

func SSE(ctx *gin.Context) {
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("Transfer-Encoding", "chunked")
	ctx.Header("Access-Control-Allow-Origin", "*")
	ctx.Header("X-Accel-Buffering", "no")
}

func SSEWrite(ctx *gin.Context, data string) {
	ctx.SSEvent("", map[string]interface{}{
		"content": data,
	})
	ctx.Writer.Flush()
}

// SSEWriteWithSeq writes a content chunk with a connection-local seq.
func SSEWriteWithSeq(ctx *gin.Context, seq int, data string) {
	ctx.SSEvent("", StreamChunk{
		Seq:     seq,
		Content: data,
		Done:    false,
	})
	ctx.Writer.Flush()
}

func SSEDone(ctx *gin.Context) {
	ctx.SSEvent("", map[string]interface{}{
		"done": true,
	})
	ctx.Writer.Flush()
}

// SSEDoneWithSeq writes the done flag with the last seq of this connection.
func SSEDoneWithSeq(ctx *gin.Context, seq int) {
	ctx.SSEvent("", StreamChunk{
		Seq:  seq,
		Done: true,
	})
	ctx.Writer.Flush()
}

func SSEError(ctx *gin.Context, err error) {
	ctx.SSEvent("error", map[string]interface{}{
		"error": err.Error(),
	})
	ctx.Writer.Flush()
}
