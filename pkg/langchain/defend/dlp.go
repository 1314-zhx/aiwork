package defend

import (
	"regexp"
	"strings"
)

var (
	rePEM        = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]+PRIVATE KEY-----.*?-----END [A-Z ]+PRIVATE KEY-----`)
	reJWT        = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
	reAliyunSK   = regexp.MustCompile(`\bsk-[A-Za-z0-9]{16,}\b`)
	reAWSKey     = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	reBearer     = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/]+=*`)
	reCNMobile   = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	reCNIDCard   = regexp.MustCompile(`\b[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`)
)

// RedactSecrets masks common secrets and PII in outbound text.
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	out := rePEM.ReplaceAllString(s, "[REDACTED_PRIVATE_KEY]")
	out = reJWT.ReplaceAllString(out, "[REDACTED_JWT]")
	out = reAliyunSK.ReplaceAllString(out, "[REDACTED_API_KEY]")
	out = reAWSKey.ReplaceAllString(out, "[REDACTED_ACCESS_KEY]")
	out = reBearer.ReplaceAllString(out, "Bearer [REDACTED]")
	out = reCNIDCard.ReplaceAllString(out, "[REDACTED_ID]")
	out = reCNMobile.ReplaceAllString(out, "[REDACTED_MOBILE]")
	return out
}

// LooksLikePoison reports likely instruction-style content in retrieved or ingested text.
func LooksLikePoison(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}
	if len(detectInjection(s)) > 0 {
		return true
	}
	needles := []string{
		"ignore previous",
		"disregard previous",
		"new instructions",
		"you are now",
		"system prompt",
		"忽略之前",
		"无视以上",
		"你现在是",
		"忘记你的",
		"系统指令",
	}
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
