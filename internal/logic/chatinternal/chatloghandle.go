/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package chatinternal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"aiworkc/internal/domain"
	"aiworkc/internal/model"
	"aiworkc/internal/svc"
	"aiworkc/pkg/audit"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/token"

	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/prompts"
)

const (
	maxChatLogSpan      = 31 * 24 * 3600
	maxChatLogGoalRunes = 200
	// 整次总结预算 4000 token。提示词、goal、输出预留一半；记忆按中文约 1 字 1 token。
	maxChatLogRunes = tokencost.AnalysisTokenLimit / 2
)

var chatLogCST = time.FixedZone("CST", 8*3600)

type chatLogLine struct {
	SendTime int64
	Text     string
}

type chatLogMemory struct {
	Text           string
	Truncated      bool
	EffectiveStart int64
	TruncateNote   string
}

type ChatLogHandle struct {
	svc *svc.ServiceContext
	llm chains.Chain
}

func NewChatLogHandle(svc *svc.ServiceContext) *ChatLogHandle {
	return &ChatLogHandle{
		svc: svc,
		llm: chains.NewLLMChain(svc.LLMs, prompts.NewPromptTemplate(_defaultChatLogPrompts, []string{"goal", "history"})),
	}
}

func (c *ChatLogHandle) Chains() chains.Chain {
	return chains.NewTransform(c.transform, nil, nil)
}

func ValidateChatLogWindow(startTime, endTime int64) error {
	if startTime <= 0 || endTime <= 0 {
		return errors.New("请填写明确的开始时间和结束时间")
	}
	if endTime <= startTime {
		return errors.New("结束时间必须晚于开始时间")
	}
	if endTime-startTime > maxChatLogSpan {
		return errors.New("总结时间范围不能超过 31 天")
	}
	return nil
}

func NormalizeChatLogGoal(goal string) (string, error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return "", errors.New("请填写总结目标")
	}
	goal = defend.Sanitize(goal, defend.DefaultSanitizeConfig())
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return "", errors.New("请填写总结目标")
	}
	if utf8.RuneCountInString(goal) > maxChatLogGoalRunes {
		goal = string([]rune(goal)[:maxChatLogGoalRunes])
	}
	return goal, nil
}

// transform 中间层：从表单输入抽出时间戳与目标，再交给 Call。不走 Router。
func (c *ChatLogHandle) transform(ctx context.Context, inputs map[string]any, options ...chains.ChainCallOption) (map[string]any, error) {
	cid, _ := inputs["conversationId"].(string)
	goal, _ := inputs["goal"].(string)
	start, err := unixTimeFrom(inputs["startTime"])
	if err != nil {
		return nil, err
	}
	end, err := unixTimeFrom(inputs["endTime"])
	if err != nil {
		return nil, err
	}

	resp, err := c.Call(ctx, cid, start, end, goal, options...)
	if err != nil {
		return nil, err
	}
	return map[string]any{langchain.OutPut: resp}, nil
}

// Call 拼接记忆、按更早时间戳截断、调用 LLM，并把截断时间写进返回。
func (c *ChatLogHandle) Call(ctx context.Context, conversationID string, startTime, endTime int64, goal string, options ...chains.ChainCallOption) (*domain.ChatResp, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, errors.New("请选择要总结的会话对象")
	}
	if err := ValidateChatLogWindow(startTime, endTime); err != nil {
		return nil, err
	}
	goal, err := NormalizeChatLogGoal(goal)
	if err != nil {
		return nil, err
	}

	tokencost.RaiseLimit(ctx, tokencost.AnalysisTokenLimit)
	if err := tokencost.RequireDaily(ctx, tokencost.DailyEnterNeed(tokencost.AnalysisTokenLimit)); err != nil {
		return nil, err
	}

	list, err := c.svc.ChatlogModel.ListBySendTime(ctx, conversationID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	mem := spliceChatLogMemory(c.formatChatLogLines(ctx, list), startTime, endTime, maxChatLogRunes)

	var text string
	if strings.TrimSpace(mem.Text) == "" {
		text = "该时间范围内没有聊天记录。"
	} else {
		text, err = c.callLLM(ctx, goal, mem.Text, options...)
		if err != nil {
			return nil, err
		}
	}

	data := domain.ChatLogSummaryData{
		Goal:           goal,
		StartTime:      startTime,
		EndTime:        endTime,
		EffectiveStart: mem.EffectiveStart,
		Truncated:      mem.Truncated,
		TruncateNote:   mem.TruncateNote,
		Text:           attachTruncateNotice(text, mem),
	}
	if data.Truncated && strings.TrimSpace(data.TruncateNote) == "" {
		return nil, errors.New("总结发生截断但缺少截断时间说明")
	}

	audit.Event(ctx, "chatlog.call", "uid", token.GetUId(ctx), "cid", conversationID, "truncated", mem.Truncated)
	return &domain.ChatResp{ChatType: domain.ChatLog, Data: data}, nil
}

func (c *ChatLogHandle) callLLM(ctx context.Context, goal, history string, options ...chains.ChainCallOption) (string, error) {
	safeHistory := defend.Sanitize(history, chatLogSanitizeConfig())
	res, err := chains.Call(ctx, c.llm, map[string]any{
		"goal":    goal,
		"history": safeHistory,
	}, options...)
	if err != nil {
		return "", err
	}
	raw, ok := res[langchain.OutPut].(string)
	if !ok {
		return "", chains.ErrInvalidOutputValues
	}
	return parseChatLogText(raw), nil
}

func (c *ChatLogHandle) formatChatLogLines(ctx context.Context, list []*model.Chatlog) []chatLogLine {
	lines := make([]chatLogLine, 0, len(list))
	record := make(map[string]*model.User)
	for i := range list {
		if list[i] == nil {
			continue
		}
		name := list[i].SendId
		if v, ok := record[list[i].SendId]; ok {
			name = v.Name
		} else if t, err := c.svc.UserModel.FindOne(ctx, list[i].SendId); err == nil && t != nil {
			record[list[i].SendId] = t
			name = t.Name
		}
		lines = append(lines, chatLogLine{
			SendTime: list[i].SendTime,
			Text:     fmt.Sprintf("%s(%s): %s\n", name, list[i].SendId, list[i].MsgContent),
		})
	}
	return lines
}

func spliceChatLogMemory(lines []chatLogLine, startTime, endTime int64, maxRunes int) chatLogMemory {
	mem := chatLogMemory{
		EffectiveStart: startTime,
	}
	if maxRunes <= 0 {
		maxRunes = maxChatLogRunes
	}
	if len(lines) == 0 {
		return mem
	}

	kept := make([]chatLogLine, 0, len(lines))
	used := 0
	truncated := false
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i].Text
		n := utf8.RuneCountInString(line)
		if len(kept) == 0 && n > maxRunes {
			line = keepRecentRunes(line, maxRunes)
			n = utf8.RuneCountInString(line)
			truncated = true
			kept = append(kept, chatLogLine{SendTime: lines[i].SendTime, Text: line})
			used = n
			break
		}
		if used+n > maxRunes {
			truncated = true
			break
		}
		kept = append(kept, chatLogLine{SendTime: lines[i].SendTime, Text: line})
		used += n
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}

	var b strings.Builder
	for i := range kept {
		b.WriteString(kept[i].Text)
	}
	mem.Text = b.String()
	mem.Truncated = truncated
	if len(kept) > 0 {
		mem.EffectiveStart = kept[0].SendTime
	}
	if mem.Truncated {
		mem.TruncateNote = formatTruncateNote(mem.EffectiveStart, endTime)
	}
	return mem
}

func formatTruncateNote(effectiveStart, endTime int64) string {
	return fmt.Sprintf("因内容超过可处理长度，已截断 %s 之前的消息。本次实际总结范围为 %s 至 %s。",
		formatChatLogTime(effectiveStart),
		formatChatLogTime(effectiveStart),
		formatChatLogTime(endTime),
	)
}

func attachTruncateNotice(text string, mem chatLogMemory) string {
	text = strings.TrimSpace(text)
	if !mem.Truncated {
		return text
	}
	note := strings.TrimSpace(mem.TruncateNote)
	if note == "" {
		note = formatTruncateNote(mem.EffectiveStart, 0)
	}
	if text == "" {
		return note
	}
	if strings.Contains(text, note) {
		return text
	}
	return note + "\n\n" + text
}

func formatChatLogTime(ts int64) string {
	if ts <= 0 {
		return "未知时间"
	}
	return time.Unix(ts, 0).In(chatLogCST).Format("2006-01-02 15:04")
}

func chatLogSanitizeConfig() defend.SanitizeConfig {
	cfg := defend.SanitizeHistoryConfig()
	cfg.MaxLen = 0
	return cfg
}

func keepRecentRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return "…\n" + string(runes[len(runes)-max:])
}

func unixTimeFrom(v any) (int64, error) {
	switch t := v.(type) {
	case nil:
		return 0, nil
	case int:
		return int64(t), nil
	case int32:
		return int64(t), nil
	case int64:
		return t, nil
	case float64:
		return int64(t), nil
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0, errors.New("时间格式无效")
		}
		return n, nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, errors.New("时间格式无效")
		}
		return n, nil
	default:
		return 0, errors.New("时间格式无效")
	}
}

func parseChatLogText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var obj struct {
		Text string `json:"text"`
	}
	if json.Unmarshal([]byte(text), &obj) == nil && strings.TrimSpace(obj.Text) != "" {
		return strings.TrimSpace(obj.Text)
	}
	return text
}
