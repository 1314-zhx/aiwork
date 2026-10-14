package filesecure

import (
	"errors"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrFileNotFound = errors.New("uploaded file not found")
	ErrFileOwner    = errors.New("file does not belong to the current user")
	ErrFileExpired  = errors.New("uploaded file has expired")
)

const DefaultFileTTL = 24 * time.Hour

type Record struct {
	ID           string
	OwnerID      string
	AbsPath      string
	Ext          string
	Size         int64
	OriginalName string
	ExpiresAt    time.Time
}

type Registry struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]Record
}

func NewRegistry(ttl time.Duration) *Registry {
	if ttl <= 0 {
		ttl = DefaultFileTTL
	}
	return &Registry{ttl: ttl, m: map[string]Record{}}
}

func (r *Registry) Put(rec Record) Record {
	if r == nil {
		return rec
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec.ExpiresAt.IsZero() {
		rec.ExpiresAt = time.Now().Add(r.ttl)
	}
	r.m[rec.ID] = rec
	return rec
}

func (r *Registry) Get(userID, id string, allowSystem bool) (Record, error) {
	if r == nil {
		return Record{}, ErrFileNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.m[id]
	if !ok {
		base := filepath.Base(id)
		rec, ok = r.m[base]
	}
	if !ok {
		return Record{}, ErrFileNotFound
	}
	if time.Now().After(rec.ExpiresAt) {
		delete(r.m, rec.ID)
		return Record{}, ErrFileExpired
	}
	if !allowSystem && rec.OwnerID != userID {
		return Record{}, ErrFileOwner
	}
	return rec, nil
}
