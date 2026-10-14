/**
 * @author: Cursor Agent
 * @doc: Tests for leave time adjustment helpers.
 */

package approval

import (
	"testing"
	"time"
)

func TestParseTimeText_Today(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	ts, ok := ParseTimeText("今天", ref)
	if !ok {
		t.Fatal("expected ParseTimeText to match")
	}
	// 今天整天应该是 00:00:00
	want := time.Date(2026, 6, 10, 0, 0, 0, 0, time.Local).Unix()
	if ts != want {
		t.Fatalf("unexpected timestamp, got %d (=%s) want %d", ts, time.Unix(ts, 0).Format("2006-01-02 15:04"), want)
	}
}

func TestParseTimeText_Tomorrow(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	ts, ok := ParseTimeText("明天", ref)
	if !ok {
		t.Fatal("expected ParseTimeText to match")
	}
	want := time.Date(2026, 6, 11, 0, 0, 0, 0, time.Local).Unix()
	if ts != want {
		t.Fatalf("unexpected timestamp, got %d (=%s) want %d", ts, time.Unix(ts, 0).Format("2006-01-02 15:04"), want)
	}
}

func TestParseTimeText_TomorrowWithTime(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	ts, ok := ParseTimeText("明天 18:00", ref)
	if !ok {
		t.Fatal("expected ParseTimeText to match")
	}
	want := time.Date(2026, 6, 11, 18, 0, 0, 0, time.Local).Unix()
	if ts != want {
		t.Fatalf("unexpected timestamp, got %d (=%s) want %d", ts, time.Unix(ts, 0).Format("2006-01-02 15:04"), want)
	}
}

func TestParseTimeText_TomorrowAfternoon(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	ts, ok := ParseTimeText("明天下午6点", ref)
	if !ok {
		t.Fatal("expected ParseTimeText to match")
	}
	want := time.Date(2026, 6, 11, 18, 0, 0, 0, time.Local).Unix()
	if ts != want {
		t.Fatalf("unexpected timestamp, got %d (=%s) want %d", ts, time.Unix(ts, 0).Format("2006-01-02 15:04"), want)
	}
}

func TestParseTimeRange_SameDay(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	start, end, ok := ParseTimeRange("明天 18:00 到 20:00", ref)
	if !ok {
		t.Fatal("expected ParseTimeRange to match")
	}

	wantStart := time.Date(2026, 6, 11, 18, 0, 0, 0, time.Local).Unix()
	wantEnd := time.Date(2026, 6, 11, 20, 0, 0, 0, time.Local).Unix()
	if start != wantStart {
		t.Fatalf("unexpected start timestamp, got %d (=%s) want %d", start, time.Unix(start, 0).Format("2006-01-02 15:04"), wantStart)
	}
	if end != wantEnd {
		t.Fatalf("unexpected end timestamp, got %d (=%s) want %d", end, time.Unix(end, 0).Format("2006-01-02 15:04"), wantEnd)
	}
}

func TestParseTimeRange_WithMinutes(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	start, end, ok := ParseTimeRange("明天 18:00-20:30", ref)
	if !ok {
		t.Fatal("expected ParseTimeRange to match")
	}

	wantStart := time.Date(2026, 6, 11, 18, 0, 0, 0, time.Local).Unix()
	wantEnd := time.Date(2026, 6, 11, 20, 30, 0, 0, time.Local).Unix()
	if start != wantStart {
		t.Fatalf("unexpected start timestamp, got %d (=%s) want %d", start, time.Unix(start, 0).Format("2006-01-02 15:04"), wantStart)
	}
	if end != wantEnd {
		t.Fatalf("unexpected end timestamp, got %d (=%s) want %d", end, time.Unix(end, 0).Format("2006-01-02 15:04"), wantEnd)
	}
}

func TestParseTimeText_NextWeek(t *testing.T) {
	// 2026年6月10日是周三
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)

	// 下周一应该是 2026年6月15日
	ts, ok := ParseTimeText("下周一", ref)
	if !ok {
		t.Fatal("expected ParseTimeText to match")
	}
	want := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local).Unix()
	if ts != want {
		t.Fatalf("unexpected timestamp, got %d (=%s) want %d", ts, time.Unix(ts, 0).Format("2006-01-02 15:04"), want)
	}
}

func TestParseTimeText_MonthDay(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	ts, ok := ParseTimeText("12月25日", ref)
	if !ok {
		t.Fatal("expected ParseTimeText to match")
	}
	// 12月25日应该是2026年12月25日
	want := time.Date(2026, 12, 25, 0, 0, 0, 0, time.Local).Unix()
	if ts != want {
		t.Fatalf("unexpected timestamp, got %d (=%s) want %d", ts, time.Unix(ts, 0).Format("2006-01-02 15:04"), want)
	}
}

func TestParseTimeText_MonthDayWithTime(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	ts, ok := ParseTimeText("12月25日 09:00", ref)
	if !ok {
		t.Fatal("expected ParseTimeText to match")
	}
	want := time.Date(2026, 12, 25, 9, 0, 0, 0, time.Local).Unix()
	if ts != want {
		t.Fatalf("unexpected timestamp, got %d (=%s) want %d", ts, time.Unix(ts, 0).Format("2006-01-02 15:04"), want)
	}
}

func TestParseTimeText_HalfHour(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	ts, ok := ParseTimeText("明天 9点半", ref)
	if !ok {
		t.Fatal("expected ParseTimeText to match")
	}
	want := time.Date(2026, 6, 11, 9, 30, 0, 0, time.Local).Unix()
	if ts != want {
		t.Fatalf("unexpected timestamp, got %d (=%s) want %d", ts, time.Unix(ts, 0).Format("2006-01-02 15:04"), want)
	}
}

func TestParseTimeText_AfterNoon(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	ts, ok := ParseTimeText("明天下午3点", ref)
	if !ok {
		t.Fatal("expected ParseTimeText to match")
	}
	want := time.Date(2026, 6, 11, 15, 0, 0, 0, time.Local).Unix()
	if ts != want {
		t.Fatalf("unexpected timestamp, got %d (=%s) want %d", ts, time.Unix(ts, 0).Format("2006-01-02 15:04"), want)
	}
}

func TestResolveLeaveTimesUsesDefaultWhenUnparseable(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	start, end, failed := ResolveLeaveTimes("随便", "看看", "帮我请个假", ref)
	if !failed {
		t.Fatal("expected parseFailed")
	}
	wantStart, wantEnd := DefaultLeaveRange(ref)
	if start != wantStart || end != wantEnd {
		t.Fatalf("start=%d end=%d want %d %d", start, end, wantStart, wantEnd)
	}
}

func TestResolveLeaveTimesKeepsParsedRange(t *testing.T) {
	ref := time.Date(2026, 6, 10, 10, 0, 0, 0, time.Local)
	start, end, failed := ResolveLeaveTimes("明天 09:00", "明天 18:00", "明天请假", ref)
	if failed {
		t.Fatal("parsed range should not be marked failed")
	}
	wantStart := time.Date(2026, 6, 11, 9, 0, 0, 0, time.Local).Unix()
	wantEnd := time.Date(2026, 6, 11, 18, 0, 0, 0, time.Local).Unix()
	if start != wantStart || end != wantEnd {
		t.Fatalf("start=%d end=%d", start, end)
	}
}
