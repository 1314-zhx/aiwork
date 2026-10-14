/**
 * @author: dn-jinmin/dn-jinmin
 * @doc: 规范化时间解析模板
 */

package approval

import "aiworkc/pkg/langchain/defend"

const (
	// TimeTextFormat 正规范化时间格式说明
	TimeTextFormat = `
## Time text format (must follow)

Output Chinese standardized time text. Never output unix timestamps.

### Date (pick one)
- relative: 今天、明天、后天、大后天
- weekday: 下周周一~周日、下下周一~周日
- calendar: 1月1日、12月25日 (may cross year)

### Time (optional; omit means all day 00:00-23:59:59)
- HH:mm: 09:00、14:30
- HH点: 9点、18点
- HH点半: 9点半、18点半

### Valid examples
- "明天 09:00"
- "明天 18点"
- "明天 9点半"
- "今天"
- "后天 14:30"

### Forbidden
- timestamps: 175xxx
- ISO dates: 2024-12-25
- slang: 明个、礼拜一、后天上午
`

	_defaultCreateApprovalTemplate = `Local date: {{.dateStr}}
Tomorrow: {{.tomorrowDate}}
Weekday today: {{.todayWeekday}}
Next week start: {{.nextWeekStart}}
Next week end: {{.nextWeekEnd}}

Extract approval fields from the user text.

1. MOST IMPORTANT: startTimeText and endTimeText MUST use the Chinese time format below.
2. timeType: 1 if duration < 24h, else 2
3. reason: keep user meaning, write in Chinese

` + TimeTextFormat + `
` + defend.DataHandlingInstruction + `
User input:
` + defend.UserInputPrefix + `{{.input}}` + defend.UserInputSuffix
)
