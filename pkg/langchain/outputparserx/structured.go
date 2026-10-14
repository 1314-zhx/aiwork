/**
 * @author: dn-jinmin/dn-jinmin
 * @doc: 符合 OWASP LLM Top 10 标准的结构化输出解析器
 *
 * 安全特性：
 * 1. 正则提取 JSON 块（兼容 markdown、大小写、空格）
 * 2. 必填字段校验
 * 3. 字段类型校验（int、string、bool、[]string 等）
 * 4. Prompt Injection 输出检测
 * 5. 解析失败重试建议
 */

package outputparserx

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

// ============================================================
// 公共类型定义
// ============================================================

// ParseError 解析错误的结构化定义
type ParseError struct {
	Text       string // 原始 LLM 输出
	Reason     string // 错误原因
	Suggestion string // 修复建议
	Phase      string // 失败阶段：extract | unmarshal | validate
}

func (e ParseError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("[%s] %s. 建议: %s\n原始输出: %s",
			e.Phase, e.Reason, e.Suggestion, truncate(e.Text, 200))
	}
	return fmt.Sprintf("[%s] %s\n原始输出: %s", e.Phase, e.Reason, truncate(e.Text, 200))
}

// ValidationError 校验错误
type ValidationError struct {
	Field    string // 失败的字段名
	Expected string // 期望值/类型
	Actual   string // 实际值/类型
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("字段 %q 类型校验失败: 期望 %s，实际 %s", e.Field, e.Expected, e.Actual)
}

// ResponseSchema 定义 LLM 输出的期望结构
// 对标 OWASP: Define and validate expected output formats
type ResponseSchema struct {
	Name        string           `json:"name"`              // 字段名
	Description string           `json:"description"`       // 字段描述
	Type        string           `json:"type,omitempty"`    // 期望类型: string, int, int64, float, bool, []string, object
	Require     bool             `json:"require,omitempty"` // 是否必填
	Default     any              `json:"default,omitempty"` // 默认值（可选字段未填时使用）
	Enum        []string         `json:"enum,omitempty"`    // 枚举值约束
	Min         *float64         `json:"min,omitempty"`     // 数值最小值
	Max         *float64         `json:"max,omitempty"`     // 数值最大值
	MinLen      *int             `json:"minLen,omitempty"`  // 字符串最小长度
	MaxLen      *int             `json:"maxLen,omitempty"`  // 字符串最大长度
	Schemas     []ResponseSchema `json:"schemas,omitempty"` // 嵌套对象（Type=object 时使用）
	Pattern     string           `json:"pattern,omitempty"` // 正则约束（Type=string 时使用）
}

// Structured 结构化输出解析器
type Structured struct {
	ResponseSchemas []ResponseSchema
	// 解析配置
	StrictMode     bool // 严格模式：启用所有校验
	AllowPlainJSON bool // 是否允许裸 JSON（不带 markdown 包裹）
}

// Option 解析器配置选项
type Option func(*Structured)

// WithStrictMode 启用严格模式
func WithStrictMode() Option {
	return func(s *Structured) {
		s.StrictMode = true
	}
}

// WithAllowPlainJSON 允许裸 JSON 输出
func WithAllowPlainJSON() Option {
	return func(s *Structured) {
		s.AllowPlainJSON = true
	}
}

// NewStructured 创建结构化解析器
func NewStructured(schema []ResponseSchema, opts ...Option) Structured {
	s := Structured{
		ResponseSchemas: schema,
		StrictMode:      false,
		AllowPlainJSON:  true,
	}
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

// ============================================================
// 公共接口实现
// ============================================================

var _ schema.OutputParser[any] = Structured{}

// Parse 解析 LLM 输出文本为 map
func (p Structured) Parse(text string) (any, error) {
	return p.parse(text)
}

// ParseWithPrompt 解析（忽略 prompt 参数）
func (p Structured) ParseWithPrompt(text string, _ llms.PromptValue) (any, error) {
	return p.parse(text)
}

// GetFormatInstructions 生成格式指令
func (p Structured) GetFormatInstructions() string {
	return fmt.Sprintf(_structuredFormatInstructionTemplate, p.jsonMarshal(p.ResponseSchemas, 0))
}

// JSONSchema 把 ResponseSchema 转成 OpenAI / DashScope function parameters 对象。
func (p Structured) JSONSchema() map[string]any {
	return responseSchemasToJSONSchema(p.ResponseSchemas)
}

func responseSchemasToJSONSchema(schemas []ResponseSchema) map[string]any {
	props := make(map[string]any, len(schemas))
	required := make([]string, 0)
	for _, rs := range schemas {
		name := strings.TrimSpace(rs.Name)
		if name == "" {
			continue
		}
		var node map[string]any
		if len(rs.Schemas) > 0 {
			node = responseSchemasToJSONSchema(rs.Schemas)
			if rs.Description != "" {
				node["description"] = rs.Description
			}
		} else {
			node = map[string]any{"type": jsonSchemaType(rs.Type)}
			if rs.Description != "" {
				node["description"] = rs.Description
			}
			if jsonSchemaType(rs.Type) == "array" {
				node["items"] = map[string]any{"type": "string"}
			}
			if len(rs.Enum) > 0 {
				node["enum"] = rs.Enum
			}
		}
		props[name] = node
		if rs.Require {
			required = append(required, name)
		}
	}
	out := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": true,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func jsonSchemaType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "int64", "float", "number":
		return "number"
	case "bool", "boolean":
		return "boolean"
	case "[]string", "array":
		return "array"
	case "object":
		return "object"
	default:
		return "string"
	}
}

// Type 返回解析器类型
func (p Structured) Type() string {
	return "structured_parser"
}

// ============================================================
// 核心解析流程
// ============================================================

// parse 完整的解析流程
// 对标 OWASP: 确定性代码验证，非 LLM 自我校验
func (p Structured) parse(text string) (map[string]any, error) {
	if text == "" {
		return nil, ParseError{
			Text:       text,
			Reason:     "LLM 输出为空",
			Suggestion: "检查模型响应是否正常",
			Phase:      "extract",
		}
	}

	// Step 1: Prompt Injection 输出检测（前置安全检查）
	if p.StrictMode {
		if injection := detectOutputInjection(text); injection != "" {
			return nil, ParseError{
				Text:       text,
				Reason:     "检测到潜在 Prompt Injection",
				Suggestion: fmt.Sprintf("模型输出包含可疑内容: %s", injection),
				Phase:      "security",
			}
		}
	}

	// Step 2: JSON 提取
	jsonStr, err := p.extractJSON(text)
	if err != nil {
		return nil, err
	}

	// Step 3: JSON 解析
	parsed, err := p.unmarshalJSON(jsonStr)
	if err != nil {
		return nil, err
	}

	// Step 4: Schema 校验
	if err := p.validate(parsed); err != nil {
		return nil, err
	}

	return parsed, nil
}

// ============================================================
// Step 1: Prompt Injection 输出检测
// ============================================================

// detectOutputInjection 检测 LLM 输出中的注入模式
// 对标 OWASP: Output monitoring and validation
func detectOutputInjection(text string) string {
	injectionPatterns := map[string]string{
		"MRKL/ReAct 格式":   "(?i)(Thought:[\\s\\S]*?Action:|Action:[\\s\\S]*?Observation:)",
		"系统指令泄露":          "(?i)(new instructions:|ignore|forget everything|system:\\s*you (?:must|should))",
		"角色劫持":            "(?i)(you are now|i am actually|i want you to act as)",
		"嵌套 Final Answer": "(?i)final answer:[\\s\\S]*?\\{[\\s\\S]*?\\}",
		"代码执行尝试":          "(?i)(exec\\(|eval\\(|os\\.system|__import__|subprocess)",
	}

	for name, pattern := range injectionPatterns {
		if regexp.MustCompile(pattern).MatchString(text) {
			return name
		}
	}
	return ""
}

// ============================================================
// Step 2: JSON 提取
// ============================================================

// jsonCodeBlockRegex 匹配 markdown 代码块包裹的 JSON
// 支持：```json ... ```、```json ... ```、``` json ... ```
var jsonCodeBlockRegex = regexp.MustCompile("```[^\\n]*\\n([\\s\\S]+?)\\n```")

// extractJSON 从 LLM 输出中提取 JSON 字符串
// 支持多种格式：
//   - ```json\n{"a": 1}\n```
//   - {"a": 1}
//   - ```json {"a": 1} ```
func (p Structured) extractJSON(text string) (string, error) {
	// 清理不可见字符
	text = cleanInvisibleChars(text)

	// 优先尝试正则提取 markdown 代码块
	matches := jsonCodeBlockRegex.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	// 降级：尝试直接解析（裸 JSON）
	if p.AllowPlainJSON {
		trimmed := strings.TrimSpace(text)
		// 简单判断是否像 JSON
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			return trimmed, nil
		}
	}

	return "", ParseError{
		Text:       text,
		Reason:     "无法从输出中提取 JSON",
		Suggestion: "LLM 输出必须包含有效的 JSON 对象，格式：```json\n{...}\n```",
		Phase:      "extract",
	}
}

// cleanInvisibleChars 清理不可见字符
func cleanInvisibleChars(s string) string {
	// 移除零宽字符、防注入常用字符
	s = strings.ReplaceAll(s, "\u200b", "") // 零宽空格
	s = strings.ReplaceAll(s, "\u200c", "") // 零宽非连接符
	s = strings.ReplaceAll(s, "\u200d", "") // 零宽连接符
	s = strings.ReplaceAll(s, "\ufeff", "") // BOM
	return s
}

// ============================================================
// Step 3: JSON 解析
// ============================================================

// unmarshalJSON 解析 JSON 字符串
func (p Structured) unmarshalJSON(jsonStr string) (map[string]any, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, ParseError{
			Text:       jsonStr,
			Reason:     fmt.Sprintf("JSON 解析失败: %s", err.Error()),
			Suggestion: "检查 JSON 语法：引号匹配、逗号位置、无尾部逗号",
			Phase:      "unmarshal",
		}
	}
	return parsed, nil
}

// ============================================================
// Step 4: Schema 校验
// ============================================================

// validate 校验解析结果是否符合 Schema 定义
// 对标 OWASP: Deterministic code to validate adherence to formats
func (p Structured) validate(parsed map[string]any) error {
	var errors []string

	for _, rs := range p.ResponseSchemas {
		value, exists := parsed[rs.Name]

		// 必填校验
		if !exists || isEmptyValue(value) {
			if rs.Require {
				if rs.Default != nil {
					// 使用默认值填充，不报错
					parsed[rs.Name] = rs.Default
				} else {
					errors = append(errors, fmt.Sprintf("必填字段 %q 缺失", rs.Name))
				}
			}
			// 即使值存在为空，也要检查长度约束
			if rs.MinLen != nil || rs.MaxLen != nil {
				if err := validateLength(rs.Name, value, rs.MinLen, rs.MaxLen); err != nil {
					errors = append(errors, err.Error())
				}
			}
			continue
		}

		// 类型校验
		if err := validateFieldType(rs, value); err != nil {
			errors = append(errors, err.Error())
			continue
		}

		// 枚举值校验
		if len(rs.Enum) > 0 {
			if err := validateEnum(rs.Name, value, rs.Enum); err != nil {
				errors = append(errors, err.Error())
			}
		}

		// 范围校验（数值类型）
		if rs.Min != nil || rs.Max != nil {
			if err := validateRange(rs.Name, value, rs.Min, rs.Max); err != nil {
				errors = append(errors, err.Error())
			}
		}

		// 字符串长度校验
		if rs.MinLen != nil || rs.MaxLen != nil {
			if err := validateLength(rs.Name, value, rs.MinLen, rs.MaxLen); err != nil {
				errors = append(errors, err.Error())
			}
		}

		// 正则校验
		if rs.Pattern != "" {
			if err := validatePattern(rs.Name, value, rs.Pattern); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	if len(errors) > 0 {
		return ParseError{
			Text:       fmt.Sprintf("%v", parsed),
			Reason:     fmt.Sprintf("Schema 校验失败: %v", errors),
			Suggestion: "根据上述错误信息调整 LLM 输出格式",
			Phase:      "validate",
		}
	}

	return nil
}

// isEmptyValue 判断值是否为空
func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	default:
		return false
	}
}

// validateFieldType 校验字段类型
func validateFieldType(rs ResponseSchema, value any) error {
	if rs.Type == "" || rs.Type == "any" {
		return nil
	}

	var actualType string
	switch v := value.(type) {
	case string:
		actualType = "string"
	case float64:
		if rs.Type == "int" || rs.Type == "int64" {
			// JSON 解码后 number 都是 float64，检查是否为整数
			if v != float64(int64(v)) {
				return ValidationError{
					Field:    rs.Name,
					Expected: rs.Type,
					Actual:   "float",
				}
			}
			actualType = rs.Type
		} else {
			actualType = "float"
		}
	case bool:
		actualType = "bool"
	case []any:
		actualType = "array"
		// 检查数组元素类型
		if len(v) > 0 && rs.Type == "[]string" {
			for i, elem := range v {
				if _, ok := elem.(string); !ok {
					return ValidationError{
						Field:    fmt.Sprintf("%s[%d]", rs.Name, i),
						Expected: "string",
						Actual:   fmt.Sprintf("%T", elem),
					}
				}
			}
		}
	case map[string]any:
		actualType = "object"
	case nil:
		return nil
	}

	if !isTypeMatch(rs.Type, actualType) {
		return ValidationError{
			Field:    rs.Name,
			Expected: rs.Type,
			Actual:   actualType,
		}
	}

	return nil
}

// isTypeMatch 类型匹配判断
func isTypeMatch(expected, actual string) bool {
	typeAliases := map[string][]string{
		"string":   {"string"},
		"int":      {"int", "int64", "number"},
		"int64":    {"int", "int64", "number"},
		"float":    {"float", "float64", "number"},
		"float64":  {"float", "float64", "number"},
		"bool":     {"bool"},
		"array":    {"array"},
		"[]string": {"array"},
		"object":   {"object"},
	}

	aliases, ok := typeAliases[expected]
	if !ok {
		return expected == actual
	}
	for _, alias := range aliases {
		if alias == actual {
			return true
		}
	}
	return false
}

// validateEnum 校验枚举值
func validateEnum(fieldName string, value any, enum []string) error {
	var strVal string
	switch v := value.(type) {
	case string:
		strVal = v
	case float64:
		strVal = fmt.Sprintf("%.0f", v)
	default:
		return nil
	}

	for _, e := range enum {
		if strVal == e {
			return nil
		}
	}
	return ValidationError{
		Field:    fieldName,
		Expected: fmt.Sprintf("枚举值: %v", enum),
		Actual:   strVal,
	}
}

// validateRange 校验数值范围
func validateRange(fieldName string, value any, min, max *float64) error {
	num, ok := value.(float64)
	if !ok {
		return nil
	}
	if min != nil && num < *min {
		return ValidationError{
			Field:    fieldName,
			Expected: fmt.Sprintf(">= %v", *min),
			Actual:   fmt.Sprintf("%v", num),
		}
	}
	if max != nil && num > *max {
		return ValidationError{
			Field:    fieldName,
			Expected: fmt.Sprintf("<= %v", *max),
			Actual:   fmt.Sprintf("%v", num),
		}
	}
	return nil
}

// validateLength 校验字符串长度
func validateLength(fieldName string, value any, minLen, maxLen *int) error {
	str, ok := value.(string)
	if !ok {
		return nil
	}
	length := len([]rune(str)) // 按 Unicode 字符计数（支持中文）
	if minLen != nil && length < *minLen {
		return ValidationError{
			Field:    fieldName,
			Expected: fmt.Sprintf("长度 >= %d", *minLen),
			Actual:   fmt.Sprintf("长度 = %d", length),
		}
	}
	if maxLen != nil && length > *maxLen {
		return ValidationError{
			Field:    fieldName,
			Expected: fmt.Sprintf("长度 <= %d", *maxLen),
			Actual:   fmt.Sprintf("长度 = %d", length),
		}
	}
	return nil
}

// validatePattern 校验正则匹配
func validatePattern(fieldName string, value any, pattern string) error {
	str, ok := value.(string)
	if !ok {
		return nil
	}
	if !regexp.MustCompile(pattern).MatchString(str) {
		return ValidationError{
			Field:    fieldName,
			Expected: fmt.Sprintf("匹配正则: %s", pattern),
			Actual:   str,
		}
	}
	return nil
}

// ============================================================
// 辅助函数
// ============================================================

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// jsonMarshal 生成 JSON Schema 描述
func (p Structured) jsonMarshal(schemas []ResponseSchema, level int) string {
	level++

	endBlank := ""
	fieldBlank := "\t"

	for i := 0; i < level; i++ {
		fieldBlank += "\t"
		endBlank += "\t"
	}

	jsonLines := "{\n"
	for i, rs := range schemas {
		// 嵌套对象处理
		if len(rs.Schemas) > 0 {
			rs.Type = p.jsonMarshal(rs.Schemas, level)
		} else if rs.Type == "" {
			rs.Type = "string"
		}

		blank := fieldBlank
		if i == 0 {
			blank = "\t"
		}

		jsonLines += blank + fmt.Sprintf(
			_structuredLineTemplate,
			rs.Name,
			rs.Type,
			rs.Description,
		)
	}

	return jsonLines + endBlank + "}"
}

// ============================================================
// 常量定义
// ============================================================

const (
	// _structuredFormatInstructionTemplate 格式指令模板
	_structuredFormatInstructionTemplate = "The output should be a markdown code snippet formatted in the following schema: \n```json\n%s\n```" // nolint

	// _structuredLineTemplate JSON Schema 行模板
	_structuredLineTemplate = "\t\"%s\": %s, // %s\n"
)
