package toolx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aiworkc/internal/svc"
	"aiworkc/pkg/audit"
	"aiworkc/pkg/filesecure"
	"aiworkc/pkg/langchain/outputparserx"
	"aiworkc/pkg/token"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores/redisvector"
)

type KnowledgeUpdate struct {
	svc          *svc.ServiceContext
	Callback     callbacks.Handler
	outPutParser outputparserx.Structured
	store        *redisvector.Store
	parents      parentStore
}

func NewKnowledgeUpdate(svc *svc.ServiceContext) *KnowledgeUpdate {
	return &KnowledgeUpdate{
		svc:      svc,
		Callback: svc.Callbacks,
		outPutParser: outputparserx.NewStructured([]outputparserx.ResponseSchema{
			{
				Name:        "fileId",
				Description: "upload file id returned by /v1/upload/file. never a filesystem path",
			}, {
				Name:        "path",
				Description: "deprecated; if present must be the same fileId, not a path",
			}, {
				Name:        "name",
				Description: "original file name for display only",
			}, {
				Name:        "time",
				Description: "file update time",
			},
		}),
	}
}

func (k *KnowledgeUpdate) Name() string {
	return "knowledge_update"
}

func (k *KnowledgeUpdate) ParameterSchema() any {
	return k.outPutParser.JSONSchema()
}

func (k *KnowledgeUpdate) Description() string {
	return `
a knowledge base update interface.
use only after an administrator has reviewed the uploaded file.
pass fileId from the upload API. never pass a filesystem path or .. segments.
the tool indexes all chunks; it does not filter wording.
your output should be in the following json format.
` + k.outPutParser.GetFormatInstructions()
}

func (k *KnowledgeUpdate) Call(ctx context.Context, input string) (string, error) {
	if err := k.svc.Auth(ctx); err != nil {
		return "", err
	}

	fileId, displayName, err := parseKnowledgeFileID(input, k.outPutParser)
	if err != nil {
		return "", err
	}

	uid := token.GetUId(ctx)
	absPath, meta, err := resolveKnowledgeFile(k.svc, uid, fileId, true)
	if err != nil {
		return "", err
	}
	if displayName == "" {
		displayName = meta.OriginalName
	}

	f, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	finfo, err := f.Stat()
	if err != nil {
		return "", err
	}

	p := documentloaders.NewPDF(f, finfo.Size())
	pages, err := p.Load(ctx)
	if err != nil {
		return "", err
	}
	chunkedDocuments, parents, err := splitParentChild([]schema.Document{joinKnowledgePages(pages)})
	if err != nil {
		return "", err
	}

	if k.store == nil {
		k.store, err = getKnowledgeStore(ctx, k.svc)
		if err != nil {
			return "", err
		}
	}
	if k.parents == nil {
		k.parents, err = getKnowledgeParentStore(k.svc)
		if err != nil {
			return "", err
		}
	}

	ok, err := k.parents.TryLock(ctx, fileId, knowledgeLockTTL)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("knowledge document is being updated")
	}

	oldGen, err := liveGenOf(ctx, k.parents, fileId)
	if err != nil {
		_ = k.parents.Unlock(ctx, fileId)
		return "", err
	}
	newGen := nextKnowledgeGen(oldGen)

	kept := make([]schema.Document, 0, len(chunkedDocuments))
	written := make([]string, 0, len(chunkedDocuments)+len(parents)+1)
	for _, doc := range chunkedDocuments {
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		delete(doc.Metadata, "parent")
		doc.Metadata["doc_id"] = fileId
		doc.Metadata["owner_uid"] = uid
		doc.Metadata["acl"] = "public"
		doc.Metadata["name"] = displayName
		doc.Metadata["gen"] = newGen
		doc.Metadata["ids"] = knowledgeVectorID(fileId, newGen, doc.Metadata["chunk"])
		kept = append(kept, doc)
	}
	if len(kept) == 0 {
		_ = k.parents.Unlock(ctx, fileId)
		return "", fmt.Errorf("file produced no chunks")
	}

	switched := false
	defer func() {
		if !switched {
			_ = drainGeneration(context.Background(), k.parents, fileId, newGen, written, knowledgeDrainTTL)
		}
		_ = k.parents.Unlock(context.Background(), fileId)
	}()

	for _, parent := range parents {
		if err = k.parents.Put(ctx, fileId, newGen, parent.ID, parent.Body); err != nil {
			return "", err
		}
		written = append(written, parentRedisKey(fileId, newGen, parent.ID))
	}

	const batchSize = 10
	for i := 0; i < len(kept); i += batchSize {
		end := i + batchSize
		if end > len(kept) {
			end = len(kept)
		}
		ids, err := k.store.AddDocuments(ctx, kept[i:end])
		if err != nil {
			return "", err
		}
		written = append(written, ids...)
	}
	if err = k.parents.SaveKeys(ctx, fileId, newGen, written); err != nil {
		return "", err
	}
	written = append(written, keysRedisKey(fileId, newGen))

	if err = k.parents.SetLive(ctx, fileId, newGen); err != nil {
		return "", err
	}
	switched = true

	if oldGen != "" {
		if err = drainGeneration(ctx, k.parents, fileId, oldGen, nil, knowledgeDrainTTL); err != nil {
			return "", err
		}
	} else {
		legacy := make([]string, 0, len(parents))
		for _, parent := range parents {
			legacy = append(legacy, parentRedisKey(fileId, "", parent.ID))
		}
		if err = k.parents.ExpireKeys(ctx, legacy, knowledgeDrainTTL); err != nil {
			return "", err
		}
	}

	audit.Event(ctx, "knowledge.update", "uid", uid, "fileId", fileId, "gen", newGen, "chunks", len(kept), "parents", len(parents))
	return Success, nil
}

func parseKnowledgeFileID(input string, parser outputparserx.Structured) (fileId, name string, err error) {
	data, err := parser.Parse(input)
	fields := map[string]any{}
	if err == nil {
		if m, ok := data.(map[string]any); ok {
			fields = m
		}
	} else {
		if json.Unmarshal([]byte(input), &fields) != nil {
			return "", "", err
		}
	}
	fileId = strings.TrimSpace(fmt.Sprintf("%v", firstNonEmpty(fields, "fileId", "path", "name")))
	name = strings.TrimSpace(fmt.Sprintf("%v", fields["name"]))
	fileId = filepath.Base(strings.ReplaceAll(fileId, `\`, "/"))
	if fileId == "" || fileId == "<nil>" || strings.Contains(fileId, "..") {
		return "", "", fmt.Errorf("fileId is required")
	}
	return fileId, name, nil
}

func firstNonEmpty(fields map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := fields[k]; ok && strings.TrimSpace(fmt.Sprintf("%v", v)) != "" && fmt.Sprintf("%v", v) != "<nil>" {
			return v
		}
	}
	return ""
}

func resolveKnowledgeFile(svc *svc.ServiceContext, uid, fileId string, allowSystem bool) (string, filesecure.Record, error) {
	if svc != nil && svc.Files != nil {
		if rec, err := svc.Files.Get(uid, fileId, allowSystem); err == nil {
			if _, err := os.Stat(rec.AbsPath); err != nil {
				return "", rec, fmt.Errorf("file not found")
			}
			return rec.AbsPath, rec, nil
		}
	}
	abs, err := filesecure.Resolve(svc.Config.Upload.SavePath, fileId)
	if err != nil {
		return "", filesecure.Record{}, err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", filesecure.Record{}, fmt.Errorf("file not found")
	}
	return abs, filesecure.Record{ID: fileId, AbsPath: abs, OwnerID: uid}, nil
}

func getKnowledgeStore(ctx context.Context, svc *svc.ServiceContext) (*redisvector.Store, error) {
	embedder, err := embeddings.NewEmbedder(svc.LLMs)
	if err != nil {
		return nil, err
	}

	return redisvector.New(ctx, redisvector.WithEmbedder(embedder), redisvector.WithConnectionURL("redis://"+svc.Config.
		Redis.
		Addr), redisvector.WithIndexName("knowledge", true))
}

func getKnowledgeParentStore(svc *svc.ServiceContext) (parentStore, error) {
	if svc == nil {
		return nil, fmt.Errorf("parent store is not configured")
	}
	return newRedisParentStore("redis://" + svc.Config.Redis.Addr)
}
