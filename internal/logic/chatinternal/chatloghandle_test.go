package chatinternal

import (
	"strings"
	"testing"
)

func TestValidateChatLogWindow(t *testing.T) {
	if err := ValidateChatLogWindow(0, 10); err == nil {
		t.Fatal("missing start")
	}
	if err := ValidateChatLogWindow(10, 10); err == nil {
		t.Fatal("end must be after start")
	}
	if err := ValidateChatLogWindow(1, 1+maxChatLogSpan+1); err == nil {
		t.Fatal("span too long")
	}
	if err := ValidateChatLogWindow(100, 200); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeChatLogGoal(t *testing.T) {
	if _, err := NormalizeChatLogGoal("  "); err == nil {
		t.Fatal("empty goal")
	}
	got, err := NormalizeChatLogGoal("  提取待办  ")
	if err != nil || got != "提取待办" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestUnixTimeFrom(t *testing.T) {
	n, err := unixTimeFrom(float64(1724084938))
	if err != nil || n != 1724084938 {
		t.Fatalf("float64: %d %v", n, err)
	}
	n, err = unixTimeFrom("1724084938")
	if err != nil || n != 1724084938 {
		t.Fatalf("string: %d %v", n, err)
	}
}

func TestSpliceChatLogMemoryTruncatesEarlierTimestamps(t *testing.T) {
	lines := []chatLogLine{
		{SendTime: 100, Text: strings.Repeat("a", 20) + "\n"},
		{SendTime: 200, Text: strings.Repeat("b", 20) + "\n"},
		{SendTime: 300, Text: strings.Repeat("c", 20) + "\n"},
	}
	mem := spliceChatLogMemory(lines, 100, 400, 50)
	if !mem.Truncated {
		t.Fatal("expected truncate")
	}
	if mem.EffectiveStart != 200 && mem.EffectiveStart != 300 {
		t.Fatalf("effective start %d", mem.EffectiveStart)
	}
	if mem.TruncateNote == "" {
		t.Fatal("truncate must not be silent")
	}
	if !strings.Contains(mem.TruncateNote, formatChatLogTime(mem.EffectiveStart)) {
		t.Fatalf("note missing cutoff time: %s", mem.TruncateNote)
	}
	if strings.Contains(mem.Text, strings.Repeat("a", 20)) && mem.EffectiveStart > 100 {
		t.Fatal("earlier timestamp content should be dropped")
	}

	printed := attachTruncateNotice("总结正文", mem)
	if !strings.HasPrefix(printed, mem.TruncateNote) {
		t.Fatalf("printed text must lead with truncate note: %s", printed)
	}
}

func TestSpliceChatLogMemoryNoSilentWhenFit(t *testing.T) {
	lines := []chatLogLine{
		{SendTime: 100, Text: "hello\n"},
		{SendTime: 200, Text: "world\n"},
	}
	mem := spliceChatLogMemory(lines, 100, 200, maxChatLogRunes)
	if mem.Truncated || mem.TruncateNote != "" {
		t.Fatalf("unexpected truncate %+v", mem)
	}
	if mem.Text != "hello\nworld\n" {
		t.Fatalf("got %q", mem.Text)
	}
}

func TestParseChatLogText(t *testing.T) {
	if got := parseChatLogText("```json\n{\"text\":\"周报已完成\"}\n```"); got != "周报已完成" {
		t.Fatalf("got %q", got)
	}
	if got := parseChatLogText("直接文本"); got != "直接文本" {
		t.Fatalf("got %q", got)
	}
}
