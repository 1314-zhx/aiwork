package analysis

import (
	"aiworkc/pkg/langchain/router"
	"strings"
)

func missingDB(partial map[string]string) error {
	return router.NewMissing(
		"db",
		[]string{"table", "columns"},
		[]string{"表名", "要查询字段"},
		partial,
		"请补充表名（目前仅支持 budget）以及要查询的字段。",
	)
}

func missingFile(partial map[string]string) error {
	return router.NewMissing(
		"file",
		[]string{"fileId", "fileName"},
		[]string{"文件ID", "文件名"},
		partial,
		"请补充文件ID或文件名。",
	)
}

func filePartial(query map[string]string, input string) map[string]string {
	partial := map[string]string{"input": input}
	if query == nil {
		return partial
	}
	for _, key := range []string{FileId, FileName, FilePath, System} {
		if v := strings.TrimSpace(query[key]); v != "" && v != "1" {
			partial[key] = v
		}
	}
	return partial
}

func fileIdentity(query map[string]string) (fileId, fileName, filePath string, ok bool) {
	if query == nil {
		return "", "", "", false
	}
	fileId = strings.TrimSpace(query[FileId])
	if fileId == "1" {
		fileId = ""
	}
	fileName = strings.TrimSpace(query[FileName])
	filePath = strings.TrimSpace(query[FilePath])
	ok = fileId != "" || fileName != "" || filePath != ""
	return
}

func queryPartial(query map[string]any, input string) map[string]string {
	partial := map[string]string{"input": strings.TrimSpace(input)}
	if query == nil {
		return partial
	}
	if q, ok := query["query"].(string); ok && strings.TrimSpace(q) != "" {
		partial["input"] = strings.TrimSpace(q)
	}
	switch tables := query["table_names_to_use"].(type) {
	case []string:
		if len(tables) > 0 && strings.TrimSpace(tables[0]) != "" {
			partial["table"] = strings.TrimSpace(tables[0])
		}
	}
	return partial
}

func MergePartial(old, neu map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range old {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	for k, v := range neu {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func InferDest(saved, supplement string, partial map[string]string) string {
	saved = strings.TrimSpace(saved)
	if saved == "db" || saved == "file" {
		return saved
	}
	blob := strings.ToLower(supplement)
	for k, v := range partial {
		blob += " " + strings.ToLower(k) + " " + strings.ToLower(v)
	}
	fileHit := hasFileSignal(blob)
	dbHit := hasDBSignal(blob)
	if fileHit && !dbHit {
		return "file"
	}
	if dbHit && !fileHit {
		return "db"
	}
	return ""
}

func MergeResumeInput(base, supplement string, partial map[string]string) string {
	var b strings.Builder
	base = strings.TrimSpace(base)
	if base != "" {
		b.WriteString(base)
	}
	for _, key := range []string{"table", "columns", FileId, FileName, FilePath} {
		v := strings.TrimSpace(partial[key])
		if v == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(v)
	}
	if s := strings.TrimSpace(supplement); s != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("用户补充：")
		b.WriteString(s)
	}
	return b.String()
}

func hasFileSignal(blob string) bool {
	for _, key := range []string{"fileid", "filename", "filepath", "文件id", "文件名", ".pdf", ".xlsx", ".xls", ".csv", ".doc"} {
		if strings.Contains(blob, key) {
			return true
		}
	}
	return strings.Contains(blob, "文件")
}

func hasDBSignal(blob string) bool {
	for _, key := range []string{"budget", "table", "columns", "表名", "字段", "表"} {
		if strings.Contains(blob, key) {
			return true
		}
	}
	return false
}
