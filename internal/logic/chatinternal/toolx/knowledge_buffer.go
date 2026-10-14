package toolx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tmc/langchaingo/schema"
)

const knowledgeRetrieveSearchK = 24

func nextKnowledgeGen(old string) string {
	old = strings.TrimSpace(old)
	if old == "" {
		return "1"
	}
	n, err := strconv.Atoi(old)
	if err != nil || n < 1 {
		return "1"
	}
	return strconv.Itoa(n + 1)
}

func knowledgeVectorID(fileID, gen string, chunk any) string {
	return safeRedisPart(fileID) + ":" + safeRedisPart(gen) + ":" + safeRedisPart(fmt.Sprintf("%v", chunk))
}

func knowledgeGen(doc schema.Document) string {
	if doc.Metadata == nil {
		return ""
	}
	gen := strings.TrimSpace(fmt.Sprintf("%v", doc.Metadata["gen"]))
	if gen == "" || gen == "<nil>" {
		return ""
	}
	return gen
}

func filterLiveKnowledgeDocs(ctx context.Context, store parentStore, docs []schema.Document) []schema.Document {
	if store == nil || len(docs) == 0 {
		return docs
	}
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		id := knowledgeDocID(doc)
		if id == "" || id == "unknown" {
			continue
		}
		ids = append(ids, id)
	}
	live, err := store.Live(ctx, ids)
	if err != nil {
		return docs
	}
	out := make([]schema.Document, 0, len(docs))
	for _, doc := range docs {
		if knowledgeIsLive(doc, live) {
			out = append(out, doc)
		}
	}
	return out
}

func knowledgeIsLive(doc schema.Document, live map[string]string) bool {
	id := knowledgeDocID(doc)
	gen := knowledgeGen(doc)
	want := ""
	if live != nil {
		want = live[id]
	}
	if want == "" {
		return gen == ""
	}
	return gen == want
}

func liveGenOf(ctx context.Context, store parentStore, docID string) (string, error) {
	if store == nil || docID == "" {
		return "", nil
	}
	got, err := store.Live(ctx, []string{docID})
	if err != nil {
		return "", err
	}
	return got[docID], nil
}

func drainGeneration(ctx context.Context, store parentStore, docID, gen string, extra []string, ttl time.Duration) error {
	if store == nil {
		return nil
	}
	keys := append([]string{}, extra...)
	if gen != "" || docID != "" {
		saved, err := store.LoadKeys(ctx, docID, gen)
		if err != nil {
			return err
		}
		keys = append(keys, saved...)
		keys = append(keys, keysRedisKey(docID, gen))
	}
	return store.ExpireKeys(ctx, compactKeys(keys), ttl)
}
