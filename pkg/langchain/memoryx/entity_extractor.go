/**
 * @author: dn-jinmin/dn-jinmin
 * @doc: 实体提取器 - 通过规则匹配从对话中提取关键实体信息
 */

package memoryx

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ExtractResult 提取结果
type ExtractResult struct {
	Type  EntityType
	Value string
}

// EntityExtractor 实体提取器
type EntityExtractor struct {
	patterns map[EntityType]*regexp.Regexp
}

func NewEntityExtractor() *EntityExtractor {
	return &EntityExtractor{
		patterns: map[EntityType]*regexp.Regexp{
			// 对话昵称：匹配"我叫xxx"、"我是xxx"、"名字是xxx"等模式
			EntityUserName: regexp.MustCompile(`(?:我(?:叫|是|的名字?)|名字[是为：:]\s*)([^\s,，,。]+)`),

			// 年龄提取：匹配"我xx岁"、"年龄xx"等模式
			EntityUserAge: regexp.MustCompile(`(?:(?:我)?(?:的)?年龄?[是为：:]\s*)?(\d{1,3})\s*(?:岁|周)`),

			// 性别提取：匹配"我是男/女"等模式
			EntityUserGender: regexp.MustCompile(`(?:性别[是为：:\s]*)?(男|女)(?:性)?`),

			// 工位号提取：匹配"A-101"、"座位号xxx"等模式
			EntityUserSeat: regexp.MustCompile(`(?:工位|座位|工号|座位号)[是为：:\s]*([A-Za-z0-9\-]+)|([A-Z]-\d{2,4})`),
		},
	}
}

// ExtractFromInput 从用户输入中提取对话实体（不读 user 表，不写事实身份）
func (e *EntityExtractor) ExtractFromInput(input string) []*ExtractResult {
	var results []*ExtractResult

	input = strings.TrimSpace(input)
	if input == "" {
		return results
	}

	// 提取姓名
	if match := e.patterns[EntityUserName].FindStringSubmatch(input); len(match) > 1 {
		results = append(results, &ExtractResult{
			Type:  EntityUserName,
			Value: match[1],
		})
	}

	// 提取年龄
	if match := e.patterns[EntityUserAge].FindStringSubmatch(input); len(match) > 1 {
		ageStr := match[1]
		if age, err := strconv.Atoi(ageStr); err == nil && age > 0 && age < 150 {
			results = append(results, &ExtractResult{
				Type:  EntityUserAge,
				Value: ageStr,
			})
		}
	}

	// 提取性别
	if match := e.patterns[EntityUserGender].FindStringSubmatch(input); len(match) > 1 {
		gender := match[1]
		results = append(results, &ExtractResult{
			Type:  EntityUserGender,
			Value: gender,
		})
	}

	// 提取工位号
	if match := e.patterns[EntityUserSeat].FindStringSubmatch(input); len(match) > 1 {
		seat := match[1]
		if seat == "" {
			seat = match[2]
		}
		if seat != "" {
			results = append(results, &ExtractResult{
				Type:  EntityUserSeat,
				Value: seat,
			})
		}
	}

	return results
}

// ExtractAndStore 从输入中提取并存储到 EntityStore
func (e *EntityExtractor) ExtractAndStore(chatId string, store *EntityStore, input string) {
	results := e.ExtractFromInput(input)

	for _, r := range results {
		entity := &Entity{
			Type:  r.Type,
			Value: r.Value,
		}
		if r.Type == EntityUserAge {
			if age, err := strconv.Atoi(r.Value); err == nil {
				entity.Metadata = map[string]any{"int_value": age}
			}
		}
		store.Set(chatId, entity)
	}
}

// ThreadSafeExtractor 线程安全的实体提取器
type ThreadSafeExtractor struct {
	*EntityExtractor
	mu sync.Mutex
}

func NewThreadSafeExtractor() *ThreadSafeExtractor {
	return &ThreadSafeExtractor{
		EntityExtractor: NewEntityExtractor(),
	}
}

// Extract 线程安全提取
func (e *ThreadSafeExtractor) Extract(input string) []*ExtractResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.EntityExtractor.ExtractFromInput(input)
}

// ExtractAndStore 线程安全提取并存储
func (e *ThreadSafeExtractor) ExtractAndStore(chatId string, store *EntityStore, input string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.EntityExtractor.ExtractAndStore(chatId, store, input)
}
