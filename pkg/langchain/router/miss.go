package router

import (
	"errors"
	"sort"
	"strings"
)

// NotHandlesError is ErrNotHandles with the reason, missing fields, and any
// partial extraction. Used when a router without EmptyHandler cannot pick a
// destination, or the chosen handler is missing required fields.
type NotHandlesError struct {
	Reason      string
	Hint        string
	Destination string
	NextInputs  string
	Accept      []string
	Need        []string
	Partial     map[string]string
}

func (e *NotHandlesError) Error() string {
	if e == nil {
		return ErrNotHandles.Error()
	}
	if r := strings.TrimSpace(e.Reason); r != "" {
		return r
	}
	return ErrNotHandles.Error()
}

func (e *NotHandlesError) Unwrap() error {
	return ErrNotHandles
}

func AsNotHandles(err error) (*NotHandlesError, bool) {
	if err == nil {
		return nil, false
	}
	var n *NotHandlesError
	if errors.As(err, &n) && n != nil {
		return n, true
	}
	if errors.Is(err, ErrNotHandles) {
		return &NotHandlesError{Reason: ErrNotHandles.Error()}, true
	}
	return nil, false
}

func ReasonMissing(items ...string) string {
	cleaned := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		cleaned = append(cleaned, item)
	}
	if len(cleaned) == 0 {
		return "无法判断分析意图。"
	}
	return "用户输入不包含" + strings.Join(cleaned, "，") + "等必须内容。"
}

func NewUnknownIntent(accept []string, dest, nextInputs string) *NotHandlesError {
	rawDest := strings.TrimSpace(dest)
	dest = normalizeDest(dest, accept)
	nextInputs = strings.TrimSpace(nextInputs)
	return &NotHandlesError{
		Reason:      ReasonMissing("文件ID", "文件名", "表名", "要查询字段"),
		Hint:        "无法判断是查询数据库还是分析文件。查库请给出表名（目前仅 budget）和要查询的字段；分析文件请给出文件ID或文件名。",
		Destination: dest,
		NextInputs:  nextInputs,
		Accept:      copyStrings(accept),
		Need:        []string{"fileId", "fileName", "table", "columns"},
		Partial: copyStringMap(map[string]string{
			"destinations": rawDest,
			"next_inputs":  nextInputs,
		}),
	}
}

func NewMissing(dest string, need, needZh []string, partial map[string]string, hint string) *NotHandlesError {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		hint = "请补充必填字段后再试。"
	}
	partial = copyStringMap(partial)
	next := ""
	if partial != nil {
		next = strings.TrimSpace(partial["input"])
		if next == "" {
			next = strings.TrimSpace(partial["next_inputs"])
		}
	}
	return &NotHandlesError{
		Reason:      ReasonMissing(needZh...),
		Hint:        hint,
		Destination: strings.TrimSpace(dest),
		NextInputs:  next,
		Need:        copyStrings(need),
		Partial:     partial,
	}
}

func normalizeDest(dest string, accept []string) string {
	dest = strings.TrimSpace(dest)
	if dest == "" || dest == Empty {
		return ""
	}
	for _, name := range accept {
		if dest == name {
			return dest
		}
	}
	return ""
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
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

func handlerNames(handlers map[string]Handler) []string {
	names := make([]string, 0, len(handlers))
	for name := range handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func inputString(inputs map[string]any) string {
	if inputs == nil {
		return ""
	}
	if v, ok := inputs[_input].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
