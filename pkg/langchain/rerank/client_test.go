package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEndpoint(t *testing.T) {
	got := Endpoint("https://dashscope.aliyuncs.com/compatible-mode/v1", "")
	want := "https://dashscope.aliyuncs.com/compatible-mode/v1/reranks"
	if got != want {
		t.Fatalf("got %s", got)
	}
	if Endpoint("https://example/v1", "https://example/v1/rerank") != "https://example/v1/rerank" {
		t.Fatal("override")
	}
	if New("", "key", "") != nil || New("http://x", "", "") != nil {
		t.Fatal("missing config should be nil")
	}
}

func TestHTTPClientRank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/reranks" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("auth=%s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "qwen3-rerank" {
			t.Fatalf("model=%v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"index": 1, "relevance_score": 0.91},
				{"index": 0, "relevance_score": 0.12},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL+"/reranks", "test-key", "qwen3-rerank")
	hits, err := c.Rank(context.Background(), "员工几点上班", []string{"通知", "员工手册：九点"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Index != 1 {
		t.Fatalf("hits=%v", hits)
	}
}

func TestParseHitsDashScopeNested(t *testing.T) {
	raw := []byte(`{"output":{"results":[{"index":2,"relevance_score":0.8}]}}`)
	hits, err := parseHits(raw)
	if err != nil || len(hits) != 1 || hits[0].Index != 2 {
		t.Fatalf("got %v err=%v", hits, err)
	}
}
