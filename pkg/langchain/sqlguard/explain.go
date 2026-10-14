package sqlguard

import (
	"fmt"
	"strings"
)

var tableLabels = map[string]string{
	"budget": "预算",
}

var columnLabels = map[string]string{
	"budget_id":     "预算ID",
	"budget_no":     "预算编号",
	"title":         "标题",
	"occurred_time": "发生时间",
	"finish_time":   "完成时间",
	"amount":        "金额",
	"amount_bat":    "金额批次",
	"tras":          "转账",
	"user":          "用户",
	"status":        "状态",
	"day":           "日期",
}

// Explain builds a Chinese summary so non-SQL users can confirm intent.
func Explain(sql, userQuery string, topK int) (summary string, tables, columns []string, err error) {
	ins, err := Parse(sql)
	if err != nil {
		return "", nil, nil, err
	}
	if topK <= 0 {
		topK = DefaultTopK
	}

	var b strings.Builder
	b.WriteString("拟执行只读查询，确认后才会查库。\n")
	if len(ins.Tables) > 0 {
		labels := make([]string, 0, len(ins.Tables))
		for _, t := range ins.Tables {
			if zh, ok := tableLabels[t]; ok {
				labels = append(labels, zh+"（"+t+"）")
			} else {
				labels = append(labels, t)
			}
		}
		b.WriteString("数据表：")
		b.WriteString(strings.Join(labels, "、"))
		b.WriteString("\n")
	}
	cols := unique(ins.Columns)
	if len(cols) > 0 {
		labels := make([]string, 0, len(cols))
		for _, c := range cols {
			if zh, ok := columnLabels[c]; ok {
				labels = append(labels, zh)
			} else {
				labels = append(labels, c)
			}
		}
		b.WriteString("查询字段：")
		b.WriteString(strings.Join(labels, "、"))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("最多返回 %d 行\n", topK))
	if q := strings.TrimSpace(userQuery); q != "" {
		b.WriteString("对应问题：")
		b.WriteString(q)
	}
	return strings.TrimSpace(b.String()), ins.Tables, cols, nil
}

func ExplainIR(ir QueryIR, topK int) (summary string, tables, columns []string) {
	if ir.Empty() {
		return "", nil, nil
	}
	if topK <= 0 {
		topK = DefaultTopK
	}
	limit := ir.Limit
	if limit <= 0 || limit > topK {
		limit = topK
	}
	var b strings.Builder
	b.WriteString("拟执行只读查询，确认后才会查库。\n")
	if zh, ok := tableLabels[ir.Table]; ok {
		b.WriteString("数据表：")
		b.WriteString(zh + "（" + ir.Table + "）\n")
	} else {
		b.WriteString("数据表：")
		b.WriteString(ir.Table + "\n")
	}
	cols := unique(append(append([]string{}, ir.Columns...), ir.GroupBy...))
	for _, a := range ir.Aggregates {
		if a.Column != "" {
			cols = unique(append(cols, a.Column))
		}
	}
	if len(cols) > 0 {
		labels := make([]string, 0, len(cols))
		for _, c := range cols {
			if zh, ok := columnLabels[c]; ok {
				labels = append(labels, zh)
			} else {
				labels = append(labels, c)
			}
		}
		b.WriteString("查询字段：")
		b.WriteString(strings.Join(labels, "、"))
		b.WriteString("\n")
	}
	if len(ir.Aggregates) > 0 {
		b.WriteString("汇总：")
		parts := make([]string, 0, len(ir.Aggregates))
		for _, a := range ir.Aggregates {
			parts = append(parts, a.Fn+"("+a.Column+")")
		}
		b.WriteString(strings.Join(parts, "、"))
		b.WriteString("\n")
	}
	if len(ir.Filters) > 0 {
		b.WriteString("条件：")
		parts := make([]string, 0, len(ir.Filters))
		for _, f := range ir.Filters {
			label := f.Column
			if zh, ok := columnLabels[f.Column]; ok {
				label = zh
			}
			parts = append(parts, fmt.Sprintf("%s %s %v", label, f.Op, f.Value))
		}
		b.WriteString(strings.Join(parts, "，"))
		b.WriteString("\n")
	}
	if len(ir.GroupBy) > 0 {
		b.WriteString("分组：")
		b.WriteString(strings.Join(ir.GroupBy, "、"))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("最多返回 %d 行\n", limit))
	if q := strings.TrimSpace(ir.Intent); q != "" {
		b.WriteString("对应问题：")
		b.WriteString(q)
	}
	return strings.TrimSpace(b.String()), []string{ir.Table}, cols
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
