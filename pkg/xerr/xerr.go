/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package xerr

import (
	"fmt"
	"runtime"
	"strconv"
)

const callerSkipOffset = 3

type Cause interface {
	Cause() error
}

type withMessage struct {
	cause error
	msg   string
}

func newWithMessage(skip int, err error, msg string) error {
	if err == nil {
		return nil
	}

	var path string
	f, ok := getCallerFrame(skip)
	if ok {
		path = f.File + ":" + strconv.Itoa(f.Line)
	}

	return &withMessage{
		cause: err,
		msg:   path + msg,
	}
}

func WithMessage(err error, message string) error {
	return newWithMessage(1, err, message)
}

func WithMessagef(err error, format string, v ...any) error {
	return newWithMessage(1, err, fmt.Sprintf(format, v...))
}

func New(err error) error {
	return newWithMessage(1, err, "")
}

func (w *withMessage) Error() string {
	if w.cause == nil {
		return w.msg
	}
	return w.msg + ": " + w.cause.Error()
}

func (w *withMessage) Cause() error {
	return w.cause
}

func getCallerFrame(skip int) (frame runtime.Frame, ok bool) {
	pc := make([]uintptr, 1)
	numFrames := runtime.Callers(skip+callerSkipOffset, pc)
	if numFrames < 1 {
		return
	}

	frame, _ = runtime.CallersFrames(pc).Next()
	return frame, frame.PC != 0
}
