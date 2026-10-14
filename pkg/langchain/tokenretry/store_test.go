package tokenretry

import (
	"context"
	"testing"
	"time"

	"github.com/tmc/langchaingo/schema"
)

func TestStoreGetDoesNotConsume(t *testing.T) {
	s := NewStore(time.Minute)
	s.Put(Ticket{ID: "t1", UserID: "u1", Kind: KindChat})

	got, err := s.Get("u1", "t1")
	if err != nil || got.Kind != KindChat {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := s.Get("u2", "t1"); err != ErrOwner {
		t.Fatalf("want owner, got %v", err)
	}
	s.Delete("u1", "t1")
	if _, err := s.Get("u1", "t1"); err != ErrNotFound {
		t.Fatalf("want deleted, got %v", err)
	}
}

func TestStoreExpired(t *testing.T) {
	s := NewStore(time.Minute)
	s.Put(Ticket{ID: "t2", UserID: "u1", ExpiresAt: time.Now().Add(-time.Second)})
	if _, err := s.Get("u1", "t2"); err != ErrExpired {
		t.Fatalf("got %v", err)
	}
}

func TestRecordCopiesSteps(t *testing.T) {
	ctx := WithSession(context.Background())
	steps := []schema.AgentStep{{Observation: "obs"}}
	Record(ctx, steps, map[string]string{"input": "q"})
	steps[0].Observation = "mutated"
	got := FromSession(ctx)
	if got.Steps[0].Observation != "obs" {
		t.Fatalf("session mutated: %+v", got.Steps)
	}
	if got.Inputs["input"] != "q" {
		t.Fatalf("inputs=%v", got.Inputs)
	}
}
