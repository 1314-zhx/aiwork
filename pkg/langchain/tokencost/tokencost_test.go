package tokencost

import (
	"context"
	"errors"
	"testing"
)

func TestCanAffordFirstHop(t *testing.T) {
	b := NewBudget(1000)
	b.Add(400, 0)
	if err := b.CanAfford(AgentFirstMaxTokens); err != nil {
		t.Fatal(err)
	}
	b.Add(200, 0)
	if err := b.CanAfford(AgentFirstMaxTokens); err != ErrRequestTokenLimit {
		t.Fatalf("got %v", err)
	}
}

func TestIsRequestLimit(t *testing.T) {
	if !IsRequestLimit(ErrRequestTokenLimit) {
		t.Fatal("direct")
	}
	if !IsRequestLimit(errors.New("call failed: " + ErrRequestTokenLimit.Error())) {
		t.Fatal("unwrapped message")
	}
	if IsRequestLimit(ErrDailyTokenLimit) {
		t.Fatal("daily is not request retry")
	}
}

func TestBudgetRejectsWhenPromptWouldExceed(t *testing.T) {
	b := NewBudget(100)
	ctx := WithBudget(context.Background(), b)
	if FromContext(ctx).Guard(50) != nil {
		t.Fatal("50 should fit")
	}
	b.Add(80, 10)
	if err := b.Guard(20); err != ErrRequestTokenLimit {
		t.Fatalf("got %v, want request limit", err)
	}
}

func TestRaiseLimitKeepsUsed(t *testing.T) {
	b := NewBudget(100)
	b.Add(80, 0)
	b.RaiseLimit(200)
	if err := b.Guard(50); err != nil {
		t.Fatal(err)
	}
	if b.Remain() != 120 {
		t.Fatalf("remain=%d", b.Remain())
	}
}

func TestDailyLimiter(t *testing.T) {
	d := NewDailyLimiter(10)
	if err := d.Allow("u1", 0); err != nil {
		t.Fatal(err)
	}
	d.Add("u1", 10)
	if err := d.Allow("u1", 0); err != ErrDailyTokenLimit {
		t.Fatalf("got %v, want daily limit", err)
	}
}

func TestDailyCanAffordTaskCosts(t *testing.T) {
	d := NewDailyLimiter(10000)
	d.Add("u1", 8000)
	if err := d.CanAfford("u1", ChatTokenLimit); err != ErrDailyNotEnough {
		t.Fatalf("chat: %v", err)
	}
	if err := d.CanAfford("u1", AnalysisTokenLimit); err != ErrDailyNotEnough {
		t.Fatalf("chatlog: %v", err)
	}
	if err := d.CanAfford("u1", ReActTokenLimit); err != ErrDailyNotEnough {
		t.Fatalf("react: %v", err)
	}

	d2 := NewDailyLimiter(10000)
	d2.Add("u2", 1500)
	if err := d2.CanAfford("u2", RouterDailyNeed); err != nil {
		t.Fatal(err)
	}
	if err := d2.CanAfford("u2", ChatTokenLimit); err != nil {
		t.Fatal(err)
	}
	if err := d2.CanAfford("u2", AnalysisTokenLimit); err != nil {
		t.Fatal(err)
	}
	if err := d2.CanAfford("u2", ReActTokenLimit); err != nil {
		t.Fatal(err)
	}
	d2.Add("u2", 1000)
	if err := d2.CanAfford("u2", ReActTokenLimit); err != ErrDailyNotEnough {
		t.Fatalf("react with 7500 remain: %v", err)
	}
}

func TestRequireDailyUsesBoundLimiter(t *testing.T) {
	d := NewDailyLimiter(5000)
	d.Add("u1", 2500)
	ctx := WithDaily(context.Background(), d, "u1")
	if err := RequireDaily(ctx, ChatTokenLimit); err != ErrDailyNotEnough {
		t.Fatalf("got %v", err)
	}
	if err := RequireDaily(ctx, 2000); err != nil {
		t.Fatal(err)
	}
}

func TestDailyEnterNeed(t *testing.T) {
	if DailyEnterNeed(ReActTokenLimit) != ReActDailyNeed {
		t.Fatalf("react enter: %d", DailyEnterNeed(ReActTokenLimit))
	}
	if DailyEnterNeed(AnalysisTokenLimit) != 3000 {
		t.Fatalf("analysis enter: %d", DailyEnterNeed(AnalysisTokenLimit))
	}
	if DailyEnterNeed(RouterDailyNeed) != 1500 {
		t.Fatalf("router enter: %d", DailyEnterNeed(RouterDailyNeed))
	}

	d := NewDailyLimiter(8000)
	d.Add("u1", 1)
	if d.Remain("u1") != 7999 {
		t.Fatalf("remain: %d", d.Remain("u1"))
	}
	if err := d.CanAfford("u1", DailyEnterNeed(ReActTokenLimit)); err != nil {
		t.Fatalf("7999 should enter react: %v", err)
	}
	if err := d.CanAfford("u1", ReActTokenLimit); err != ErrDailyNotEnough {
		t.Fatalf("7999 still below full 8000 cap: %v", err)
	}

	d2 := NewDailyLimiter(10000)
	d2.Add("u2", 7000)
	if err := d2.CanAfford("u2", ReActDailyNeed); err != nil {
		t.Fatalf("remain 3000 should enter react: %v", err)
	}
	d2.Add("u2", 1)
	if err := d2.CanAfford("u2", ReActDailyNeed); err != ErrDailyNotEnough {
		t.Fatalf("remain 2999 should not enter react: %v", err)
	}
}

func TestHandlerDailyNeed(t *testing.T) {
	if HandlerDailyNeed("todo") != ReActDailyNeed || HandlerDailyNeed("knowledge") != ReActDailyNeed || HandlerDailyNeed("approval") != ReActDailyNeed {
		t.Fatal("react handlers")
	}
	if HandlerDailyNeed("chat_log") != 3000 || HandlerDailyNeed("analysis") != 3000 {
		t.Fatal("analysis handlers")
	}
	if HandlerDailyNeed("DEFAULT") != 1500 {
		t.Fatal("chat")
	}
}

func TestIsDailyLimit(t *testing.T) {
	if !IsDailyLimit(ErrDailyNotEnough) || !IsDailyLimit(ErrDailyTokenLimit) {
		t.Fatal("daily")
	}
	if IsDailyLimit(ErrRequestTokenLimit) || IsRequestLimit(ErrDailyNotEnough) {
		t.Fatal("must not mix daily with request retry")
	}
}
