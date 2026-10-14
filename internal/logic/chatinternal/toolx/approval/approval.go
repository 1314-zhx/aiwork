/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package approval

import (
	"aiworkc/internal/domain"
	"aiworkc/internal/model"
	"aiworkc/internal/svc"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/outputparserx"
	"aiworkc/pkg/token"
	"aiworkc/pkg/xerr"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/prompts"
	"time"
)

func confirmDraft(req domain.Approval) (string, error) {
	body, err := json.Marshal(domain.ChatResp{
		ChatType: int(domain.ConfirmApproval),
		Data:     req,
	})
	if err != nil {
		return "", err
	}
	return "Final Answer: " + string(body) + "\n单据尚未写入数据库。请将上述 JSON 原样作为最终答案，等待用户点击提交。不要再次调用创建类工具。\n", nil
}

type Approval interface {
	// Extract 只抽取业务字段并返回待确认草稿，不写数据库。
	Extract(ctx context.Context, input string) (string, error)
}

var Approvals map[model.ApprovalType]Approval

func NewApproval(svc *svc.ServiceContext, approvalType model.ApprovalType) (Approval, error) {
	if Approvals == nil {
		Approvals = map[model.ApprovalType]Approval{
			model.LeaveApproval:    NewLeave(svc),
			model.MakeCardApproval: NewMakeCard(svc),
			model.GoOutApproval:    NewGoOut(svc),
		}
	}

	a := Approvals[approvalType]
	if a == nil {
		return nil, errors.New("不存在该审批类型" + fmt.Sprintf("%v", approvalType))
	}

	return a, nil
}

type MakeCard struct {
	svc          *svc.ServiceContext
	c            chains.Chain
	outputparser outputparserx.Structured
}

func NewMakeCard(svc *svc.ServiceContext) *MakeCard {
	output := outputparserx.NewStructured([]outputparserx.ResponseSchema{
		{
			Name:        "date",
			Description: "submit timestamp in seconds, e.g. 1720921573",
			Type:        "int64",
		}, {
			Name:        "reason",
			Description: "make-card reason, Chinese",
		}, {
			Name:        "day",
			Description: "make-card date as YYYYMMDD, e.g. 20221011",
			Type:        "int64",
		}, {
			Name:        "workCheckType",
			Description: "card type; 1=clock-in, 2=clock-out",
			Type:        "int",
		},
	})
	return &MakeCard{
		svc: svc,
		c: chains.NewLLMChain(svc.LLMs, prompts.NewPromptTemplate(
			_defaultCreateApprovalTemplate+output.GetFormatInstructions(), []string{langchain.Input, "dateStr", "tomorrowDate", "tomorrowStart", "tomorrowEnd"},
		)),
		outputparser: output,
	}
}

func (m *MakeCard) Extract(ctx context.Context, input string) (string, error) {
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)
	tomorrowStart := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, now.Location()).Unix()
	tomorrowEnd := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 23, 59, 59, 0, now.Location()).Unix()

	// Prompt injection 防护：清洗用户输入
	safeInput := defend.Sanitize(input, defend.DefaultSanitizeConfig())

	out, err := chains.Predict(ctx, m.c, map[string]any{
		langchain.Input: safeInput,
		"dateStr":       now.Format("2006-01-02"),
		"tomorrowDate":  tomorrow.Format("2006-01-02"),
		"tomorrowStart": tomorrowStart,
		"tomorrowEnd":   tomorrowEnd,
	})
	if err != nil {
		return "", err
	}

	v, err := m.outputparser.Parse(out)
	if err != nil {
		return "", err
	}

	var data domain.MakeCard
	if err := mapstructure.Decode(v, &data); err != nil {
		return "", err
	}

	req := domain.Approval{
		Type:     int(model.MakeCardApproval),
		MakeCard: &data,
	}
	if err := assertExtractedApproval(req); err != nil {
		return "", err
	}
	return confirmDraft(req)
}

type Leave struct {
	svc          *svc.ServiceContext
	c            chains.Chain
	outPutParser outputparserx.Structured
}

func NewLeave(svc *svc.ServiceContext) *Leave {
	output := outputparserx.NewStructured([]outputparserx.ResponseSchema{
		{
			Name:        "type",
			Description: "leave type enum: 0.事假 1.调休 2.病假 3.年假 4.产假 5.陪产假 6.婚假 7.丧假 8.哺乳假; pick closest, default 0",
			Type:        "int",
		}, {
			Name:        "startTimeText",
			Description: "leave start, Chinese time text, e.g. 明天 09:00, 后天 14:30, 下周一 10:00",
			Type:        "string",
		}, {
			Name:        "endTimeText",
			Description: "leave end, Chinese time text, e.g. 明天 18:00, 后天 18:00, 下周一 18:00",
			Type:        "string",
		}, {
			Name:        "reason",
			Description: "leave reason, Chinese",
			Type:        "string",
		}, {
			Name:        "timeType",
			Description: "duration type; 1=hours (<24h), 2=days (>=24h)",
			Type:        "int",
		},
	})
	return &Leave{
		svc:          svc,
		outPutParser: output,
		c: chains.NewLLMChain(svc.LLMs, prompts.NewPromptTemplate(
			_defaultCreateApprovalTemplate+output.GetFormatInstructions(), []string{langchain.Input, "dateStr", "tomorrowDate", "todayWeekday", "nextWeekStart", "nextWeekEnd"},
		)),
	}
}

func (m *Leave) Extract(ctx context.Context, input string) (string, error) {
	if err := rejectIfLeaveOccupied(ctx, m.svc); err != nil {
		return "", err
	}
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)

	// 计算下周第一天和最后一天
	nextWeekStart := now.AddDate(0, 0, 7-int(now.Weekday())+1) // 下周一
	nextWeekEnd := nextWeekStart.AddDate(0, 0, 6)              // 下周日

	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

	// Prompt injection 防护：清洗用户输入
	safeInput := defend.Sanitize(input, defend.DefaultSanitizeConfig())

	out, err := chains.Predict(ctx, m.c, map[string]any{
		langchain.Input: safeInput,
		"dateStr":       now.Format("2006-01-02"),
		"tomorrowDate":  tomorrow.Format("2006-01-02"),
		"todayWeekday":  weekdays[now.Weekday()],
		"nextWeekStart": nextWeekStart.Format("2006-01-02"),
		"nextWeekEnd":   nextWeekEnd.Format("2006-01-02"),
	}, chains.WithCallback(m.svc.Callbacks))
	if err != nil {
		return "", xerr.WithMessage(err, "chains.Predict")
	}

	v, err := m.outPutParser.Parse(out)
	if err != nil {
		return "", xerr.WithMessage(err, "m.outPutParser.Parse")
	}

	// 使用 mapstructure 获取原始数据
	var rawData map[string]interface{}
	if err := mapstructure.Decode(v, &rawData); err != nil {
		return "", xerr.WithMessage(err, "mapstructure.Decode")
	}

	// 解析时间文本为时间戳
	startTimeText, _ := rawData["startTimeText"].(string)
	endTimeText, _ := rawData["endTimeText"].(string)

	startTs, endTs, parseFailed := ResolveLeaveTimes(startTimeText, endTimeText, input, now)

	timeType := 1
	if endTs-startTs >= 24*60*60 {
		timeType = 2
	}

	data := domain.Leave{
		Type:      intVal(rawData, "type"),
		StartTime: startTs,
		EndTime:   endTs,
		Reason:    strVal(rawData, "reason"),
		TimeType:  timeType,
	}
	if parseFailed {
		data.TimeParseFailed = true
		data.TimeParseHint = LeaveTimeParseHint
	}

	req := domain.Approval{
		Type:  int(model.LeaveApproval),
		Leave: &data,
	}
	if err := assertExtractedApproval(req); err != nil {
		return "", err
	}
	return confirmDraft(req)
}

func rejectIfLeaveOccupied(ctx context.Context, svc *svc.ServiceContext) error {
	if svc == nil || svc.ApprovalModel == nil {
		return nil
	}
	uid := token.GetUId(ctx)
	if uid == "" {
		return nil
	}
	active, err := svc.ApprovalModel.FindActiveLeave(ctx, uid)
	if err != nil {
		return err
	}
	if active == nil {
		return nil
	}
	return model.LeaveOccupiedError(active)
}

// intVal 安全获取 int 值
func intVal(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

// strVal 安全获取 string 值
func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

type GoOut struct {
	svc          *svc.ServiceContext
	c            chains.Chain
	outPutParser outputparserx.Structured
}

func NewGoOut(svc *svc.ServiceContext) *GoOut {
	output := outputparserx.NewStructured([]outputparserx.ResponseSchema{
		{
			Name:        "startTime",
			Description: "go-out start timestamp in seconds, e.g. 1720921573",
			Type:        "int64",
		}, {
			Name:        "endTime",
			Description: "go-out end timestamp in seconds, e.g. 1720921573",
			Type:        "int64",
		}, {
			Name:        "reason",
			Description: "go-out reason, Chinese",
		},
	})
	return &GoOut{
		svc:          svc,
		outPutParser: output,
		c: chains.NewLLMChain(svc.LLMs, prompts.NewPromptTemplate(
			_defaultCreateApprovalTemplate+output.GetFormatInstructions(), []string{langchain.Input, "dateStr", "tomorrowDate", "tomorrowStart", "tomorrowEnd"},
		)),
	}
}

func (m *GoOut) Extract(ctx context.Context, input string) (string, error) {
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)
	tomorrowStart := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, now.Location()).Unix()
	tomorrowEnd := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 23, 59, 59, 0, now.Location()).Unix()

	// Prompt injection 防护：清洗用户输入
	safeInput := defend.Sanitize(input, defend.DefaultSanitizeConfig())

	out, err := chains.Predict(ctx, m.c, map[string]any{
		langchain.Input: safeInput,
		"dateStr":       now.Format("2006-01-02"),
		"tomorrowDate":  tomorrow.Format("2006-01-02"),
		"tomorrowStart": tomorrowStart,
		"tomorrowEnd":   tomorrowEnd,
	})
	if err != nil {
		return "", xerr.WithMessage(err, "chains.Predict")
	}

	v, err := m.outPutParser.Parse(out)
	if err != nil {
		return "", xerr.WithMessage(err, " m.outPutParser.Parse")
	}

	var data domain.GoOut
	if err := mapstructure.Decode(v, &data); err != nil {
		return "", xerr.WithMessage(err, "domain.GoOut")
	}

	req := domain.Approval{
		Type:  int(model.GoOutApproval),
		GoOut: &data,
	}
	if err := assertExtractedApproval(req); err != nil {
		return "", err
	}
	return confirmDraft(req)
}

func assertExtractedApproval(req domain.Approval) error {
	switch model.ApprovalType(req.Type) {
	case model.LeaveApproval:
		if req.Leave == nil || req.Leave.StartTime <= 0 || req.Leave.EndTime <= 0 {
			return errors.New("请假时间解析失败")
		}
		if req.Leave.EndTime <= req.Leave.StartTime {
			return errors.New("请假结束时间必须晚于开始时间")
		}
	case model.MakeCardApproval:
		if req.MakeCard == nil || req.MakeCard.Day == 0 {
			return errors.New("补卡信息解析失败")
		}
	case model.GoOutApproval:
		if req.GoOut == nil || req.GoOut.StartTime <= 0 || req.GoOut.EndTime <= 0 {
			return errors.New("外出时间解析失败")
		}
		if req.GoOut.EndTime <= req.GoOut.StartTime {
			return errors.New("外出结束时间必须晚于开始时间")
		}
	default:
		return errors.New("不支持的审批类型")
	}
	return nil
}
