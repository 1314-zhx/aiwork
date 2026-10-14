package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultModel = "qwen3-rerank"
	DefaultPath  = "/reranks"
)

// Client scores query-document pairs. Not a chat LLM.
type Client interface {
	Rank(ctx context.Context, query string, documents []string, topN int) ([]Hit, error)
}

type Hit struct {
	Index int
	Score float64
}

type HTTPClient struct {
	endpoint string
	apiKey   string
	model    string
	http     *http.Client
}

type Option func(*HTTPClient)

func WithHTTPClient(h *http.Client) Option {
	return func(c *HTTPClient) {
		if h != nil {
			c.http = h
		}
	}
}

func New(endpoint, apiKey, model string, opts ...Option) *HTTPClient {
	endpoint = strings.TrimSpace(endpoint)
	apiKey = strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)
	if endpoint == "" || apiKey == "" {
		return nil
	}
	if model == "" {
		model = DefaultModel
	}
	c := &HTTPClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    model,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func Endpoint(baseURL, override string) string {
	if u := strings.TrimSpace(override); u != "" {
		return strings.TrimRight(u, "/")
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return ""
	}
	if strings.HasSuffix(base, DefaultPath) {
		return base
	}
	return base + DefaultPath
}

func (c *HTTPClient) Rank(ctx context.Context, query string, documents []string, topN int) ([]Hit, error) {
	if c == nil {
		return nil, fmt.Errorf("rerank client is not configured")
	}
	if len(documents) == 0 {
		return nil, nil
	}
	if topN <= 0 || topN > len(documents) {
		topN = len(documents)
	}
	body, err := json.Marshal(map[string]any{
		"model":     c.model,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
		"instruct":  "Retrieve the official policy passage that best answers the query. Prefer employee handbook and attendance rules over incidental mentions.",
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rerank http %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	hits, err := parseHits(raw)
	if err != nil {
		return nil, err
	}
	return filterHits(hits, len(documents), topN), nil
}

func parseHits(raw []byte) ([]Hit, error) {
	var payload struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
			Score          float64 `json:"score"`
		} `json:"results"`
		Output struct {
			Results []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
			} `json:"results"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	rows := payload.Results
	if len(rows) == 0 {
		for _, r := range payload.Output.Results {
			rows = append(rows, struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
				Score          float64 `json:"score"`
			}{Index: r.Index, RelevanceScore: r.RelevanceScore})
		}
	}
	out := make([]Hit, 0, len(rows))
	for _, r := range rows {
		score := r.RelevanceScore
		if score == 0 {
			score = r.Score
		}
		out = append(out, Hit{Index: r.Index, Score: score})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("rerank returned no results")
	}
	return out, nil
}

func filterHits(hits []Hit, n, topN int) []Hit {
	out := make([]Hit, 0, len(hits))
	seen := map[int]struct{}{}
	for _, h := range hits {
		if h.Index < 0 || h.Index >= n {
			continue
		}
		if _, ok := seen[h.Index]; ok {
			continue
		}
		seen[h.Index] = struct{}{}
		out = append(out, h)
		if len(out) >= topN {
			break
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
