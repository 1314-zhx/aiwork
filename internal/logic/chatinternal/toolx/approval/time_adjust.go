/**
 * @author: dn-jinmin/dn-jinmin
 * @doc: 规范化时间解析 - 支持确定性时间解析，避免 LLM 时间戳幻觉
 */

package approval

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"aiworkc/internal/domain"
)

type leaveTimeAdjustTarget struct {
	StartTime int64
	EndTime   int64
	TimeType  int
}

const LeaveTimeParseHint = "时间解析失败，请手动填写请假起止时间。若不修改，将按现在起一天计算。"

func DefaultLeaveRange(now time.Time) (start, end int64) {
	if now.IsZero() {
		now = time.Now()
	}
	return now.Unix(), now.Add(24 * time.Hour).Unix()
}

func leaveRangeValid(start, end int64) bool {
	return start > 0 && end > start
}

// ResolveLeaveTimes 先解析模型抽出的时间文本，失败再用原句校正；
// 仍不合法则用「现在起一天」的默认值，并标记需要用户手填。
func ResolveLeaveTimes(startText, endText, original string, now time.Time) (start, end int64, parseFailed bool) {
	if now.IsZero() {
		now = time.Now()
	}
	start, startOk := ParseTimeText(startText, now)
	end, endOk := ParseTimeText(endText, now)
	if !startOk || !endOk || !leaveRangeValid(start, end) {
		adjusted := struct {
			StartTime int64
			EndTime   int64
			TimeType  int
		}{}
		AdjustLeaveTimeFromInput(original, &adjusted)
		if !startOk {
			start = adjusted.StartTime
		}
		if !endOk {
			end = adjusted.EndTime
		}
	}
	if !leaveRangeValid(start, end) {
		start, end = DefaultLeaveRange(now)
		return start, end, true
	}
	return start, end, false
}

func ApplyDefaultLeaveTimes(leave *domain.Leave, now time.Time) {
	if leave == nil {
		return
	}
	if leaveRangeValid(leave.StartTime, leave.EndTime) {
		return
	}
	leave.StartTime, leave.EndTime = DefaultLeaveRange(now)
	leave.TimeType = 2
}

// ParseTimeText 将规范化时间文本解析为时间戳（导出）
// 支持格式：
//   - "今天"、"明天"、"后天"、"大后天"
//   - "今天 09:00"、"明天 14:30"
//   - "今天 9点"、"明天 18点半"
//   - "下周一"、"下周二"...
//   - "下周周三 10:00"
//   - "1月1日"、"12月25日"
//   - "1月1日 09:00"
func ParseTimeText(input string, ref time.Time) (int64, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return 0, false
	}

	// 检测日期部分
	dayOffset, weekday, explicitDate := detectDatePart(trimmed, ref)

	// 检测时间部分
	hour, minute := extractClock(trimmed)

	// 计算目标日期
	targetDay := calculateTargetDay(ref, dayOffset, weekday, explicitDate)
	if targetDay.IsZero() {
		targetDay = ref // 默认今天
	}

	// 设置时间（默认为整天）
	if hour == -1 {
		hour, minute = 0, 0
	}

	t := time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day(), hour, minute, 0, 0, ref.Location())
	return t.Unix(), true
}

// ParseTimeRange 解析时间范围
// 支持格式：
//   - "明天 09:00 到 18:00"
//   - "明天 9点到18点"
//   - "下周三 14:00 到 17:00"
func ParseTimeRange(input string, ref time.Time) (startTs, endTs int64, ok bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return 0, 0, false
	}

	// 尝试分割开始和结束
	var startText, endText string

	// 支持多种分隔符
	for _, sep := range []string{" 到 ", " 至 ", "-"} {
		idx := strings.Index(trimmed, sep)
		if idx > 0 {
			startText = strings.TrimSpace(trimmed[:idx])
			endText = strings.TrimSpace(trimmed[idx+len(sep):])
			break
		}
	}

	if startText == "" {
		// 只有一个时间点
		ts, ok := ParseTimeText(trimmed, ref)
		return ts, ts, ok
	}

	startTs, ok = ParseTimeText(startText, ref)
	if !ok {
		return 0, 0, false
	}

	endTs, ok = ParseTimeText(endText, ref)
	if !ok {
		return startTs, startTs, true // 结束时间解析失败，用开始时间
	}

	// 如果结束时间早于开始时间，且结束时间没有明确日期，说明是第二天
	if endTs < startTs {
		if !containsExplicitDate(endText) {
			endTs += 24 * 60 * 60
		}
	}

	return startTs, endTs, true
}

// detectDatePart 检测日期部分，返回相对偏移、星期几、是否显式指定日期
func detectDatePart(input string, ref time.Time) (dayOffset int, weekday time.Weekday, explicitDate string) {
	dayOffset = 0
	weekday = -1
	explicitDate = ""

	// 1. 检测相对日期（今天、明天、后天、大后天）
	switch {
	case strings.Contains(input, "大后天"):
		dayOffset = 3
		explicitDate = extractBeforeTime(input, "大后天")
	case strings.Contains(input, "后天"):
		dayOffset = 2
		explicitDate = extractBeforeTime(input, "后天")
	case strings.Contains(input, "明天"):
		dayOffset = 1
		explicitDate = extractBeforeTime(input, "明天")
	case strings.Contains(input, "今天"):
		dayOffset = 0
		explicitDate = extractBeforeTime(input, "今天")
	}

	if explicitDate != "" || dayOffset != 0 {
		return dayOffset, weekday, explicitDate
	}

	// 2. 检测星期（本周）
	weekday = detectWeekday(input)
	if weekday != -1 {
		explicitDate = input
		return 0, weekday, explicitDate
	}

	// 3. 检测下周
	if strings.Contains(input, "下周一") {
		weekday = time.Monday
		dayOffset = 7 + int(ref.Weekday())
		if ref.Weekday() <= time.Monday {
			dayOffset = 7 - int(ref.Weekday()-time.Monday+7)%7
		} else {
			dayOffset = 14 - int(ref.Weekday()-time.Monday+7)%7
		}
		explicitDate = input
		return dayOffset, weekday, explicitDate
	}
	if strings.Contains(input, "下周二") {
		dayOffset = 7 - int(ref.Weekday()-time.Tuesday+7)%7
		if ref.Weekday() <= time.Tuesday {
			dayOffset = 7 - int(ref.Weekday()-time.Tuesday+7)%7
		} else {
			dayOffset = 14 - int(ref.Weekday()-time.Tuesday+7)%7
		}
		weekday = time.Tuesday
		explicitDate = input
		return
	}
	if strings.Contains(input, "下周三") {
		dayOffset = 7 - int(ref.Weekday()-time.Wednesday+7)%7
		if ref.Weekday() <= time.Wednesday {
			dayOffset = 7 - int(ref.Weekday()-time.Wednesday+7)%7
		} else {
			dayOffset = 14 - int(ref.Weekday()-time.Wednesday+7)%7
		}
		weekday = time.Wednesday
		explicitDate = input
		return
	}
	if strings.Contains(input, "下周四") {
		dayOffset = 7 - int(ref.Weekday()-time.Thursday+7)%7
		if ref.Weekday() <= time.Thursday {
			dayOffset = 7 - int(ref.Weekday()-time.Thursday+7)%7
		} else {
			dayOffset = 14 - int(ref.Weekday()-time.Thursday+7)%7
		}
		weekday = time.Thursday
		explicitDate = input
		return
	}
	if strings.Contains(input, "下周五") {
		dayOffset = 7 - int(ref.Weekday()-time.Friday+7)%7
		if ref.Weekday() <= time.Friday {
			dayOffset = 7 - int(ref.Weekday()-time.Friday+7)%7
		} else {
			dayOffset = 14 - int(ref.Weekday()-time.Friday+7)%7
		}
		weekday = time.Friday
		explicitDate = input
		return
	}
	if strings.Contains(input, "下周六") {
		dayOffset = 7 - int(ref.Weekday()-time.Saturday+7)%7
		if ref.Weekday() <= time.Saturday {
			dayOffset = 7 - int(ref.Weekday()-time.Saturday+7)%7
		} else {
			dayOffset = 14 - int(ref.Weekday()-time.Saturday+7)%7
		}
		weekday = time.Saturday
		explicitDate = input
		return
	}
	if strings.Contains(input, "下周日") || strings.Contains(input, "下周天") {
		dayOffset = 7 - int(ref.Weekday()-time.Sunday+7)%7
		if ref.Weekday() <= time.Sunday {
			dayOffset = 7 - int(ref.Weekday()-time.Sunday+7)%7
		} else {
			dayOffset = 14 - int(ref.Weekday()-time.Sunday+7)%7
		}
		weekday = time.Sunday
		explicitDate = input
		return
	}

	// 4. 检测具体日期（1月1日、12月25日）
	dateStr := detectMonthDay(input)
	if dateStr != "" {
		explicitDate = dateStr
	}

	return 0, weekday, explicitDate
}

// extractBeforeTime 提取关键词之前的内容（用于判断是否有具体时间）
func extractBeforeTime(input, keyword string) string {
	idx := strings.Index(input, keyword)
	if idx >= 0 {
		return input[:idx+len(keyword)]
	}
	return input
}

// containsExplicitDate 检查是否包含显式日期
func containsExplicitDate(input string) bool {
	_, _, explicitDate := detectDatePart(input, time.Now())
	return explicitDate != ""
}

// detectWeekday 检测星期几
func detectWeekday(input string) time.Weekday {
	switch {
	case strings.Contains(input, "周一") || strings.Contains(input, "星期一"):
		return time.Monday
	case strings.Contains(input, "周二") || strings.Contains(input, "星期二"):
		return time.Tuesday
	case strings.Contains(input, "周三") || strings.Contains(input, "星期三"):
		return time.Wednesday
	case strings.Contains(input, "周四") || strings.Contains(input, "星期四"):
		return time.Thursday
	case strings.Contains(input, "周五") || strings.Contains(input, "星期五"):
		return time.Friday
	case strings.Contains(input, "周六") || strings.Contains(input, "星期六"):
		return time.Saturday
	case strings.Contains(input, "周日") || strings.Contains(input, "星期天") || strings.Contains(input, "星期日"):
		return time.Sunday
	}
	return -1
}

// detectMonthDay 检测具体日期（1月1日、12月25日）
func detectMonthDay(input string) string {
	patterns := []string{
		`(\d{1,2})月(\d{1,2})日`,
		`(\d{1,2})月(\d{1,2})`,
	}
	for _, pattern := range patterns {
		if matches := regexp.MustCompile(pattern).FindStringSubmatch(input); len(matches) == 3 {
			return matches[0]
		}
	}
	return ""
}

// calculateTargetDay 根据检测结果计算目标日期
func calculateTargetDay(ref time.Time, dayOffset int, weekday time.Weekday, explicitDate string) time.Time {
	// 如果有具体日期（如 12月25日）
	if explicitDate != "" {
		if monthDay := detectMonthDay(explicitDate); monthDay != "" {
			if matches := regexp.MustCompile(`(\d{1,2})月(\d{1,2})日?`).FindStringSubmatch(monthDay); len(matches) == 3 {
				month, _ := strconv.Atoi(matches[1])
				day, _ := strconv.Atoi(matches[2])
				year := ref.Year()
				// 如果日期已过，移到下一年
				targetDate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, ref.Location())
				if targetDate.Before(ref) {
					targetDate = targetDate.AddDate(1, 0, 0)
				}
				return targetDate
			}
		}
	}

	// 如果指定了星期几
	if weekday != -1 {
		daysUntil := int(weekday) - int(ref.Weekday())
		if daysUntil < 0 {
			daysUntil += 7
		}
		return ref.AddDate(0, 0, daysUntil)
	}

	// 使用相对偏移
	if dayOffset != 0 {
		return ref.AddDate(0, 0, dayOffset)
	}

	// 默认今天
	return ref
}

// extractClock 提取时钟时间，返回 (hour, minute, found)
// hour = -1 表示没有找到具体时间（整天）
func extractClock(input string) (hour int, minute int) {
	hour = -1
	minute = 0

	// 检测时段
	period := detectPeriod(input)

	// 尝试各种时间格式
	// HH:mm 格式
	if matches := regexp.MustCompile(`(\d{1,2}):(\d{1,2})`).FindStringSubmatch(input); len(matches) == 3 {
		hour, _ = strconv.Atoi(matches[1])
		minute, _ = strconv.Atoi(matches[2])
		hour = adjustHourByPeriod(period, hour)
		return
	}

	// HH点半 格式
	if matches := regexp.MustCompile(`(\d{1,2})点半`).FindStringSubmatch(input); len(matches) == 2 {
		hour, _ = strconv.Atoi(matches[1])
		minute = 30
		hour = adjustHourByPeriod(period, hour)
		return
	}

	// HH点mm分 格式
	if matches := regexp.MustCompile(`(\d{1,2})点(\d{1,2})分?`).FindStringSubmatch(input); len(matches) == 3 {
		hour, _ = strconv.Atoi(matches[1])
		minute, _ = strconv.Atoi(matches[2])
		hour = adjustHourByPeriod(period, hour)
		return
	}

	// HH点 格式
	if matches := regexp.MustCompile(`(\d{1,2})点`).FindStringSubmatch(input); len(matches) == 2 {
		hour, _ = strconv.Atoi(matches[1])
		minute = 0
		hour = adjustHourByPeriod(period, hour)
		return
	}

	return -1, 0
}

// detectPeriod 检测时段（上午、下午、晚上等）
func detectPeriod(input string) periodHint {
	switch {
	case strings.Contains(input, "凌晨") || strings.Contains(input, "清晨"):
		return periodLateNight
	case strings.Contains(input, "上午") || strings.Contains(input, "早上") || strings.Contains(input, "早晨"):
		return periodMorning
	case strings.Contains(input, "中午") || strings.Contains(input, "午间"):
		return periodNoon
	case strings.Contains(input, "下午"):
		return periodAfternoon
	case strings.Contains(input, "晚上") || strings.Contains(input, "傍晚") || strings.Contains(input, "今晚"):
		return periodEvening
	default:
		return periodUnknown
	}
}

// detectPeriodInt 检测时段，返回 int 类型（兼容用）
func detectPeriodInt(input string) int {
	return int(detectPeriod(input))
}

// adjustHourByPeriod 根据时段调整小时
func adjustHourByPeriod(period periodHint, hour int) int {
	switch period {
	case periodAfternoon, periodEvening:
		if hour < 12 {
			return hour + 12
		}
	case periodNoon:
		if hour > 0 && hour < 11 {
			return hour + 12
		}
	case periodLateNight:
		if hour == 12 {
			return 0
		}
	case periodMorning:
		if hour == 12 {
			return 0
		}
	}
	return hour
}

// ============ 以下是原有的兼容代码 ============

type dayOffset int

const (
	dayUnknown dayOffset = -999
)

// AdjustLeaveTimeFromInput 用原始输入二次校正时间（导出，供外部调用）
func AdjustLeaveTimeFromInput(input string, data *struct {
	StartTime int64
	EndTime   int64
	TimeType  int
}) {
	if data == nil {
		return
	}

	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return
	}

	adjusted := leaveTimeAdjustTarget{
		StartTime: data.StartTime,
		EndTime:   data.EndTime,
		TimeType:  data.TimeType,
	}
	adjustLeaveRangeFromInput(trimmed, time.Now(), &adjusted)
	data.StartTime = adjusted.StartTime
	data.EndTime = adjusted.EndTime
	data.TimeType = adjusted.TimeType
}

func adjustLeaveRangeFromInput(input string, ref time.Time, data *leaveTimeAdjustTarget) {
	if data == nil {
		return
	}

	startTs, endTs, ok := ParseTimeRange(input, ref)
	if !ok {
		return
	}

	data.StartTime = startTs
	data.EndTime = endTs
	if endTs-startTs >= 24*60*60 {
		data.TimeType = 2
	} else {
		data.TimeType = 1
	}
}

func parseChineseRelativeTime(input string, ref time.Time) (int64, bool) {
	return ParseTimeText(input, ref)
}

func parseChineseRelativeRange(input string, ref time.Time) (int64, int64, bool) {
	return ParseTimeRange(input, ref)
}

func normalizeRangeInput(input string) string {
	replacer := strings.NewReplacer(
		"—", "-",
		"－", "-",
		"–", "-",
		"～", "到",
		"至", "到",
	)
	return replacer.Replace(strings.TrimSpace(input))
}

func splitRangeParts(input string) []string {
	for _, sep := range []string{"到", "-"} {
		idx := strings.Index(input, sep)
		if idx > 0 {
			left := strings.TrimSpace(input[:idx])
			right := strings.TrimSpace(input[idx+len(sep):])
			if left != "" && right != "" {
				return []string{left, right}
			}
		}
	}
	return []string{input}
}

type periodHint int

const (
	periodUnknown periodHint = iota
	periodMorning
	periodNoon
	periodAfternoon
	periodEvening
	periodLateNight
)

type timeAnchor struct {
	day    dayOffset
	period periodHint
}

func parseTimePoint(input string, ref time.Time, fallbackDay dayOffset, fallbackPeriod periodHint) (int64, timeAnchor, bool) {
	ts, ok := ParseTimeText(input, ref)
	if !ok {
		return 0, timeAnchor{}, false
	}

	period := detectPeriod(input)
	if period == periodUnknown {
		period = fallbackPeriod
	}

	return ts, timeAnchor{day: dayOffset(0), period: period}, true
}

func containsExplicitDay(input string) bool {
	_, _, explicitDate := detectDatePart(input, time.Now())
	return explicitDate != ""
}

func extractClockWithPeriod(input string, period periodHint) (int, int, bool) {
	h, m := extractClock(input)
	if h == -1 {
		return 0, 0, false
	}
	return h, m, true
}

func adjustPeriodHourByHint(period periodHint, hour int) int {
	return adjustHourByPeriod(period, hour)
}

func sameDayStartOfDay(ts int64) int64 {
	t := time.Unix(ts, 0)
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return start.Unix()
}
