package memoryx

import (
	"context"
	"errors"
	"strings"

	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/token"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/memory"
)

type outParser func(ctx context.Context, input string) string

type SummarySegment struct {
	Content   string
	Catalog   string
	StartTurn int
	EndTurn   int
	Tokens    int
}

type SummaryBuffer struct {
	outParser
	*memory.ConversationBuffer
	level1Chain  chains.Chain
	level2Chain  chains.Chain
	callback     callbacks.Handler
	model        string
	entityStore  *EntityStore
	entityInject bool

	MaxTokenLimit        int
	SummaryMaxTokens     int
	CompressCooldown     int
	ProtectedRecentTurns int
	Level0MaxTokens      int
	Level1MaxTokens      int
	Level2MaxTokens      int
	TotalMemoryMaxTokens int
	Level0BatchTurns     int

	level1Segments   []SummarySegment
	level2Summary    string
	level2Catalog    string
	lastCompressTurn int
	turnCounter      int
}

func NewSummaryBuffer(model llms.Model, maxTokenLimit int, opts ...Option) *SummaryBuffer {
	opt := newOption(opts...)
	level1Limit := opt.Level1MaxTokens
	if level1Limit <= 0 {
		level1Limit = opt.SummaryMaxTokens
	}
	return &SummaryBuffer{
		ConversationBuffer:   memory.NewConversationBuffer(),
		level1Chain:          chains.NewLLMChain(model, createLevel1Prompt(), chains.WithCallback(opt.callback)),
		level2Chain:          chains.NewLLMChain(model, createLevel2Prompt(), chains.WithCallback(opt.callback)),
		callback:             opt.callback,
		model:                opt.model,
		MaxTokenLimit:        maxTokenLimit,
		SummaryMaxTokens:     opt.SummaryMaxTokens,
		CompressCooldown:     opt.CompressCooldown,
		ProtectedRecentTurns: opt.ProtectedRecentTurns,
		Level0MaxTokens:      opt.Level0MaxTokens,
		Level1MaxTokens:      level1Limit,
		Level2MaxTokens:      opt.Level2MaxTokens,
		TotalMemoryMaxTokens: opt.TotalMemoryMaxTokens,
		Level0BatchTurns:     opt.Level0BatchTurns,
		outParser:            opt.outParser,
		entityStore:          opt.entityStore,
		entityInject:         opt.entityStore != nil,
	}
}

func WithEntityStore(store *EntityStore) Option {
	return func(options *Options) { options.entityStore = store }
}

func (s *SummaryBuffer) GetMemoryKey(ctx context.Context) string {
	return s.ConversationBuffer.GetMemoryKey(ctx)
}

func (s *SummaryBuffer) MemoryVariables(ctx context.Context) []string {
	return s.ConversationBuffer.MemoryVariables(ctx)
}

func (s *SummaryBuffer) entityContext(ctx context.Context) string {
	if !s.entityInject || s.entityStore == nil {
		return ""
	}
	chatID := getChatIdFromContext(ctx)
	uid := token.GetUId(ctx)
	if uid == "" {
		uid = chatID
	}
	if chatID == "" && uid == "" {
		return ""
	}
	facts := s.entityStore.GetFacts(uid)
	dialogue := s.entityStore.GetUserProfile(chatID)
	if !facts.HasData() && (dialogue == nil || !dialogue.HasData()) {
		return ""
	}
	return NewEntityInjector(s.entityStore, s.model).Inject(ctx, uid, chatID, "")
}

func (s *SummaryBuffer) level1Text() string {
	parts := make([]string, 0, len(s.level1Segments))
	for _, segment := range s.level1Segments {
		parts = append(parts, segment.Content)
	}
	return strings.Join(parts, "\n")
}

func (s *SummaryBuffer) level1CatalogText() string {
	parts := make([]string, 0, len(s.level1Segments))
	for _, segment := range s.level1Segments {
		if item := strings.TrimSpace(segment.Catalog); item != "" {
			parts = append(parts, "- "+item)
		}
	}
	return strings.Join(parts, "\n")
}

func (s *SummaryBuffer) memoryMessages(ctx context.Context, recent []llms.ChatMessage, inputs map[string]any) []llms.ChatMessage {
	mode := loadModeFrom(inputs)
	query := queryFrom(inputs)
	messages := make([]llms.ChatMessage, 0, len(recent)+5)
	if entity := s.entityContext(ctx); entity != "" {
		messages = append(messages, &llms.SystemChatMessage{Content: entity})
	}
	if mode == LoadModeL0 {
		return append(messages, recent...)
	}

	if s.level2Catalog != "" {
		messages = append(messages, &llms.SystemChatMessage{Content: "## 长期记忆目录\n" + s.level2Catalog})
	}
	if catalogRelevant(query, s.level2Catalog) && s.level2Summary != "" {
		messages = append(messages, &llms.SystemChatMessage{Content: "## 长期事件记忆\n" + s.level2Summary})
	}
	if level1Cat := s.level1CatalogText(); level1Cat != "" {
		messages = append(messages, &llms.SystemChatMessage{Content: "## 近期记忆目录\n" + level1Cat})
	}
	for _, segment := range s.level1Segments {
		if catalogRelevant(query, segment.Catalog) && strings.TrimSpace(segment.Content) != "" {
			messages = append(messages, &llms.SystemChatMessage{Content: "## 近期记忆\n" + segment.Content})
		}
	}
	return append(messages, recent...)
}

func (s *SummaryBuffer) memoryTokenCount(ctx context.Context, recent []llms.ChatMessage, currentInput string) (int, error) {
	text, err := llms.GetBufferString(s.memoryMessages(ctx, recent, map[string]any{
		LoadModeKey: LoadModeCatalog,
		"input":     currentInput,
	}), s.HumanPrefix, s.AIPrefix)
	if err != nil {
		return 0, err
	}
	return llms.CountTokens(s.model, text+"\n"+currentInput), nil
}

func (s *SummaryBuffer) EnsureTokenBudget(ctx context.Context, currentInput string, maxTokens int) error {
	if maxTokens <= 0 {
		return nil
	}
	messages, err := s.ChatHistory.Messages(ctx)
	if err != nil {
		return err
	}
	tokens, err := s.memoryTokenCount(ctx, messages, currentInput)
	if err != nil {
		return err
	}
	if tokens <= maxTokens {
		return nil
	}

	if turnCount(messages) > s.protectedTurns() {
		migrated, remaining, err := s.compressLevel0(ctx, messages)
		if err != nil {
			return err
		}
		if migrated {
			messages = remaining
			if err := s.ChatHistory.SetMessages(ctx, messages); err != nil {
				return err
			}
		}
	}
	if err := s.compactLevel1(ctx); err != nil {
		return err
	}
	tokens, err = s.memoryTokenCount(ctx, messages, currentInput)
	if err != nil {
		return err
	}
	if tokens <= maxTokens {
		return nil
	}
	return errors.New("当前会话上下文过长，且最近一轮无法继续安全压缩")
}

func (s *SummaryBuffer) LoadMemoryVariables(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	recent, err := s.ChatHistory.Messages(ctx)
	if err != nil {
		return nil, err
	}
	text, err := llms.GetBufferString(s.memoryMessages(ctx, recent, inputs), s.HumanPrefix, s.AIPrefix)
	if err != nil {
		return nil, err
	}
	return map[string]any{s.MemoryKey: text}, nil
}

func (s *SummaryBuffer) SaveContext(ctx context.Context, inputs map[string]any, outputs map[string]any) error {
	userInput, err := memory.GetInputValue(inputs, s.InputKey)
	if err != nil {
		return err
	}
	if s.entityInject && s.entityStore != nil {
		s.extractDialogueEntities(ctx, userInput)
	}
	if err := s.ChatHistory.AddUserMessage(ctx, userInput); err != nil {
		return err
	}
	aiOutput, err := memory.GetInputValue(outputs, s.OutputKey)
	if err != nil {
		return err
	}
	if s.outParser != nil {
		aiOutput = s.outParser(ctx, aiOutput)
	}
	if err := s.ChatHistory.AddAIMessage(ctx, aiOutput); err != nil {
		return err
	}
	s.turnCounter++

	messages, err := s.ChatHistory.Messages(ctx)
	if err != nil {
		return err
	}
	level0Tokens, err := messagesTokenCount(s.model, messages, s.HumanPrefix, s.AIPrefix)
	if err != nil {
		return err
	}
	if level0Tokens <= s.Level0MaxTokens || s.Level0MaxTokens <= 0 {
		return s.compactLevel1(ctx)
	}
	if s.CompressCooldown > 0 && s.turnCounter-s.lastCompressTurn < s.CompressCooldown {
		return nil
	}

	migrated, remaining, err := s.compressLevel0(ctx, messages)
	if err != nil {
		return err
	}
	if !migrated {
		return s.compactLevel1(ctx)
	}
	if err := s.ChatHistory.SetMessages(ctx, remaining); err != nil {
		return err
	}
	s.lastCompressTurn = s.turnCounter
	return s.compactLevel1(ctx)
}

func (s *SummaryBuffer) protectedTurns() int {
	if s.ProtectedRecentTurns <= 0 {
		return 1
	}
	return s.ProtectedRecentTurns
}

// compressLevel0 按层压缩：保留受保护近轮，其余全部一次压进 Level 1 并生成目录项。
func (s *SummaryBuffer) compressLevel0(ctx context.Context, messages []llms.ChatMessage) (bool, []llms.ChatMessage, error) {
	available := turnCount(messages) - s.protectedTurns()
	if available <= 0 && turnCount(messages) > 1 {
		available = turnCount(messages) - 1
	}
	if available <= 0 {
		return false, messages, nil
	}
	return s.summarizeOldestTurns(ctx, messages, available)
}

func (s *SummaryBuffer) summarizeOldestTurns(ctx context.Context, messages []llms.ChatMessage, turns int) (bool, []llms.ChatMessage, error) {
	end := messageIndexAfterTurns(messages, turns)
	if end <= 0 {
		return false, messages, nil
	}
	conversation, err := llms.GetBufferString(messages[:end], s.HumanPrefix, s.AIPrefix)
	if err != nil {
		return false, messages, err
	}
	summary, err := chains.Predict(ctx, s.level1Chain, map[string]any{"conversation": conversation}, chains.WithMaxTokens(tokencost.SummaryMaxTokens))
	if err != nil {
		return false, messages, err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return false, messages, errors.New("Level 1 摘要为空")
	}
	catalog, body := splitCatalogBody(summary)
	coveredTurns := countHumanMessages(messages[:end])
	start := 1
	if len(s.level1Segments) > 0 {
		start = s.level1Segments[len(s.level1Segments)-1].EndTurn + 1
	} else if s.level2Summary != "" {
		start = s.turnCounter - turnCount(messages) + 1
	}
	s.level1Segments = append(s.level1Segments, SummarySegment{
		Content:   body,
		Catalog:   catalog,
		StartTurn: start,
		EndTurn:   start + coveredTurns - 1,
		Tokens:    llms.CountTokens(s.model, body),
	})
	return true, messages[end:], nil
}

func (s *SummaryBuffer) compactLevel1(ctx context.Context) error {
	if s.Level1MaxTokens <= 0 {
		return nil
	}
	if llms.CountTokens(s.model, s.level1Text()) <= s.Level1MaxTokens {
		return nil
	}
	return s.promoteLevel1(ctx)
}

func (s *SummaryBuffer) promoteLevel1(ctx context.Context) error {
	if len(s.level1Segments) == 0 {
		return nil
	}
	incomingParts := make([]string, 0, len(s.level1Segments))
	for _, segment := range s.level1Segments {
		incomingParts = append(incomingParts, segment.Content)
	}
	newCold, err := chains.Predict(ctx, s.level2Chain, map[string]any{
		"cold_summary":       s.level2Summary,
		"incoming_summaries": strings.Join(incomingParts, "\n"),
	}, chains.WithMaxTokens(tokencost.SummaryMaxTokens))
	if err != nil {
		return err
	}
	newCold = strings.TrimSpace(newCold)
	if newCold == "" {
		return errors.New("Level 2 摘要为空")
	}
	catalog, body := splitCatalogBody(newCold)
	s.level2Summary = body
	s.level2Catalog = catalog
	s.level1Segments = nil
	if s.Level2MaxTokens > 0 && llms.CountTokens(s.model, s.level2Summary) > s.Level2MaxTokens {
		return s.compactLevel2(ctx)
	}
	return nil
}

func (s *SummaryBuffer) compactLevel2(ctx context.Context) error {
	if s.level2Summary == "" {
		return nil
	}
	compacted, err := chains.Predict(ctx, s.level2Chain, map[string]any{
		"cold_summary":       "",
		"incoming_summaries": s.level2Summary,
	}, chains.WithMaxTokens(tokencost.SummaryMaxTokens))
	if err != nil {
		return err
	}
	compacted = strings.TrimSpace(compacted)
	if compacted == "" {
		return errors.New("Level 2 自压缩摘要为空")
	}
	s.level2Summary = compacted
	if catalog, body := splitCatalogBody(compacted); catalog != "" {
		s.level2Catalog = catalog
		s.level2Summary = body
	}
	return nil
}

func (s *SummaryBuffer) totalSummaryTokens() int {
	return llms.CountTokens(s.model, s.level1Text()) + llms.CountTokens(s.model, s.level2Summary)
}

func messagesTokenCount(model string, messages []llms.ChatMessage, humanPrefix, aiPrefix string) (int, error) {
	text, err := llms.GetBufferString(messages, humanPrefix, aiPrefix)
	if err != nil {
		return 0, err
	}
	return llms.CountTokens(model, text), nil
}

func turnCount(messages []llms.ChatMessage) int { return countHumanMessages(messages) }

func countHumanMessages(messages []llms.ChatMessage) int {
	count := 0
	for _, message := range messages {
		if message.GetType() == llms.ChatMessageTypeHuman {
			count++
		}
	}
	return count
}

func messageIndexAfterTurns(messages []llms.ChatMessage, turns int) int {
	if turns <= 0 {
		return 0
	}
	humans := 0
	for i, message := range messages {
		if message.GetType() == llms.ChatMessageTypeHuman {
			if humans == turns {
				return i
			}
			humans++
		}
	}
	if humans >= turns {
		return len(messages)
	}
	return 0
}

func (s *SummaryBuffer) extractDialogueEntities(ctx context.Context, input string) {
	chatID := getChatIdFromContext(ctx)
	if chatID == "" {
		return
	}
	NewThreadSafeExtractor().ExtractAndStore(chatID, s.entityStore, input)
}

func getChatIdFromContext(ctx context.Context) string {
	if value := ctx.Value(langchain.ChatId); value != nil {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return ""
}

func (s *SummaryBuffer) Clear(ctx context.Context) error {
	s.level1Segments = nil
	s.level2Summary = ""
	s.level2Catalog = ""
	s.lastCompressTurn = 0
	s.turnCounter = 0
	if s.entityStore != nil {
		if chatID := getChatIdFromContext(ctx); chatID != "" {
			s.entityStore.Delete(chatID)
		}
	}
	return s.ConversationBuffer.Clear(ctx)
}
