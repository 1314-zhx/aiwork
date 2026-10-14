package toolx

import (
	"context"
	"testing"

	"aiworkc/pkg/langchain/rerank"

	"github.com/tmc/langchaingo/schema"
)

func TestRerankKnowledgeDocsNilClientKeepsVectorTop1(t *testing.T) {
	docs := []schema.Document{
		{PageContent: "其它通知里提到上班", Metadata: map[string]any{"doc_id": "other", "name": "通知.pdf"}},
		{PageContent: "员工手册规定上班时间为 9:00", Metadata: map[string]any{"doc_id": "handbook", "name": "员工手册.pdf"}},
	}
	got := rerankKnowledgeDocs(nil, nil, "员工几点上班", docs)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if knowledgeDocID(got[0]) != "other" {
		t.Fatalf("fallback should keep vector rank, got %s", knowledgeDocID(got[0]))
	}
}

func TestRerankKnowledgeDocsSingleUnchanged(t *testing.T) {
	docs := []schema.Document{{PageContent: "only", Metadata: map[string]any{"doc_id": "a"}}}
	got := rerankKnowledgeDocs(nil, nil, "q", docs)
	if len(got) != 1 || knowledgeDocID(got[0]) != "a" {
		t.Fatalf("got %+v", got)
	}
}

func TestPickRerankedSelectsTop1(t *testing.T) {
	docs := []schema.Document{
		{PageContent: "通知：记得按时上班", Metadata: map[string]any{"doc_id": "notice", "name": "通知.pdf"}},
		{PageContent: "员工手册：工作时间 9:00-18:00", Metadata: map[string]any{"doc_id": "handbook", "name": "员工手册.pdf"}},
	}
	got := pickReranked(docs, 1)
	if len(got) != 1 || knowledgeDocID(got[0]) != "handbook" {
		t.Fatalf("got %+v", got)
	}
	got = pickReranked(docs, 9)
	if len(got) != 1 || knowledgeDocID(got[0]) != "notice" {
		t.Fatalf("fallback got %+v", got)
	}
}

type stubRerank struct {
	idx int
	err error
}

func (s stubRerank) Rank(context.Context, string, []string, int) ([]rerank.Hit, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []rerank.Hit{{Index: s.idx, Score: 0.9}}, nil
}

func TestRerankKnowledgeDocsUsesRerankerTop1(t *testing.T) {
	docs := []schema.Document{
		{PageContent: "通知：记得按时上班", Metadata: map[string]any{"doc_id": "notice", "name": "通知.pdf"}},
		{PageContent: "员工手册：工作时间 9:00-18:00", Metadata: map[string]any{"doc_id": "handbook", "name": "员工手册.pdf"}},
	}
	got := rerankKnowledgeDocs(context.Background(), stubRerank{idx: 1}, "员工几点上班", docs)
	if len(got) != 1 || knowledgeDocID(got[0]) != "handbook" {
		t.Fatalf("got %+v", got)
	}
}
