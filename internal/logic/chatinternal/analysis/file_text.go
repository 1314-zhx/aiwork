package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/tokencost"

	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

const fileEmbedModel = "qwen-turbo"

func readAnalysisFileText(abs string) (string, error) {
	ext := strings.ToLower(filepath.Ext(abs))
	switch ext {
	case ".txt", ".csv":
		raw, err := os.ReadFile(abs)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(raw)), nil
	case ".pdf":
		f, err := os.Open(abs)
		if err != nil {
			return "", err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return "", err
		}
		pages, err := documentloaders.NewPDF(f, info.Size()).Load(context.Background())
		if err != nil {
			return "", err
		}
		return joinPDFPages(pages), nil
	default:
		return "", fmt.Errorf("文件分析仅支持 pdf/txt/csv 正文，当前类型 %s", ext)
	}
}

func joinPDFPages(pages []schema.Document) string {
	var b strings.Builder
	for _, page := range pages {
		text := strings.TrimSpace(page.PageContent)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	return b.String()
}

func fitFileBody(ctx context.Context, system, question, body string) (string, bool, error) {
	cfg := defend.UntrustedConfig()
	cfg.MaxLen = 0
	body = defend.Sanitize(body, cfg)
	if strings.TrimSpace(body) == "" {
		return "", false, fmt.Errorf("无法从文件提取正文")
	}

	overhead := tokencost.CountTokens(fileEmbedModel, system+"\n"+question)
	remain := tokencost.AnalysisTokenLimit
	if b := tokencost.FromContext(ctx); b != nil {
		remain = b.Remain()
	}
	limit := remain - overhead
	if limit > tokencost.AnalysisTokenLimit {
		limit = tokencost.AnalysisTokenLimit
	}
	if limit < 1 {
		return "", false, tokencost.ErrRequestTokenLimit
	}

	cut := tokencost.TruncateToTokens(fileEmbedModel, body, limit)
	if strings.TrimSpace(cut) == "" {
		return "", false, tokencost.ErrRequestTokenLimit
	}
	truncated := tokencost.CountTokens(fileEmbedModel, body) > tokencost.CountTokens(fileEmbedModel, cut)
	return cut, truncated, nil
}

func analyzeFilePrompt(system, question, body string) string {
	nonce := defend.NewNonce()
	return strings.TrimSpace(system) + `

Use only the untrusted file frame. Reply in Chinese. If this is data analysis, omit row-level details.

File:
` + defend.Wrap(defend.KindTool, nonce, body) + `

Question:
` + defend.IsolateUser(question, nonce)
}

func (f *File) analyzeFileText(ctx context.Context, prompt string) (string, error) {
	if f == nil || f.svc == nil || f.svc.LLMs == nil {
		return "", fmt.Errorf("llm is not configured")
	}
	out, err := llms.GenerateFromSinglePrompt(ctx, f.svc.LLMs, prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
