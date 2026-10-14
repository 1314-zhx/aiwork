package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterRejectsBurst(t *testing.T) {
	l := New(2, time.Minute)
	if !l.Allow("a") || !l.Allow("a") {
		t.Fatal("first two must pass")
	}
	if l.Allow("a") {
		t.Fatal("third must fail")
	}
	if !l.Allow("b") {
		t.Fatal("other key must pass")
	}
}
