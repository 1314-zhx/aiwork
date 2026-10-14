package tokenretry

import "errors"

var (
	ErrNotFound = errors.New("重试票不存在或已使用")
	ErrExpired  = errors.New("重试票已过期，请重新提问")
	ErrOwner    = errors.New("重试票不属于当前用户")
)
