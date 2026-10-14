package defend

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	KindUser      = "user"
	KindHistory   = "history"
	KindTool      = "tool"
	KindRAG       = "rag"
	KindSQLResult = "sqlresult"
	KindModel     = "model"

	MaxToolRunes      = 2000
	MaxRAGRunes       = 1500
	MaxSQLResultRunes = 2500
	MaxUserRunes      = 4000
)

// UntrustedConfig is for tool results / RAG / SQL rows: cap length and
// neutralize frame breakout, but do not strip business keywords.
func UntrustedConfig() SanitizeConfig {
	return SanitizeConfig{
		MaxLen:         MaxToolRunes,
		StripHTML:      true,
		StripCodeFence: false,
		StripControl:   false,
		NormalizeSpace: true,
	}
}

func NewNonce() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "fallbacknonce"
	}
	return hex.EncodeToString(b)
}

func Truncate(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes]) + "…"
}

func IsolateUser(raw, nonce string) string {
	return Wrap(KindUser, nonce, Truncate(raw, MaxUserRunes))
}

func IsolateHistory(raw, nonce string) string {
	cfg := SanitizeHistoryConfig()
	return Wrap(KindHistory, nonce, Sanitize(raw, cfg))
}

func IsolateTool(raw, nonce string) string {
	return Wrap(KindTool, nonce, Truncate(raw, MaxToolRunes))
}

func IsolateRAG(raw, nonce string) string {
	cfg := UntrustedConfig()
	cfg.MaxLen = MaxRAGRunes
	return Wrap(KindRAG, nonce, Sanitize(Truncate(raw, MaxRAGRunes), cfg))
}

func IsolateSQLResult(raw, nonce string) string {
	cfg := UntrustedConfig()
	cfg.MaxLen = MaxSQLResultRunes
	return Wrap(KindSQLResult, nonce, Sanitize(Truncate(RedactSecrets(raw), MaxSQLResultRunes), cfg))
}

func IsolateModel(raw, nonce string) string {
	cfg := SanitizeHistoryConfig()
	cfg.MaxLen = MaxToolRunes
	return Wrap(KindModel, nonce, Sanitize(Truncate(raw, MaxToolRunes), cfg))
}

// Wrap puts untrusted text in a nonce-bound frame so the model cannot treat
// it as instructions, and so the content cannot close the frame early.
func Wrap(kind, nonce, raw string) string {
	if nonce == "" {
		nonce = NewNonce()
	}
	kind = sanitizeToken(kind)
	nonce = sanitizeToken(nonce)
	cleaned := Sanitize(raw, UntrustedConfig())
	cleaned = strings.ReplaceAll(cleaned, nonce, "")
	cleaned = stripDelimiterTokens(cleaned)
	return fmt.Sprintf("\n<<<UNTRUSTED kind=%s nonce=%s>>>\n%s\n<<<END nonce=%s>>>\n", kind, nonce, cleaned, nonce)
}

func sanitizeToken(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	if s == "" {
		return "x"
	}
	return s
}
