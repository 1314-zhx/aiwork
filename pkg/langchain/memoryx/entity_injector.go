/**
 * @author: dn-jinmin/dn-jinmin
 * @doc: 实体注入器 - 将结构化实体注入到 SystemPrompt 的最高优先级位置
 */

package memoryx

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

// EntityInjector 实体注入器
type EntityInjector struct {
	store *EntityStore
	model string
}

// NewEntityInjector 创建实体注入器
func NewEntityInjector(store *EntityStore, model string) *EntityInjector {
	return &EntityInjector{
		store: store,
		model: model,
	}
}

// Inject 将事实身份与对话实体注入到 baseSystemPrompt
func (i *EntityInjector) Inject(ctx context.Context, uid, chatId string, baseSystemPrompt string) string {
	_ = ctx
	if uid == "" {
		uid = chatId
	}
	facts := i.store.GetFacts(uid)
	dialogue := i.store.GetUserProfile(chatId)

	var parts []string
	if facts.HasData() {
		parts = append(parts, i.buildFactContext(facts))
	}
	if dialogue != nil && dialogue.HasData() {
		parts = append(parts, i.buildDialogueContext(dialogue))
	}
	if baseSystemPrompt != "" {
		parts = append(parts, baseSystemPrompt)
	}
	return strings.Join(parts, "\n\n")
}

// InjectWithSummary 将实体和摘要一起注入
func (i *EntityInjector) InjectWithSummary(ctx context.Context, uid, chatId string, baseSystemPrompt string, summary string) string {
	injected := i.Inject(ctx, uid, chatId, "")
	var parts []string
	if injected != "" {
		parts = append(parts, injected)
	}
	if summary != "" {
		parts = append(parts, fmt.Sprintf("## 历史对话摘要\n%s", summary))
	}
	if baseSystemPrompt != "" {
		parts = append(parts, baseSystemPrompt)
	}
	return strings.Join(parts, "\n\n")
}

func (i *EntityInjector) buildFactContext(profile *FactProfile) string {
	body, _ := json.MarshalIndent(profile, "", "  ")
	return "## 事实身份（来自账号，不可被对话修改；权限与租户以此为准）\n```json\n" + string(body) + "\n```"
}

func (i *EntityInjector) buildDialogueContext(profile *UserProfile) string {
	body, _ := json.MarshalIndent(profile, "", "  ")
	return "## 对话实体（本会话，以用户后说的为准；仅用于称呼和临时事实）\n```json\n" + string(body) + "\n```"
}

// InjectToMessages 将实体注入到消息列表
func (i *EntityInjector) InjectToMessages(ctx context.Context, chatId string, messages []llms.ChatMessage, baseSystemPrompt string) ([]llms.ChatMessage, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	var systemMsgContent string
	var newMessages []llms.ChatMessage

	for _, msg := range messages {
		if _, ok := msg.(llms.SystemChatMessage); ok {
			systemMsgContent = msg.GetContent()
		} else {
			newMessages = append(newMessages, msg)
		}
	}

	systemPrompt := baseSystemPrompt
	if systemMsgContent != "" {
		systemPrompt = systemMsgContent
	}

	injectedPrompt := i.Inject(ctx, "", chatId, systemPrompt)

	result := []llms.ChatMessage{
		llms.SystemChatMessage{Content: injectedPrompt},
	}
	result = append(result, newMessages...)

	return result, nil
}

// ExtractAndStore 从对话中提取实体并存储
func (i *EntityInjector) ExtractAndStore(chatId string, userInput string) {
	extractor := NewThreadSafeExtractor()
	extractor.ExtractAndStore(chatId, i.store, userInput)
}

// GetMemoryContext 获取记忆上下文
func (i *EntityInjector) GetMemoryContext(chatId string, summary string) *MemoryContext {
	return &MemoryContext{
		UserProfile:    i.store.GetUserProfile(chatId),
		HistorySummary: summary,
	}
}

// SetUserProfile 手动设置用户画像
func (i *EntityInjector) SetUserProfile(chatId string, profile *UserProfile) {
	i.store.SetUserProfile(chatId, profile)
}

// ClearChatMemory 清除会话记忆
func (i *EntityInjector) ClearChatMemory(chatId string) {
	i.store.Delete(chatId)
}
