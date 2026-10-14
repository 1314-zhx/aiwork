package memoryx

import (
	"context"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/schema"
)

type staticTextChain struct {
	text string
}

func (c staticTextChain) Call(_ context.Context, _ map[string]any, _ ...chains.ChainCallOption) (map[string]any, error) {
	return map[string]any{"text": c.text}, nil
}
func (c staticTextChain) GetMemory() schema.Memory { return memory.NewSimple() }
func (c staticTextChain) GetInputKeys() []string   { return nil }
func (c staticTextChain) GetOutputKeys() []string  { return []string{"text"} }

func testBuffer() *SummaryBuffer {
	return &SummaryBuffer{
		ConversationBuffer:   memory.NewConversationBuffer(),
		model:                "qwen-turbo",
		ProtectedRecentTurns: 1,
		Level0MaxTokens:      40,
		Level1MaxTokens:      80,
		Level2MaxTokens:      80,
		level1Chain:          staticTextChain{text: "CATALOG: 请假草稿 · 明天\nSUMMARY:\n用户要请假，时间明天。"},
		level2Chain:          staticTextChain{text: "CATALOG: 办公事项归档\nMEMORY:\n请假与审批已归档。"},
	}
}

func TestSplitCatalogBody(t *testing.T) {
	cat, body := splitCatalogBody("CATALOG: 请假 · 明天\nSUMMARY:\n用户要请假")
	if cat != "请假 · 明天" {
		t.Fatalf("catalog=%q", cat)
	}
	if !strings.Contains(body, "用户要请假") {
		t.Fatalf("body=%q", body)
	}
}

func TestCatalogRelevantUsesBigrams(t *testing.T) {
	if !catalogRelevant("上次请假定哪天", "请假草稿 · 明天") {
		t.Fatal("expected 请假 to hit catalog")
	}
	if catalogRelevant("今天天气怎么样", "请假草稿 · 明天") {
		t.Fatal("unrelated query should not hit")
	}
}

func TestLoadModeL0OmitsCatalogAndBodies(t *testing.T) {
	buf := testBuffer()
	ctx := context.Background()
	if err := buf.ChatHistory.AddUserMessage(ctx, "改成后天"); err != nil {
		t.Fatal(err)
	}
	if err := buf.ChatHistory.AddAIMessage(ctx, "好的"); err != nil {
		t.Fatal(err)
	}
	buf.level1Segments = []SummarySegment{{
		Content: "用户要请假，时间明天。含内部id=abc",
		Catalog: "请假草稿 · 明天",
	}}
	buf.level2Summary = "去年述职结论待出"
	buf.level2Catalog = "述职 · 未完成"

	vars, err := buf.LoadMemoryVariables(ctx, map[string]any{LoadModeKey: LoadModeL0})
	if err != nil {
		t.Fatal(err)
	}
	history := vars[buf.MemoryKey].(string)
	if !strings.Contains(history, "改成后天") {
		t.Fatalf("L0 missing: %s", history)
	}
	for _, leak := range []string{"请假草稿", "述职", "内部id", "去年述职"} {
		if strings.Contains(history, leak) {
			t.Fatalf("闲聊不应加载办公记忆 %q: %s", leak, history)
		}
	}
}

func TestLoadModeCatalogHidesBodiesUntilHit(t *testing.T) {
	buf := testBuffer()
	ctx := context.Background()
	if err := buf.ChatHistory.AddUserMessage(ctx, "你好"); err != nil {
		t.Fatal(err)
	}
	if err := buf.ChatHistory.AddAIMessage(ctx, "你好"); err != nil {
		t.Fatal(err)
	}
	buf.level1Segments = []SummarySegment{{
		Content: "用户要请假，时间明天。内部备注secret",
		Catalog: "请假草稿 · 明天",
	}}
	buf.level2Catalog = "述职 · 未完成"
	buf.level2Summary = "述职正文不应默认出现"

	vars, err := buf.LoadMemoryVariables(ctx, map[string]any{
		LoadModeKey: LoadModeCatalog,
		"input":     "帮我写个祝福",
	})
	if err != nil {
		t.Fatal(err)
	}
	history := vars[buf.MemoryKey].(string)
	if !strings.Contains(history, "请假草稿") || !strings.Contains(history, "述职") {
		t.Fatalf("catalog should be visible: %s", history)
	}
	if strings.Contains(history, "secret") || strings.Contains(history, "述职正文") {
		t.Fatalf("bodies should stay unloaded: %s", history)
	}

	vars, err = buf.LoadMemoryVariables(ctx, map[string]any{
		LoadModeKey: LoadModeCatalog,
		"input":     "上次请假定哪天",
	})
	if err != nil {
		t.Fatal(err)
	}
	history = vars[buf.MemoryKey].(string)
	if !strings.Contains(history, "secret") {
		t.Fatalf("matching query should expand L1 body: %s", history)
	}
	if strings.Contains(history, "述职正文") {
		t.Fatalf("unmatched L2 body should stay closed: %s", history)
	}
}

func TestCompressLevel0KeepsProtectedTurn(t *testing.T) {
	buf := testBuffer()
	buf.Level0MaxTokens = 100000
	ctx := context.Background()
	turns := [][2]string{
		{"第一轮办公请假", "已记录"},
		{"第二轮改时间", "改到周三"},
		{"第三轮确认", "好的"},
	}
	for _, turn := range turns {
		if err := buf.SaveContext(ctx, map[string]any{"input": turn[0]}, map[string]any{"output": turn[1]}); err != nil {
			t.Fatal(err)
		}
	}
	if turnCountMust(t, buf, ctx) != 3 {
		t.Fatalf("expected 3 L0 turns before overflow, got %d", turnCountMust(t, buf, ctx))
	}

	buf.Level0MaxTokens = 1
	if err := buf.SaveContext(ctx, map[string]any{"input": "第四轮继续"}, map[string]any{"output": "收到"}); err != nil {
		t.Fatal(err)
	}
	if turnCountMust(t, buf, ctx) != 1 {
		t.Fatalf("expected 1 protected L0 turn, got %d", turnCountMust(t, buf, ctx))
	}
	if len(buf.level1Segments) != 1 {
		t.Fatalf("overflow should compress into one L1 catalog segment, got %d", len(buf.level1Segments))
	}
	if buf.level1Segments[0].Catalog != "请假草稿 · 明天" {
		t.Fatalf("catalog=%q", buf.level1Segments[0].Catalog)
	}
	vars, err := buf.LoadMemoryVariables(ctx, map[string]any{LoadModeKey: LoadModeL0})
	if err != nil {
		t.Fatal(err)
	}
	history := vars[buf.MemoryKey].(string)
	if !strings.Contains(history, "第四轮继续") {
		t.Fatalf("protected turn missing: %s", history)
	}
	if strings.Contains(history, "第一轮办公请假") {
		t.Fatalf("overflow L0 should be compressed: %s", history)
	}
}

func TestPromoteLevel1RewritesLevel2Catalog(t *testing.T) {
	buf := testBuffer()
	buf.Level1MaxTokens = 1
	buf.level1Segments = []SummarySegment{
		{Content: strings.Repeat("请假摘要。", 20), Catalog: "请假"},
		{Content: strings.Repeat("审批摘要。", 20), Catalog: "审批"},
	}
	if err := buf.compactLevel1(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(buf.level1Segments) != 0 {
		t.Fatalf("L1 should be emptied, got %d", len(buf.level1Segments))
	}
	if buf.level2Catalog != "办公事项归档" {
		t.Fatalf("L2 catalog=%q", buf.level2Catalog)
	}
	if !strings.Contains(buf.level2Summary, "请假与审批已归档") {
		t.Fatalf("L2 body=%q", buf.level2Summary)
	}
}

func turnCountMust(t *testing.T, buf *SummaryBuffer, ctx context.Context) int {
	t.Helper()
	msgs, err := buf.ChatHistory.Messages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return turnCount(msgs)
}
