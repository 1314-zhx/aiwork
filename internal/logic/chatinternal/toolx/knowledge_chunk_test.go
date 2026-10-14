package toolx

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/tmc/langchaingo/schema"
)

func TestJoinKnowledgePages(t *testing.T) {
	got := joinKnowledgePages([]schema.Document{
		{PageContent: "第一页规定。", Metadata: map[string]any{"page": 1, "total_pages": 2}},
		{PageContent: "第二页细则。", Metadata: map[string]any{"page": 2, "total_pages": 2}},
	})
	if !strings.Contains(got.PageContent, "第一页规定。") || !strings.Contains(got.PageContent, "第二页细则。") {
		t.Fatalf("joined=%q", got.PageContent)
	}
	if _, ok := got.Metadata["page"]; ok {
		t.Fatal("joined text should not keep a single page number")
	}
	if got.Metadata["total_pages"] != 2 {
		t.Fatalf("total_pages=%v", got.Metadata["total_pages"])
	}
}

func TestSplitParentChildWindowAndChildren(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&b, "第%d条规定员工应在九点前到岗并完成考勤打卡，迟到需向直属主管报备。", i)
	}
	docs, parents, err := splitParentChild([]schema.Document{{PageContent: b.String()}})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) < 2 {
		t.Fatalf("want multiple children, got %d", len(docs))
	}
	if len(parents) < 2 {
		t.Fatalf("want sliding parent windows, got %d", len(parents))
	}
	for _, doc := range docs {
		if parentFromMeta(doc) != "" {
			t.Fatal("child must not embed parent text")
		}
		if utf8.RuneCountInString(doc.PageContent) > knowledgeChildSize+80 {
			t.Fatalf("child too large: %d", utf8.RuneCountInString(doc.PageContent))
		}
	}
	byID := map[string]string{}
	for _, p := range parents {
		if utf8.RuneCountInString(p.Body) > knowledgeParentSize+120 {
			t.Fatalf("parent too large: %d", utf8.RuneCountInString(p.Body))
		}
		byID[p.ID] = p.Body
	}
	p0, p1 := byID["p0"], byID["p1"]
	if p0 == "" || p1 == "" {
		t.Fatalf("missing p0/p1: %v", byID)
	}
	if !windowsOverlap(p0, p1) {
		t.Fatal("parent windows should overlap")
	}
}

func TestSplitParentChildShortDoc(t *testing.T) {
	docs, parents, err := splitParentChild([]schema.Document{{
		PageContent: "员工手册：上班时间九点。",
		Metadata:    map[string]any{"name": "手册.pdf"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || len(parents) != 1 {
		t.Fatalf("docs=%d parents=%d", len(docs), len(parents))
	}
	if parentFromMeta(docs[0]) != "" {
		t.Fatal("child must not embed parent text")
	}
	if parents[0].Body != "员工手册：上班时间九点。" {
		t.Fatalf("parent=%q", parents[0].Body)
	}
	if docs[0].Metadata["name"] != "手册.pdf" || knowledgeParentID(docs[0]) != "p0" {
		t.Fatalf("metadata dropped: %+v", docs[0].Metadata)
	}
}

func TestKnowledgeContextFallsBackToChild(t *testing.T) {
	doc := schema.Document{PageContent: "旧索引片段", Metadata: map[string]any{"doc_id": "x"}}
	if knowledgeContext(doc) != "旧索引片段" {
		t.Fatal(knowledgeContext(doc))
	}
}

func TestDedupeKnowledgeParentsKeepsFirstPerParent(t *testing.T) {
	docs := []schema.Document{
		{PageContent: "p0-child0", Metadata: map[string]any{"doc_id": "abc", "parent_id": "p0", "child": 0}},
		{PageContent: "p0-child1", Metadata: map[string]any{"doc_id": "abc", "parent_id": "p0", "child": 1}},
		{PageContent: "p1-child0", Metadata: map[string]any{"doc_id": "abc", "parent_id": "p1", "child": 0}},
		{PageContent: "other-p0", Metadata: map[string]any{"doc_id": "xyz", "parent_id": "p0", "child": 0}},
	}
	got := dedupeKnowledgeParents(docs)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].PageContent != "p0-child0" || got[1].PageContent != "p1-child0" || got[2].PageContent != "other-p0" {
		t.Fatalf("order=%v %v %v", got[0].PageContent, got[1].PageContent, got[2].PageContent)
	}
}

func TestDedupeKnowledgeParentsKeepsOrphans(t *testing.T) {
	docs := []schema.Document{
		{PageContent: "a", Metadata: map[string]any{"doc_id": "old"}},
		{PageContent: "b", Metadata: map[string]any{"doc_id": "old"}},
	}
	got := dedupeKnowledgeParents(docs)
	if len(got) != 2 {
		t.Fatalf("legacy chunks without parent_id must not collapse, got %d", len(got))
	}
}

func TestAttachParentsLoadsOnceByID(t *testing.T) {
	store := newMemoryParentStore()
	if err := store.Put(context.Background(), "abc", "1", "p0", "父窗口全文九点上班"); err != nil {
		t.Fatal(err)
	}
	docs := []schema.Document{
		{PageContent: "九点上班", Metadata: map[string]any{"doc_id": "abc", "parent_id": "p0", "gen": "1"}},
		{PageContent: "九点上班", Metadata: map[string]any{"doc_id": "abc", "parent_id": "p0", "gen": "1"}},
	}
	got := attachParents(context.Background(), store, docs)
	if knowledgeContext(got[0]) != "父窗口全文九点上班" {
		t.Fatalf("got %q", knowledgeContext(got[0]))
	}
	if parentRedisKey("abc", "", "p0") != "kb:parent:abc:p0" {
		t.Fatal(parentRedisKey("abc", "", "p0"))
	}
	if parentRedisKey("abc", "1", "p0") != "kb:parent:abc:1:p0" {
		t.Fatal(parentRedisKey("abc", "1", "p0"))
	}
}

func TestAttachParentsKeepsLegacyInlineParent(t *testing.T) {
	store := newMemoryParentStore()
	docs := []schema.Document{{
		PageContent: "子",
		Metadata:    map[string]any{"doc_id": "abc", "parent_id": "p0", "parent": "旧拷贝的父块"},
	}}
	got := attachParents(context.Background(), store, docs)
	if knowledgeContext(got[0]) != "旧拷贝的父块" {
		t.Fatal(knowledgeContext(got[0]))
	}
}

func windowsOverlap(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	n := 24
	if len(ar) < n || len(br) < n {
		return strings.Contains(b, a) || strings.Contains(a, b)
	}
	tail := string(ar[len(ar)-n:])
	head := string(br[:n])
	return strings.Contains(b, tail) || strings.Contains(a, head)
}
