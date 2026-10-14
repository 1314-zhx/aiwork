package memoryx

import (
	"testing"
	"time"
)

func TestEntityExtractor_ExtractFromInput(t *testing.T) {
	extractor := NewEntityExtractor()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "提取姓名",
			input:    "我叫张三",
			expected: 1,
		},
		{
			name:     "提取年龄",
			input:    "我今年25岁",
			expected: 1,
		},
		{
			name:     "提取工位号",
			input:    "我的工位是A-101",
			expected: 1,
		},
		{
			name:     "同时提取多个实体",
			input:    "我叫张三，今年25岁，工位号A-101",
			expected: 3,
		},
		{
			name:     "空输入",
			input:    "",
			expected: 0,
		},
		{
			name:     "无实体输入",
			input:    "今天天气真好",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := extractor.ExtractFromInput(tt.input)
			if len(results) != tt.expected {
				t.Errorf("期望提取到 %d 个实体，实际提取到 %d 个", tt.expected, len(results))
			}
		})
	}
}

func TestEntityStore_SetAndGet(t *testing.T) {
	store := NewEntityStore()
	chatId := "test-chat-001"

	store.Set(chatId, &Entity{
		Type:  EntityUserName,
		Value: "张三",
	})

	entity := store.Get(chatId, EntityUserName)
	if entity == nil {
		t.Fatal("期望获取到实体，实际为 nil")
	}
	if entity.Value != "张三" {
		t.Errorf("期望值为 '张三'，实际为 '%s'", entity.Value)
	}

	nonExist := store.Get(chatId, EntityUserAge)
	if nonExist != nil {
		t.Error("期望获取到 nil，实际不为 nil")
	}
}

func TestEntityStore_GetUserProfile(t *testing.T) {
	store := NewEntityStore()
	chatId := "test-chat-002"

	store.Set(chatId, &Entity{
		Type:  EntityUserName,
		Value: "李四",
	})
	store.Set(chatId, &Entity{
		Type:     EntityUserAge,
		Value:    "30",
		Metadata: map[string]any{"int_value": 30},
	})
	store.Set(chatId, &Entity{
		Type:  EntityUserGender,
		Value: "男",
	})
	store.Set(chatId, &Entity{
		Type:  EntityUserSeat,
		Value: "B-202",
	})

	profile := store.GetUserProfile(chatId)

	if profile.Name != "李四" {
		t.Errorf("期望姓名为 '李四'，实际为 '%s'", profile.Name)
	}
	if profile.Age != 30 {
		t.Errorf("期望年龄为 30，实际为 %d", profile.Age)
	}
	if profile.Gender != "男" {
		t.Errorf("期望性别为 '男'，实际为 '%s'", profile.Gender)
	}
	if profile.SeatNo != "B-202" {
		t.Errorf("期望工位号为 'B-202'，实际为 '%s'", profile.SeatNo)
	}
}

func TestUserProfile_HasData(t *testing.T) {
	tests := []struct {
		name     string
		profile  *UserProfile
		expected bool
	}{
		{name: "空画像", profile: &UserProfile{}, expected: false},
		{name: "有姓名", profile: &UserProfile{Name: "张三"}, expected: true},
		{name: "有年龄", profile: &UserProfile{Age: 25}, expected: true},
		{name: "有性别", profile: &UserProfile{Gender: "男"}, expected: true},
		{name: "有工位号", profile: &UserProfile{SeatNo: "A-101"}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.profile.HasData() != tt.expected {
				t.Errorf("期望 HasData() = %v，实际 = %v", tt.expected, tt.profile.HasData())
			}
		})
	}
}

func TestEntityStore_Delete(t *testing.T) {
	store := NewEntityStore()
	chatId := "test-chat-003"

	store.Set(chatId, &Entity{
		Type:  EntityUserName,
		Value: "测试",
	})

	if store.Get(chatId, EntityUserName) == nil {
		t.Fatal("实体应该存在")
	}

	store.Delete(chatId)

	if store.Get(chatId, EntityUserName) != nil {
		t.Error("实体应该已被删除")
	}
}

func TestEntityStore_Clear(t *testing.T) {
	store := NewEntityStore()

	store.Set("chat-1", &Entity{Type: EntityUserName, Value: "用户1"})
	store.Set("chat-2", &Entity{Type: EntityUserName, Value: "用户2"})

	store.Clear()

	if store.Get("chat-1", EntityUserName) != nil {
		t.Error("chat-1 的实体应该已被清空")
	}
	if store.Get("chat-2", EntityUserName) != nil {
		t.Error("chat-2 的实体应该已被清空")
	}
}

func TestEntityInjector_Inject(t *testing.T) {
	store := NewEntityStore()
	chatId := "test-chat-004"

	store.SetUserProfile(chatId, &UserProfile{
		Name:   "王五",
		Age:    28,
		Gender: "女",
		SeatNo: "C-303",
	})

	injector := NewEntityInjector(store, "gpt-4")
	basePrompt := "你是一个AI助手。"

	result := injector.Inject(nil, "", chatId, basePrompt)

	if result == "" {
		t.Fatal("注入后的 prompt 不应为空")
	}

	if result == basePrompt {
		t.Error("注入后的 prompt 应该包含用户信息")
	}
}

func TestEntityInjector_Inject_NoProfile(t *testing.T) {
	store := NewEntityStore()
	injector := NewEntityInjector(store, "gpt-4")
	basePrompt := "你是一个AI助手。"

	result := injector.Inject(nil, "", "no-profile-chat", basePrompt)

	if result != basePrompt {
		t.Error("没有用户画像时应该返回原 prompt")
	}
}

func TestMemoryContext_ToJSON(t *testing.T) {
	ctx := &MemoryContext{
		UserProfile: &UserProfile{
			Name:   "赵六",
			Age:    35,
			Gender: "男",
		},
		HistorySummary: "用户之前询问过天气问题。",
	}

	json := ctx.ToJSON()
	if json == "" {
		t.Error("JSON 不应为空")
	}
}

func TestEntity_ExtractedTime(t *testing.T) {
	store := NewEntityStore()
	chatId := "test-time-chat"

	time.Sleep(time.Millisecond) // 确保时间戳有差异
	store.Set(chatId, &Entity{
		Type:  EntityUserName,
		Value: "测试",
	})

	entity := store.Get(chatId, EntityUserName)
	if entity == nil {
		t.Fatal("实体应该存在")
	}

	// 只验证 Extracted 不为零
	if entity.Extracted.IsZero() {
		t.Error("Extracted 时间应该被设置")
	}
}
