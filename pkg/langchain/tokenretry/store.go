package tokenretry

import (
	"sync"
	"time"

	"github.com/tmc/langchaingo/schema"
)

const DefaultTTL = 10 * time.Minute

type Kind string

const (
	KindSQLExecute   Kind = "sql_execute"
	KindSQLRevise    Kind = "sql_revise"
	KindChat         Kind = "chat"
	KindReact        Kind = "react"
	KindAnalysisFill Kind = "analysis_fill"
)

// Ticket is a server-side checkpoint after a request-token failure.
// Daily quota exhaustion is not stored; retry cannot help that.
type Ticket struct {
	ID          string
	UserID      string
	Kind        Kind
	SQLDraftID  string
	Feedback    string
	Chat        ChatSnapshot
	Handler     string
	Steps       []schema.AgentStep
	AgentInputs map[string]string
	Destination string
	NextInputs  string
	Partial     map[string]string
	Need        []string
	Accept      []string
	Reason      string
	Hint        string
	ExpiresAt   time.Time
}

type ChatSnapshot struct {
	Prompts    string
	RelationId int
	StartTime  int64
	EndTime    int64
}

type Store struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]Ticket
}

func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{ttl: ttl, m: make(map[string]Ticket)}
}

func (s *Store) Put(t Ticket) Ticket {
	if s == nil {
		return t
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.ExpiresAt.IsZero() {
		t.ExpiresAt = time.Now().Add(s.ttl)
	}
	s.m[t.ID] = t
	return t
}

func (s *Store) Get(userID, id string) (Ticket, error) {
	if s == nil {
		return Ticket{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookupLocked(userID, id)
}

func (s *Store) Delete(userID, id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.lookupLocked(userID, id)
	if err != nil {
		return
	}
	if t.UserID == userID {
		delete(s.m, id)
	}
}

func (s *Store) lookupLocked(userID, id string) (Ticket, error) {
	t, ok := s.m[id]
	if !ok {
		return Ticket{}, ErrNotFound
	}
	if t.UserID != userID {
		return Ticket{}, ErrOwner
	}
	if time.Now().After(t.ExpiresAt) {
		delete(s.m, id)
		return Ticket{}, ErrExpired
	}
	return t, nil
}
