package tokencost

import (
	"context"
	"errors"
	"sync"
	"time"
)

type DailyLimiter struct {
	mu    sync.Mutex
	used  map[string]int
	limit int
}

func NewDailyLimiter(limit int) *DailyLimiter {
	if limit <= 0 {
		limit = DailyTokenLimit
	}
	return &DailyLimiter{
		used:  make(map[string]int),
		limit: limit,
	}
}

func (d *DailyLimiter) Remain(uid string) int {
	if d == nil {
		return DailyTokenLimit
	}
	if uid == "" {
		return d.limit
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	remain := d.limit - d.used[dailyKey(uid)]
	if remain < 0 {
		return 0
	}
	return remain
}

func (d *DailyLimiter) Allow(uid string, alreadyUsed int) error {
	need := 1
	if alreadyUsed > 0 {
		need = 1 + alreadyUsed
	}
	return d.CanAfford(uid, need)
}

// CanAfford rejects when today's remaining quota cannot cover need.
func (d *DailyLimiter) CanAfford(uid string, need int) error {
	if d == nil || uid == "" {
		return nil
	}
	if need < 1 {
		need = 1
	}
	remain := d.Remain(uid)
	if remain <= 0 {
		return ErrDailyTokenLimit
	}
	if remain < need {
		return ErrDailyNotEnough
	}
	return nil
}

func (d *DailyLimiter) Add(uid string, tokens int) {
	if d == nil || uid == "" || tokens <= 0 {
		return
	}
	d.mu.Lock()
	d.used[dailyKey(uid)] += tokens
	d.mu.Unlock()
}

func dailyKey(uid string) string {
	loc := time.FixedZone("CST", 8*3600)
	return uid + "|" + time.Now().In(loc).Format("2006-01-02")
}

type dailyCtxKey struct{}

type dailyBind struct {
	limiter *DailyLimiter
	uid     string
}

func WithDaily(ctx context.Context, limiter *DailyLimiter, uid string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, dailyCtxKey{}, &dailyBind{limiter: limiter, uid: uid})
}

func RequireDaily(ctx context.Context, need int) error {
	b, _ := ctx.Value(dailyCtxKey{}).(*dailyBind)
	if b == nil || b.limiter == nil {
		return nil
	}
	return b.limiter.CanAfford(b.uid, need)
}

func IsDailyLimit(err error) bool {
	return errors.Is(err, ErrDailyTokenLimit) || errors.Is(err, ErrDailyNotEnough)
}

// DailyEnterNeed is remaining daily quota required to start a path.
// Request caps stay full (8000/4000/2000). ReAct enters at typical cost (~3000), other paths at 3/4.
func DailyEnterNeed(limit int) int {
	if limit <= 0 {
		return 1
	}
	if limit == ReActTokenLimit {
		return ReActDailyNeed
	}
	need := limit * 3 / 4
	if need < 1 {
		return 1
	}
	return need
}

// HandlerDailyNeed is the daily quota a destination must still have before it runs.
func HandlerDailyNeed(name string) int {
	switch name {
	case "todo", "knowledge", "approval":
		return ReActDailyNeed
	case "chat_log", "analysis", "db", "file":
		return DailyEnterNeed(AnalysisTokenLimit)
	default:
		return DailyEnterNeed(RouterDailyNeed)
	}
}
