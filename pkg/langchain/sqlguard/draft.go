package sqlguard

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrDraftNotFound = errors.New("查询草稿不存在或已使用")
	ErrDraftExpired  = errors.New("查询草稿已过期，请重新提问")
	ErrDraftOwner    = errors.New("查询草稿不属于当前用户")
)

const DefaultDraftTTL = 10 * time.Minute

// Draft is a server-side ticket: the SQL never has to come back from the client.
type Draft struct {
	ID         string
	UserID     string
	SQL        string
	Query      string
	Tables     []string
	IR         QueryIR
	RelationID int
	ExpiresAt  time.Time
}

type DraftStore struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]Draft
}

func NewDraftStore(ttl time.Duration) *DraftStore {
	if ttl <= 0 {
		ttl = DefaultDraftTTL
	}
	return &DraftStore{
		ttl: ttl,
		m:   make(map[string]Draft),
	}
}

func (s *DraftStore) Put(d Draft) Draft {
	if s == nil {
		return d
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.ExpiresAt.IsZero() {
		d.ExpiresAt = time.Now().Add(s.ttl)
	}
	s.m[d.ID] = d
	return d
}

func (s *DraftStore) lookupLocked(userID, id string) (Draft, error) {
	d, ok := s.m[id]
	if !ok {
		return Draft{}, ErrDraftNotFound
	}
	if d.UserID != userID {
		return Draft{}, ErrDraftOwner
	}
	if time.Now().After(d.ExpiresAt) {
		delete(s.m, id)
		return Draft{}, ErrDraftExpired
	}
	return d, nil
}

// Get returns the draft without consuming it. Expired tickets are deleted.
func (s *DraftStore) Get(userID, id string) (Draft, error) {
	if s == nil {
		return Draft{}, ErrDraftNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookupLocked(userID, id)
}

// Take returns the draft once and deletes it. Wrong user does not consume the ticket.
func (s *DraftStore) Take(userID, id string) (Draft, error) {
	if s == nil {
		return Draft{}, ErrDraftNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.lookupLocked(userID, id)
	if err != nil {
		return Draft{}, err
	}
	delete(s.m, id)
	return d, nil
}

// Restore puts a consumed ticket back (same id) so a failed execute can be retried.
func (s *DraftStore) Restore(d Draft) Draft {
	return s.Put(d)
}
