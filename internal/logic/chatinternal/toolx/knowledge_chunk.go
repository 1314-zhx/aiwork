package toolx

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/textsplitter"
)

const (
	knowledgeParentSize    = 480
	knowledgeParentOverlap = 80
	knowledgeChildSize     = 140
	knowledgeChildOverlap  = 40
)

var knowledgeSeparators = []string{"\n\n", "\n", "。", "！", "？", "；", "，", " ", ""}

func joinKnowledgePages(pages []schema.Document) schema.Document {
	var b strings.Builder
	meta := map[string]any{}
	for i, page := range pages {
		text := strings.TrimSpace(page.PageContent)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
		if page.Metadata == nil {
			continue
		}
		if _, ok := meta["total_pages"]; !ok {
			if v, ok := page.Metadata["total_pages"]; ok {
				meta["total_pages"] = v
			}
		}
		if i == 0 {
			for k, v := range page.Metadata {
				if k == "page" {
					continue
				}
				meta[k] = v
			}
		}
	}
	return schema.Document{PageContent: b.String(), Metadata: meta}
}

func splitParentChild(docs []schema.Document) ([]schema.Document, []knowledgeParent, error) {
	parentSplitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(knowledgeParentSize),
		textsplitter.WithChunkOverlap(knowledgeParentOverlap),
		textsplitter.WithSeparators(knowledgeSeparators),
		textsplitter.WithKeepSeparator(true),
		textsplitter.WithLenFunc(utf8.RuneCountInString),
	)
	childSplitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(knowledgeChildSize),
		textsplitter.WithChunkOverlap(knowledgeChildOverlap),
		textsplitter.WithSeparators(knowledgeSeparators),
		textsplitter.WithKeepSeparator(true),
		textsplitter.WithLenFunc(utf8.RuneCountInString),
	)

	windows, err := textsplitter.SplitDocuments(parentSplitter, docs)
	if err != nil {
		return nil, nil, err
	}

	out := make([]schema.Document, 0)
	parents := make([]knowledgeParent, 0)
	childN := 0
	for pi, window := range windows {
		body := strings.TrimSpace(window.PageContent)
		if body == "" {
			continue
		}
		parts, err := childSplitter.SplitText(body)
		if err != nil {
			return nil, nil, err
		}
		if len(parts) == 0 {
			parts = []string{body}
		}
		parentID := fmt.Sprintf("p%d", pi)
		parents = append(parents, knowledgeParent{ID: parentID, Body: body})
		for ci, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			meta := copyMeta(window.Metadata)
			delete(meta, "parent")
			meta["parent_id"] = parentID
			meta["child"] = ci
			meta["chunk"] = childN
			out = append(out, schema.Document{PageContent: part, Metadata: meta})
			childN++
		}
	}
	return out, parents, nil
}

func parentFromMeta(doc schema.Document) string {
	if doc.Metadata == nil {
		return ""
	}
	parent := strings.TrimSpace(fmt.Sprintf("%v", doc.Metadata["parent"]))
	if parent == "" || parent == "<nil>" {
		return ""
	}
	return parent
}

func knowledgeContext(doc schema.Document) string {
	if parent := parentFromMeta(doc); parent != "" {
		return parent
	}
	return doc.PageContent
}

func knowledgeParentID(doc schema.Document) string {
	if doc.Metadata == nil {
		return ""
	}
	id := strings.TrimSpace(fmt.Sprintf("%v", doc.Metadata["parent_id"]))
	if id == "" || id == "<nil>" {
		return ""
	}
	return id
}

// dedupeKnowledgeParents keeps the first hit per (doc_id, parent_id).
// SimilaritySearch is already ordered by distance, so the first child of a
// parent is the closest. Children without parent_id are left as-is.
func dedupeKnowledgeParents(docs []schema.Document) []schema.Document {
	if len(docs) < 2 {
		return docs
	}
	seen := map[string]struct{}{}
	out := make([]schema.Document, 0, len(docs))
	for i, doc := range docs {
		key := parentDedupeKey(doc, i)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, doc)
	}
	return out
}

func parentDedupeKey(doc schema.Document, idx int) string {
	pid := knowledgeParentID(doc)
	if pid == "" {
		return fmt.Sprintf("%s\x00orphan\x00%d", knowledgeDocID(doc), idx)
	}
	return knowledgeDocID(doc) + "\x00" + pid
}

func copyMeta(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
