package toolx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/rueidis"
	"github.com/tmc/langchaingo/schema"
)

const (
	knowledgeParentKeyPrefix = "kb:parent:"
	knowledgeLiveKeyPrefix   = "kb:live:"
	knowledgeLockKeyPrefix   = "kb:lock:"
	knowledgeKeysPrefix      = "kb:keys:"

	knowledgeDrainTTL = time.Minute
	knowledgeLockTTL  = 2 * time.Minute
)

type knowledgeParent struct {
	ID   string
	Body string
}

type parentStore interface {
	Put(ctx context.Context, docID, gen, parentID, body string) error
	Get(ctx context.Context, docID, gen, parentID string) (string, error)
	TryLock(ctx context.Context, docID string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, docID string) error
	Live(ctx context.Context, docIDs []string) (map[string]string, error)
	SetLive(ctx context.Context, docID, gen string) error
	SaveKeys(ctx context.Context, docID, gen string, keys []string) error
	LoadKeys(ctx context.Context, docID, gen string) ([]string, error)
	ExpireKeys(ctx context.Context, keys []string, ttl time.Duration) error
}

type memEntry struct {
	body    string
	expires time.Time
}

type memoryParentStore struct {
	mu    sync.Mutex
	m     map[string]memEntry
	live  map[string]string
	locks map[string]time.Time
	keys  map[string][]string
}

func newMemoryParentStore() *memoryParentStore {
	return &memoryParentStore{
		m:     map[string]memEntry{},
		live:  map[string]string{},
		locks: map[string]time.Time{},
		keys:  map[string][]string{},
	}
}

func (s *memoryParentStore) Put(_ context.Context, docID, gen, parentID, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[parentRedisKey(docID, gen, parentID)] = memEntry{body: body}
	return nil
}

func (s *memoryParentStore) Get(_ context.Context, docID, gen, parentID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(parentRedisKey(docID, gen, parentID)), nil
}

func (s *memoryParentStore) TryLock(_ context.Context, docID string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := lockRedisKey(docID)
	if until, ok := s.locks[key]; ok && time.Now().Before(until) {
		return false, nil
	}
	if ttl <= 0 {
		ttl = knowledgeLockTTL
	}
	s.locks[key] = time.Now().Add(ttl)
	return true, nil
}

func (s *memoryParentStore) Unlock(_ context.Context, docID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.locks, lockRedisKey(docID))
	return nil
}

func (s *memoryParentStore) Live(_ context.Context, docIDs []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for _, id := range docIDs {
		if id == "" || id == "unknown" {
			continue
		}
		if gen := s.live[liveRedisKey(id)]; gen != "" {
			out[id] = gen
		}
	}
	return out, nil
}

func (s *memoryParentStore) SetLive(_ context.Context, docID, gen string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[liveRedisKey(docID)] = gen
	return nil
}

func (s *memoryParentStore) SaveKeys(_ context.Context, docID, gen string, keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[keysRedisKey(docID, gen)] = append([]string{}, keys...)
	return nil
}

func (s *memoryParentStore) LoadKeys(_ context.Context, docID, gen string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.keys[keysRedisKey(docID, gen)]...), nil
}

func (s *memoryParentStore) ExpireKeys(_ context.Context, keys []string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp := time.Time{}
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if ttl <= 0 {
			delete(s.m, key)
			delete(s.live, key)
			delete(s.keys, key)
			continue
		}
		if e, ok := s.m[key]; ok {
			e.expires = exp
			s.m[key] = e
		}
	}
	return nil
}

func (s *memoryParentStore) getLocked(key string) string {
	e, ok := s.m[key]
	if !ok {
		return ""
	}
	if !e.expires.IsZero() && time.Now().After(e.expires) {
		delete(s.m, key)
		return ""
	}
	return e.body
}

type redisParentStore struct {
	client rueidis.Client
}

func newRedisParentStore(url string) (*redisParentStore, error) {
	opt, err := rueidis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	client, err := rueidis.NewClient(opt)
	if err != nil {
		return nil, err
	}
	return &redisParentStore{client: client}, nil
}

func (s *redisParentStore) Put(ctx context.Context, docID, gen, parentID, body string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("parent store is not configured")
	}
	return s.client.Do(ctx, s.client.B().Set().Key(parentRedisKey(docID, gen, parentID)).Value(body).Build()).Error()
}

func (s *redisParentStore) Get(ctx context.Context, docID, gen, parentID string) (string, error) {
	if s == nil || s.client == nil {
		return "", fmt.Errorf("parent store is not configured")
	}
	val, err := s.client.Do(ctx, s.client.B().Get().Key(parentRedisKey(docID, gen, parentID)).Build()).ToString()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

func (s *redisParentStore) TryLock(ctx context.Context, docID string, ttl time.Duration) (bool, error) {
	if s == nil || s.client == nil {
		return false, fmt.Errorf("parent store is not configured")
	}
	if ttl <= 0 {
		ttl = knowledgeLockTTL
	}
	err := s.client.Do(ctx, s.client.B().Set().Key(lockRedisKey(docID)).Value("1").Nx().ExSeconds(int64(ttl.Seconds())).Build()).Error()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *redisParentStore) Unlock(ctx context.Context, docID string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("parent store is not configured")
	}
	return s.client.Do(ctx, s.client.B().Del().Key(lockRedisKey(docID)).Build()).Error()
}

func (s *redisParentStore) Live(ctx context.Context, docIDs []string) (map[string]string, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("parent store is not configured")
	}
	keys := make([]string, 0, len(docIDs))
	order := make([]string, 0, len(docIDs))
	seen := map[string]struct{}{}
	for _, id := range docIDs {
		if id == "" || id == "unknown" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		order = append(order, id)
		keys = append(keys, liveRedisKey(id))
	}
	out := map[string]string{}
	if len(keys) == 0 {
		return out, nil
	}
	vals, err := s.client.Do(ctx, s.client.B().Mget().Key(keys...).Build()).ToArray()
	if err != nil {
		return nil, err
	}
	for i, id := range order {
		if i >= len(vals) {
			break
		}
		gen, err := vals[i].ToString()
		if err != nil {
			if rueidis.IsRedisNil(err) {
				continue
			}
			return nil, err
		}
		if strings.TrimSpace(gen) != "" {
			out[id] = gen
		}
	}
	return out, nil
}

func (s *redisParentStore) SetLive(ctx context.Context, docID, gen string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("parent store is not configured")
	}
	return s.client.Do(ctx, s.client.B().Set().Key(liveRedisKey(docID)).Value(gen).Build()).Error()
}

func (s *redisParentStore) SaveKeys(ctx context.Context, docID, gen string, keys []string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("parent store is not configured")
	}
	keys = compactKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	return s.client.Do(ctx, s.client.B().Sadd().Key(keysRedisKey(docID, gen)).Member(keys...).Build()).Error()
}

func (s *redisParentStore) LoadKeys(ctx context.Context, docID, gen string) ([]string, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("parent store is not configured")
	}
	vals, err := s.client.Do(ctx, s.client.B().Smembers().Key(keysRedisKey(docID, gen)).Build()).AsStrSlice()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return nil, nil
		}
		return nil, err
	}
	return vals, nil
}

func (s *redisParentStore) ExpireKeys(ctx context.Context, keys []string, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("parent store is not configured")
	}
	keys = compactKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	sec := int64(ttl.Seconds())
	if sec < 0 {
		sec = 0
	}
	cmds := make([]rueidis.Completed, 0, len(keys))
	for _, key := range keys {
		if ttl <= 0 {
			cmds = append(cmds, s.client.B().Del().Key(key).Build())
			continue
		}
		cmds = append(cmds, s.client.B().Expire().Key(key).Seconds(sec).Build())
	}
	var errs []error
	for _, res := range s.client.DoMulti(ctx, cmds...) {
		if res.Error() != nil {
			errs = append(errs, res.Error())
		}
	}
	return errors.Join(errs...)
}

func parentRedisKey(docID, gen, parentID string) string {
	if gen == "" {
		return knowledgeParentKeyPrefix + safeRedisPart(docID) + ":" + safeRedisPart(parentID)
	}
	return knowledgeParentKeyPrefix + safeRedisPart(docID) + ":" + safeRedisPart(gen) + ":" + safeRedisPart(parentID)
}

func liveRedisKey(docID string) string {
	return knowledgeLiveKeyPrefix + safeRedisPart(docID)
}

func lockRedisKey(docID string) string {
	return knowledgeLockKeyPrefix + safeRedisPart(docID)
}

func keysRedisKey(docID, gen string) string {
	if gen == "" {
		return knowledgeKeysPrefix + safeRedisPart(docID)
	}
	return knowledgeKeysPrefix + safeRedisPart(docID) + ":" + safeRedisPart(gen)
}

func safeRedisPart(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, " ", "_")
	if s == "" {
		return "_"
	}
	return s
}

func compactKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func attachParents(ctx context.Context, store parentStore, docs []schema.Document) []schema.Document {
	if store == nil || len(docs) == 0 {
		return docs
	}
	for i := range docs {
		if parentFromMeta(docs[i]) != "" {
			continue
		}
		pid := knowledgeParentID(docs[i])
		did := knowledgeDocID(docs[i])
		if pid == "" || did == "" || did == "unknown" {
			continue
		}
		body, err := store.Get(ctx, did, knowledgeGen(docs[i]), pid)
		if err != nil || strings.TrimSpace(body) == "" {
			continue
		}
		if docs[i].Metadata == nil {
			docs[i].Metadata = map[string]any{}
		}
		docs[i].Metadata["parent"] = body
	}
	return docs
}
