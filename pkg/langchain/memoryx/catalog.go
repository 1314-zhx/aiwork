package memoryx

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	LoadModeKey      = "_memoryLoadMode"
	LoadModeL0       = "l0"
	LoadModeCatalog  = "catalog"
	maxCatalogRunes  = 80
	expandTokenRunes = 2
)

var catalogStop = map[string]struct{}{
	"的": {}, "了": {}, "吗": {}, "呢": {}, "啊": {}, "吧": {},
	"我": {}, "你": {}, "是": {}, "在": {}, "有": {}, "和": {},
	"就": {}, "都": {}, "也": {}, "不": {}, "要": {}, "请": {},
	"帮": {}, "一下": {}, "这个": {}, "那个": {}, "什么": {},
}

func splitCatalogBody(raw string) (catalog, body string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	var catalogLine, bodyStart string
	lines := strings.Split(raw, "\n")
	rest := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		upper := strings.ToUpper(trim)
		switch {
		case catalogLine == "" && strings.HasPrefix(upper, "CATALOG:"):
			catalogLine = strings.TrimSpace(trim[len("CATALOG:"):])
		case strings.HasPrefix(upper, "SUMMARY:") || strings.HasPrefix(upper, "MEMORY:"):
			idx := strings.Index(trim, ":")
			if idx >= 0 {
				bodyStart = strings.TrimSpace(trim[idx+1:])
			}
		default:
			rest = append(rest, line)
		}
	}
	body = strings.TrimSpace(strings.Join(rest, "\n"))
	if bodyStart != "" {
		if body == "" {
			body = bodyStart
		} else {
			body = strings.TrimSpace(bodyStart + "\n" + body)
		}
	}
	if catalogLine == "" {
		catalogLine = firstLine(raw)
	}
	if body == "" {
		body = raw
	}
	return clipRunes(catalogLine, maxCatalogRunes), body
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func clipRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

func catalogRelevant(query, catalog string) bool {
	query = strings.TrimSpace(query)
	catalog = strings.TrimSpace(catalog)
	if query == "" || catalog == "" {
		return false
	}
	for _, token := range catalogTokens(query) {
		if strings.Contains(catalog, token) {
			return true
		}
	}
	return false
}

func catalogTokens(query string) []string {
	var tokens []string
	runes := []rune(query)
	var latin strings.Builder
	flushLatin := func() {
		tok := strings.ToLower(strings.TrimSpace(latin.String()))
		latin.Reset()
		if tok == "" || utf8.RuneCountInString(tok) < expandTokenRunes {
			return
		}
		if _, stop := catalogStop[tok]; stop {
			return
		}
		tokens = append(tokens, tok)
	}
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if unicode.Is(unicode.Han, r) {
			flushLatin()
			if i+1 < len(runes) && unicode.Is(unicode.Han, runes[i+1]) {
				tokens = append(tokens, string(runes[i:i+2]))
			}
			continue
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			flushLatin()
			continue
		}
		latin.WriteRune(r)
	}
	flushLatin()
	return tokens
}

func loadModeFrom(inputs map[string]any) string {
	if inputs == nil {
		return LoadModeCatalog
	}
	if mode, ok := inputs[LoadModeKey].(string); ok && mode != "" {
		return mode
	}
	return LoadModeCatalog
}

func queryFrom(inputs map[string]any) string {
	if inputs == nil {
		return ""
	}
	if q, ok := inputs["input"].(string); ok {
		return q
	}
	return ""
}
