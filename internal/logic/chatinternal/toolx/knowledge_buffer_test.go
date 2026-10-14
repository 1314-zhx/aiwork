package toolx

import (
	"context"
	"testing"
	"time"

	"github.com/tmc/langchaingo/schema"
)

func TestNextKnowledgeGen(t *testing.T) {
	if nextKnowledgeGen("") != "1" || nextKnowledgeGen("1") != "2" || nextKnowledgeGen("9") != "10" {
		t.Fatalf("got %s %s %s", nextKnowledgeGen(""), nextKnowledgeGen("1"), nextKnowledgeGen("9"))
	}
}

func TestKnowledgeIsLive(t *testing.T) {
	live := map[string]string{"abc": "4"}
	keep := schema.Document{Metadata: map[string]any{"doc_id": "abc", "gen": "4"}}
	old := schema.Document{Metadata: map[string]any{"doc_id": "abc", "gen": "3"}}
	staging := schema.Document{Metadata: map[string]any{"doc_id": "abc", "gen": "5"}}
	legacy := schema.Document{Metadata: map[string]any{"doc_id": "abc"}}
	if !knowledgeIsLive(keep, live) || knowledgeIsLive(old, live) || knowledgeIsLive(staging, live) || knowledgeIsLive(legacy, live) {
		t.Fatal("live gen 4 should keep only gen 4")
	}
	if !knowledgeIsLive(legacy, nil) || knowledgeIsLive(keep, nil) {
		t.Fatal("no pointer keeps unversioned docs only")
	}
}

func TestFilterLiveKnowledgeDocsAfterSwitch(t *testing.T) {
	store := newMemoryParentStore()
	ctx := context.Background()
	if err := store.SetLive(ctx, "abc", "4"); err != nil {
		t.Fatal(err)
	}
	docs := []schema.Document{
		{PageContent: "old", Metadata: map[string]any{"doc_id": "abc", "gen": "3"}},
		{PageContent: "new", Metadata: map[string]any{"doc_id": "abc", "gen": "4"}},
		{PageContent: "other-old", Metadata: map[string]any{"doc_id": "xyz", "gen": "1"}},
	}
	got := filterLiveKnowledgeDocs(ctx, store, docs)
	if len(got) != 1 || got[0].PageContent != "new" {
		t.Fatalf("got %+v", got)
	}
}

func TestDualBufferSwitchKeepsOldParentForInFlight(t *testing.T) {
	store := newMemoryParentStore()
	ctx := context.Background()
	if err := store.Put(ctx, "abc", "3", "p0", "旧版九点上班"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetLive(ctx, "abc", "3"); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "abc", "4", "p0", "新版十点上班"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetLive(ctx, "abc", "4"); err != nil {
		t.Fatal(err)
	}

	inflight := []schema.Document{{
		PageContent: "九点",
		Metadata:    map[string]any{"doc_id": "abc", "parent_id": "p0", "gen": "3"},
	}}
	got := attachParents(ctx, store, inflight)
	if knowledgeContext(got[0]) != "旧版九点上班" {
		t.Fatalf("in-flight should still read gen 3, got %q", knowledgeContext(got[0]))
	}

	fresh := filterLiveKnowledgeDocs(ctx, store, []schema.Document{
		{PageContent: "九点", Metadata: map[string]any{"doc_id": "abc", "parent_id": "p0", "gen": "3"}},
		{PageContent: "十点", Metadata: map[string]any{"doc_id": "abc", "parent_id": "p0", "gen": "4"}},
	})
	if len(fresh) != 1 || knowledgeGen(fresh[0]) != "4" {
		t.Fatalf("new traffic should be gen 4, got %+v", fresh)
	}
	fresh = attachParents(ctx, store, fresh)
	if knowledgeContext(fresh[0]) != "新版十点上班" {
		t.Fatalf("got %q", knowledgeContext(fresh[0]))
	}
}

func TestDrainGenerationLeavesOldReadableUntilTTL(t *testing.T) {
	store := newMemoryParentStore()
	ctx := context.Background()
	if err := store.Put(ctx, "abc", "3", "p0", "旧版"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveKeys(ctx, "abc", "3", []string{parentRedisKey("abc", "3", "p0")}); err != nil {
		t.Fatal(err)
	}
	if err := drainGeneration(ctx, store, "abc", "3", nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	body, err := store.Get(ctx, "abc", "3", "p0")
	if err != nil || body != "旧版" {
		t.Fatalf("draining gen must stay readable within ttl, got %q %v", body, err)
	}
	if err := drainGeneration(ctx, store, "abc", "3", nil, 0); err != nil {
		t.Fatal(err)
	}
	body, err = store.Get(ctx, "abc", "3", "p0")
	if err != nil || body != "" {
		t.Fatalf("ttl 0 should drop draining gen, got %q %v", body, err)
	}
}

func TestKnowledgeLockBlocksSecondWriter(t *testing.T) {
	store := newMemoryParentStore()
	ctx := context.Background()
	ok, err := store.TryLock(ctx, "abc", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	ok, err = store.TryLock(ctx, "abc", time.Minute)
	if err != nil || ok {
		t.Fatal("second lock should wait")
	}
	if err := store.Unlock(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	ok, err = store.TryLock(ctx, "abc", time.Minute)
	if err != nil || !ok {
		t.Fatalf("after unlock: %v %v", ok, err)
	}
}

func TestFailedStagingIsNotLive(t *testing.T) {
	store := newMemoryParentStore()
	ctx := context.Background()
	if err := store.SetLive(ctx, "abc", "3"); err != nil {
		t.Fatal(err)
	}
	docs := []schema.Document{
		{PageContent: "live", Metadata: map[string]any{"doc_id": "abc", "gen": "3"}},
		{PageContent: "staging", Metadata: map[string]any{"doc_id": "abc", "gen": "4"}},
	}
	got := filterLiveKnowledgeDocs(ctx, store, docs)
	if len(got) != 1 || got[0].PageContent != "live" {
		t.Fatalf("staging must stay hidden until SetLive, got %+v", got)
	}
}
