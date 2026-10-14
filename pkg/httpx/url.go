package httpx

import "strings"

func JoinHostPath(host, p string) string {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	p = strings.TrimLeft(strings.TrimSpace(p), "/")
	if host == "" {
		return "/" + p
	}
	if p == "" {
		return host
	}
	return host + "/" + p
}
