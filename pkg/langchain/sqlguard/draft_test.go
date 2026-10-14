package sqlguard

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExplainChineseSummary(t *testing.T) {
	summary, tables, cols, err := Explain(
		"SELECT title, amount, status FROM budget WHERE status = 1",
		"分析进行中的预算金额",
		100,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0] != "budget" {
		t.Fatalf("tables=%v", tables)
	}
	if !containsAll(cols, "title", "amount", "status") {
		t.Fatalf("columns=%v", cols)
	}
	for _, want := range []string{"预算", "标题", "金额", "状态", "100", "分析进行中的预算金额"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestDraftStoreTakeOnceAndOwner(t *testing.T) {
	s := NewDraftStore(time.Minute)
	s.Put(Draft{ID: "d1", UserID: "u1", SQL: "SELECT title FROM budget"})

	if _, err := s.Take("u2", "d1"); !errors.Is(err, ErrDraftOwner) {
		t.Fatalf("want owner error, got %v", err)
	}
	got, err := s.Take("u1", "d1")
	if err != nil {
		t.Fatal(err)
	}
	if got.SQL != "SELECT title FROM budget" {
		t.Fatalf("got %+v", got)
	}
	if _, err := s.Take("u1", "d1"); !errors.Is(err, ErrDraftNotFound) {
		t.Fatalf("want consumed, got %v", err)
	}
}

func TestDraftStoreGetDoesNotConsume(t *testing.T) {
	s := NewDraftStore(time.Minute)
	s.Put(Draft{ID: "d3", UserID: "u1", SQL: "SELECT title FROM budget"})

	got, err := s.Get("u1", "d3")
	if err != nil || got.SQL != "SELECT title FROM budget" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := s.Get("u2", "d3"); !errors.Is(err, ErrDraftOwner) {
		t.Fatalf("want owner error, got %v", err)
	}
	if _, err := s.Take("u1", "d3"); err != nil {
		t.Fatal(err)
	}
}

func TestDraftStoreExpired(t *testing.T) {
	s := NewDraftStore(time.Minute)
	s.Put(Draft{
		ID:        "d2",
		UserID:    "u1",
		SQL:       "SELECT title FROM budget",
		ExpiresAt: time.Now().Add(-time.Second),
	})
	if _, err := s.Take("u1", "d2"); !errors.Is(err, ErrDraftExpired) {
		t.Fatalf("got %v", err)
	}
}

func containsAll(have []string, want ...string) bool {
	set := map[string]struct{}{}
	for _, h := range have {
		set[h] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}
