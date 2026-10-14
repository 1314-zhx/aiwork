package toolx

import (
	"context"
	"fmt"
	"strings"

	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/rerank"

	"github.com/tmc/langchaingo/schema"
)

const (
	knowledgeRetrieveK     = 8
	knowledgeRerankTopK    = 1
	knowledgeRerankSnippet = 400
)

func rerankKnowledgeDocs(ctx context.Context, client rerank.Client, question string, docs []schema.Document) []schema.Document {
	if len(docs) == 0 {
		return docs
	}
	if len(docs) <= knowledgeRerankTopK {
		return docs
	}
	if client == nil {
		return docs[:knowledgeRerankTopK]
	}

	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = rerankDocumentText(doc)
	}
	hits, err := client.Rank(ctx, question, texts, knowledgeRerankTopK)
	if err != nil || len(hits) == 0 {
		return docs[:knowledgeRerankTopK]
	}
	return pickReranked(docs, hits[0].Index)
}

func pickReranked(docs []schema.Document, idx int) []schema.Document {
	if len(docs) == 0 {
		return docs
	}
	if idx < 0 || idx >= len(docs) {
		return docs[:knowledgeRerankTopK]
	}
	return []schema.Document{docs[idx]}
}

func rerankDocumentText(doc schema.Document) string {
	body := defend.Truncate(doc.PageContent, knowledgeRerankSnippet)
	name := knowledgeDocName(doc)
	if name == "" {
		return body
	}
	return "文档:" + name + "\n" + body
}

func knowledgeDocName(doc schema.Document) string {
	if doc.Metadata == nil {
		return ""
	}
	name := strings.TrimSpace(fmt.Sprintf("%v", doc.Metadata["name"]))
	if name == "" || name == "<nil>" {
		return ""
	}
	return name
}
