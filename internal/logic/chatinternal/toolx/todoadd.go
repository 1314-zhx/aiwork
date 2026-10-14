/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package toolx

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aiworkc/internal/domain"
	"aiworkc/internal/svc"
	"aiworkc/pkg/langchain/outputparserx"

	"github.com/tmc/langchaingo/callbacks"
)

type TodoAdd struct {
	svc          *svc.ServiceContext
	callback     callbacks.Handler
	outputparser outputparserx.Structured
}

func NewTodoAdd(svc *svc.ServiceContext) *TodoAdd {
	return &TodoAdd{
		svc:      svc,
		callback: svc.Callbacks,
		outputparser: outputparserx.NewStructured([]outputparserx.ResponseSchema{
			{
				Name:        "title",
				Description: "todo title",
			}, {
				Name:        "deadlineAt",
				Description: "deadline in natural language, e.g. \"2026-06-05 16:00\" or \"tomorrow 16:00\" or \"next Monday 09:00\". Return as string, the tool will parse it.",
				Type:        "string",
			}, {
				Name:        "desc",
				Description: "todo description",
			}, {
				Name:        "executeIds",
				Description: "list of participating users in the backlog. the data type is a set of string ids. none is empty",
				Type:        "[]string",
			},
		}),
	}
}

func (t *TodoAdd) Name() string {
	return "todo_add"
}

func (t *TodoAdd) ParameterSchema() any {
	return t.outputparser.JSONSchema()
}

func (t *TodoAdd) Description() string {
	template := `
	extract todo fields for user confirmation.
	do not write to database and do not claim the todo is saved.
	if fields cannot be parsed, return an error instead of a draft.
	Reply in Chinese.
` + t.outputparser.GetFormatInstructions()

	return template
}

// Call 只抽取待办字段。解析失败不返回草稿、不写库；成功则交给用户确认后由后端插入。
func (t *TodoAdd) Call(ctx context.Context, input string) (string, error) {
	if t.callback != nil {
		t.callback.HandleText(ctx, "todo add start : "+input)
	}

	data, err := t.outputparser.Parse(input)
	if err != nil {
		return "", err
	}

	fields, ok := data.(map[string]any)
	if !ok {
		return "", fmt.Errorf("todo fields parse failed")
	}

	if deadlineStr, ok := fields["deadlineAt"].(string); ok && deadlineStr != "" {
		deadlineTs, errParse := parseNaturalTime(deadlineStr)
		if errParse != nil {
			return "", fmt.Errorf("deadline time parsing failed: %w", errParse)
		}
		fields["deadlineAt"] = deadlineTs
	}

	draft, err := todoDraftFromFields(fields)
	if err != nil {
		return "", err
	}
	return ConfirmDraftAnswer(domain.ConfirmTodo, draft)
}

func todoDraftFromFields(fields map[string]any) (*domain.Todo, error) {
	b, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	var parsed domain.Todo
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(parsed.Title)
	if title == "" {
		return nil, fmt.Errorf("todo title parse failed")
	}
	return &domain.Todo{
		Title:      title,
		DeadlineAt: parsed.DeadlineAt,
		Desc:       parsed.Desc,
		ExecuteIds: parsed.ExecuteIds,
	}, nil
}

// parseNaturalTime 解析自然语言时间描述，返回 Unix 时间戳（秒）
// 支持格式：2026-06-05 16:00, tomorrow 16:00, next Monday 09:00 等
func parseNaturalTime(input string) (int64, error) {
	input = strings.TrimSpace(input)
	now := time.Now().Unix()
	loc := time.FixedZone("CST", 8*3600)
	ref := time.Now().In(loc)

	// 尝试直接解析标准格式: 2026-06-05 16:00 或 2026/06/05 16:00
	for _, layout := range []string{
		"2006-01-02 15:04",
		"2006/01/02 15:04",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, input, loc); err == nil {
			return t.Unix(), nil
		}
	}

	// 处理相对时间
	input = strings.ToLower(input)

	// 匹配 "tomorrow HH:MM" 或 "tomorrow HHMM"
	if matches := regexp.MustCompile(`tomorrow\s*(\d{1,2}):?(\d{2})`).FindStringSubmatch(input); len(matches) == 3 {
		hour, _ := strconv.Atoi(matches[1])
		min, _ := strconv.Atoi(matches[2])
		tomorrow := ref.AddDate(0, 0, 1)
		target := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), hour, min, 0, 0, loc)
		if target.Unix() > now {
			return target.Unix(), nil
		}
	}

	// 匹配 "next Monday" 等星期
	weekdayMap := map[string]time.Weekday{
		"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
		"wednesday": time.Wednesday, "thursday": time.Thursday,
		"friday": time.Friday, "saturday": time.Saturday,
	}
	for name, wd := range weekdayMap {
		if strings.Contains(input, "next "+name) || strings.Contains(input, "下个"+name) {
			daysUntil := (int(wd) - int(ref.Weekday()) + 7) % 7
			if daysUntil == 0 {
				daysUntil = 7
			}
			target := ref.AddDate(0, 0, daysUntil)
			hour, min := 12, 0
			if hm := regexp.MustCompile(`(\d{1,2}):(\d{2})`).FindStringSubmatch(input); len(hm) == 3 {
				hour, _ = strconv.Atoi(hm[1])
				min, _ = strconv.Atoi(hm[2])
			}
			t := time.Date(target.Year(), target.Month(), target.Day(), hour, min, 0, 0, loc)
			return t.Unix(), nil
		}
	}

	return 0, fmt.Errorf("unable to parse time: %s", input)
}
