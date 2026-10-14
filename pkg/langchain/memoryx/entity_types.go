/**
 * @author: dn-jinmin/dn-jinmin
 * @doc: 结构化实体类型定义
 */

package memoryx

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// EntityType 实体类型
type EntityType string

const (
	EntityUserName   EntityType = "user_name"   // 对话昵称（用户后说的为准）
	EntityUserAge    EntityType = "user_age"    // 对话中的年龄
	EntityUserGender EntityType = "user_gender" // 对话中的性别
	EntityUserSeat   EntityType = "user_seat"   // 对话中的工位
	EntityCustom     EntityType = "custom"      // 自定义对话实体
)

// Entity 实体结构
type Entity struct {
	Type      EntityType       `json:"type"`                 // 实体类型
	Value     string          `json:"value"`                // 实体值
	Extracted time.Time       `json:"extracted_at"`         // 提取时间
	Metadata  map[string]any  `json:"metadata,omitempty"`   // 额外元数据
}

// FactProfile 账号事实，来自 user/部门表，对话抽取不能改。
type FactProfile struct {
	UserID   string `json:"user_id,omitempty"`
	Name     string `json:"name,omitempty"`
	DeptID   string `json:"dept_id,omitempty"`
	DeptName string `json:"dept_name,omitempty"`
}

func (p *FactProfile) HasData() bool {
	if p == nil {
		return false
	}
	return p.UserID != "" || p.Name != "" || p.DeptID != "" || p.DeptName != ""
}

// UserProfile 对话实体（结构化集合）
type UserProfile struct {
	Name   string            `json:"name,omitempty"`    // 姓名
	Age    int               `json:"age,omitempty"`     // 年龄
	Gender string            `json:"gender,omitempty"`   // 性别
	SeatNo string            `json:"seat_no,omitempty"` // 工位号
	Custom map[string]string `json:"custom,omitempty"`   // 自定义字段
}

// ToJSON 转为 JSON 格式（用于注入 SystemPrompt）
func (p *UserProfile) ToJSON() string {
	data, _ := json.Marshal(p)
	return string(data)
}

// HasData 检查是否有有效数据
func (p *UserProfile) HasData() bool {
	return p.Name != "" || p.Age != 0 || p.Gender != "" || p.SeatNo != "" || len(p.Custom) > 0
}

// MemoryContext 记忆上下文（用于注入到 SystemPrompt）
type MemoryContext struct {
	UserProfile    *UserProfile `json:"user_profile,omitempty"`    // 用户画像
	TaskContext    string       `json:"task_context,omitempty"`    // 当前任务上下文
	HistorySummary string       `json:"history_summary,omitempty"`  // 历史摘要
}

// ToJSON 转为格式化 JSON
func (m *MemoryContext) ToJSON() string {
	data, _ := json.MarshalIndent(m, "", "  ")
	return string(data)
}

// EntityStore 实体内存存储：facts 按 uid，dialogue 按 chatId。
type EntityStore struct {
	mu       sync.RWMutex
	facts    map[string]*FactProfile
	entities map[string]map[EntityType]*Entity // chatId -> 对话实体
}

func NewEntityStore() *EntityStore {
	return &EntityStore{
		facts:    make(map[string]*FactProfile),
		entities: make(map[string]map[EntityType]*Entity),
	}
}

// Set 保存对话实体。后写入覆盖先写入。不能改事实实体。
func (s *EntityStore) Set(chatId string, entity *Entity) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.entities[chatId] == nil {
		s.entities[chatId] = make(map[EntityType]*Entity)
	}
	// 自动设置提取时间
	if entity.Extracted.IsZero() {
		entity.Extracted = time.Now()
	}
	s.entities[chatId][entity.Type] = entity
}

// Get 获取单个实体
func (s *EntityStore) Get(chatId string, entityType EntityType) *Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if entities, ok := s.entities[chatId]; ok {
		if entity, ok := entities[entityType]; ok {
			return entity
		}
	}
	return nil
}

// GetUserProfile 获取对话实体画像（不含数据库事实）。
func (s *EntityStore) GetUserProfile(chatId string) *UserProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()

	profile := &UserProfile{}

	if entities, ok := s.entities[chatId]; ok {
		if name := entities[EntityUserName]; name != nil {
			profile.Name = name.Value
		}
		if age := entities[EntityUserAge]; age != nil {
			if v, ok := age.Metadata["int_value"].(int); ok {
				profile.Age = v
			}
		}
		if gender := entities[EntityUserGender]; gender != nil {
			profile.Gender = gender.Value
		}
		if seat := entities[EntityUserSeat]; seat != nil {
			profile.SeatNo = seat.Value
		}
		if custom, ok := entities[EntityCustom]; ok {
			if fields, ok := custom.Metadata["fields"].(map[string]string); ok {
				profile.Custom = fields
			}
		}
	}

	return profile
}

// SetFacts 写入账号事实。整份替换，不被 ExtractAndStore 覆盖。
func (s *EntityStore) SetFacts(uid string, fact FactProfile) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return
	}
	if fact.UserID == "" {
		fact.UserID = uid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := fact
	s.facts[uid] = &cp
}

// GetFacts 读取账号事实。
func (s *EntityStore) GetFacts(uid string) *FactProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if uid == "" {
		return &FactProfile{}
	}
	if fact, ok := s.facts[uid]; ok && fact != nil {
		cp := *fact
		return &cp
	}
	return &FactProfile{}
}

// SetUserProfile 批量设置对话实体画像
func (s *EntityStore) SetUserProfile(chatId string, profile *UserProfile) {
	if profile.Name != "" {
		s.Set(chatId, &Entity{
			Type:      EntityUserName,
			Value:     profile.Name,
			Extracted: time.Now(),
		})
	}
	if profile.Age > 0 {
		s.Set(chatId, &Entity{
			Type:      EntityUserAge,
			Value:     string(rune(profile.Age)),
			Extracted: time.Now(),
			Metadata:  map[string]any{"int_value": profile.Age},
		})
	}
	if profile.Gender != "" {
		s.Set(chatId, &Entity{
			Type:      EntityUserGender,
			Value:     profile.Gender,
			Extracted: time.Now(),
		})
	}
	if profile.SeatNo != "" {
		s.Set(chatId, &Entity{
			Type:      EntityUserSeat,
			Value:     profile.SeatNo,
			Extracted: time.Now(),
		})
	}
}

// Delete 删除会话的对话实体，不删除账号事实。
func (s *EntityStore) Delete(chatId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entities, chatId)
}

// Clear 清空对话实体和账号事实。
func (s *EntityStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.facts = make(map[string]*FactProfile)
	s.entities = make(map[string]map[EntityType]*Entity)
}
