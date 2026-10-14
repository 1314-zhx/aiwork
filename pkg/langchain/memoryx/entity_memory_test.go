package memoryx

import (
	"context"
	"strings"
	"testing"

	"aiworkc/pkg/langchain"
	"github.com/tmc/langchaingo/memory"
)

func TestEntityExtractorStoresProfileFields(t *testing.T) {
	store := NewEntityStore()
	extractor := NewEntityExtractor()
	extractor.ExtractAndStore("chat-1", store, "我叫张三，年龄是28岁，性别男，工位是A-101")

	profile := store.GetUserProfile("chat-1")
	if profile.Name != "张三" {
		t.Fatalf("unexpected name: %q", profile.Name)
	}
	if profile.Age != 28 {
		t.Fatalf("unexpected age: %d", profile.Age)
	}
	if profile.Gender != "男" {
		t.Fatalf("unexpected gender: %q", profile.Gender)
	}
	if profile.SeatNo != "A-101" {
		t.Fatalf("unexpected seat: %q", profile.SeatNo)
	}
}

func TestSummaryBufferLoadsEntityContext(t *testing.T) {
	store := NewEntityStore()
	store.SetUserProfile("chat-1", &UserProfile{Name: "张三", Age: 28, Gender: "男", SeatNo: "A-101"})
	buffer := &SummaryBuffer{
		ConversationBuffer: memoryConversationBuffer(),
		model:              "qwen-turbo",
		entityStore:        store,
		entityInject:       true,
	}
	ctx := context.WithValue(context.Background(), langchain.ChatId, "chat-1")

	variables, err := buffer.LoadMemoryVariables(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	history, _ := variables[buffer.MemoryKey].(string)
	for _, expected := range []string{"对话实体", "张三", "28", "A-101"} {
		if !strings.Contains(history, expected) {
			t.Fatalf("history does not contain %q: %s", expected, history)
		}
	}
}

func TestFactsStayWhenDialogueExtracts(t *testing.T) {
	store := NewEntityStore()
	store.SetFacts("u1", FactProfile{UserID: "u1", Name: "张皓翔", DeptID: "d1", DeptName: "研发"})
	NewEntityExtractor().ExtractAndStore("chat-1", store, "我叫小张，工位是A-101")

	facts := store.GetFacts("u1")
	if facts.Name != "张皓翔" || facts.DeptName != "研发" {
		t.Fatalf("facts mutated: %+v", facts)
	}
	dialogue := store.GetUserProfile("chat-1")
	if dialogue.Name != "小张" || dialogue.SeatNo != "A-101" {
		t.Fatalf("dialogue: %+v", dialogue)
	}

	NewEntityExtractor().ExtractAndStore("chat-1", store, "我叫阿翔")
	if store.GetUserProfile("chat-1").Name != "阿翔" {
		t.Fatal("later utterance should win dialogue nickname")
	}
	if store.GetFacts("u1").Name != "张皓翔" {
		t.Fatal("later utterance must not change account name")
	}

	store.Delete("chat-1")
	if store.GetUserProfile("chat-1").HasData() {
		t.Fatal("clearing chat should drop dialogue only")
	}
	if store.GetFacts("u1").Name != "张皓翔" {
		t.Fatal("clearing chat must keep facts")
	}
}

func TestInjectorKeepsFactsAndDialogueSeparate(t *testing.T) {
	store := NewEntityStore()
	store.SetFacts("u1", FactProfile{UserID: "u1", Name: "张皓翔", DeptName: "研发"})
	store.SetUserProfile("chat-1", &UserProfile{Name: "小张", SeatNo: "A-101"})
	got := NewEntityInjector(store, "qwen-turbo").Inject(context.Background(), "u1", "chat-1", "你是助手")
	if !strings.Contains(got, "事实身份") || !strings.Contains(got, "张皓翔") {
		t.Fatalf("missing facts: %s", got)
	}
	if !strings.Contains(got, "对话实体") || !strings.Contains(got, "小张") {
		t.Fatalf("missing dialogue: %s", got)
	}
	if !strings.Contains(got, "你是助手") {
		t.Fatal("base prompt dropped")
	}
}

func memoryConversationBuffer() *memory.ConversationBuffer {
	return memory.NewConversationBuffer(memory.WithReturnMessages(false))
}
