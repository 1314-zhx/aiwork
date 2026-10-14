package ratelimit

import (
	"sync"
	"time"
)

type window struct {
	n    int
	from time.Time
}

type Limiter struct {
	mu      sync.Mutex
	limit   int
	every   time.Duration
	windows map[string]window
}

func New(limit int, every time.Duration) *Limiter {
	if limit <= 0 {
		limit = 60
	}
	if every <= 0 {
		every = time.Minute
	}
	return &Limiter{limit: limit, every: every, windows: map[string]window{}}
}

func (l *Limiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.windows[key]
	if !ok || now.Sub(w.from) >= l.every {
		l.windows[key] = window{n: 1, from: now}
		return true
	}
	if w.n >= l.limit {
		return false
	}
	w.n++
	l.windows[key] = w
	return true
}
