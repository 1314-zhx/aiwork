/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aiworkc/internal/domain"
	"aiworkc/internal/svc"
	"aiworkc/pkg/filesecure"
	"aiworkc/pkg/httpx"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/router"
	"aiworkc/pkg/langchain/visual"
	"aiworkc/pkg/token"

	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/outputparser"
	"github.com/tmc/langchaingo/prompts"
)

type File struct {
	svc          *svc.ServiceContext
	c            chains.Chain
	outputParser outputparser.Structured
	visual       chains.Chain
	callbacks    callbacks.Handler
}

func NewFile(svc *svc.ServiceContext) *File {
	return &File{
		svc:          svc,
		outputParser: _defaultFileOutputParser,
		c: chains.NewLLMChain(svc.LLMs, prompts.PromptTemplate{
			Template:       _defaultFileQuery,
			InputVariables: []string{langchain.Input, "history"},
			TemplateFormat: prompts.TemplateFormatGoTemplate,
			PartialVariables: map[string]any{
				"formatting": _defaultFileOutputParser.GetFormatInstructions(),
			},
		}),
		visual: visual.NewVisualChains(svc.LLMs, svc.Config.Upload.SavePath),
	}
}

func (d *File) Name() string {
	return "file"
}

func (d *File) Description() string {
	return `use when the user or history points to a specific file to analyze`
}

func (d *File) Chains() chains.Chain {
	return chains.NewTransform(d.transform, nil, nil)
}

func (d *File) transform(ctx context.Context, inputs map[string]any, options ...chains.ChainCallOption) (map[string]any,
	error) {
	fmt.Println("analysis --- file --- start")
	if err := RequireAnalysisAccess(ctx, d.svc); err != nil {
		return nil, err
	}

	rawInput := inputs[langchain.Input].(string)
	safeInput := defend.Sanitize(rawInput, defend.DefaultSanitizeConfig())
	inputs[langchain.Input] = safeInput
	if _, ok := inputs["history"]; !ok {
		inputs["history"] = ""
	}
	if rawHistory, ok := inputs["history"].(string); ok {
		inputs["history"] = defend.Sanitize(rawHistory, defend.SanitizeHistoryConfig())
	}

	out, err := chains.Predict(ctx, d.c, inputs, options...)
	if err != nil {
		return nil, err
	}

	query, err := d.extractQuery(ctx, out)
	if err != nil {
		if _, ok := router.AsNotHandles(err); ok {
			return nil, err
		}
		return nil, missingFile(filePartial(nil, safeInput))
	}

	fileId, _, _, hasFile := fileIdentity(query)
	if !hasFile {
		return nil, missingFile(filePartial(query, safeInput))
	}

	abs, localID, err := d.resolveLocalFile(ctx, query)
	if err != nil {
		return nil, err
	}
	if fileId == "" || fileId == "1" {
		fileId = localID
	}

	rawBody, err := readAnalysisFileText(abs)
	if err != nil {
		return nil, err
	}
	sys := query[System] + " If this is data analysis, omit row-level details. Reply in Chinese."
	body, truncated, err := fitFileBody(ctx, sys, safeInput, rawBody)
	if err != nil {
		return nil, err
	}

	answer, err := d.analyzeFileText(ctx, analyzeFilePrompt(sys, safeInput, body))
	if err != nil {
		return nil, err
	}
	if truncated {
		answer += "\n（文件正文已按本次 4000 token 限额截断）"
	}

	v, err := chains.Call(ctx, d.visual, map[string]any{
		langchain.Input: visualInput(safeInput, answer),
	})

	var resp domain.ChatResp
	if err != nil {
		fmt.Println("visual : ", err)
		resp = domain.ChatResp{
			ChatType: 0,
			Data:     answer,
		}
	} else {
		imgUrl, ok := v[visual.ImgUrl].(string)
		if !ok || imgUrl == "" {
			resp = domain.ChatResp{
				ChatType: 0,
				Data:     answer,
			}
		} else {
			resp = domain.ChatResp{
				ChatType: domain.ImgAndText,
				Data: domain.ImgAndTextResp{
					Text: answer,
					Url:  httpx.JoinHostPath(d.svc.Config.Upload.Host, imgUrl),
				},
			}
		}
	}
	respBytes, err := json.Marshal(&resp)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		langchain.OutPut: string(respBytes),
		langchain.Input:  fmt.Sprintf("%s fileId : %s", query[FileName], fileId),
	}, nil
}

func (f *File) extractQuery(ctx context.Context, text string) (map[string]string, error) {
	res, err := f.outputParser.Parse(text)
	if err != nil {
		return nil, err
	}

	data, ok := res.(map[string]string)
	if !ok {
		return nil, missingFile(nil)
	}

	return data, nil
}

func (f *File) resolveLocalFile(ctx context.Context, fileInfo map[string]string) (abs, fileId string, err error) {
	uid := token.GetUId(ctx)
	fileId = strings.TrimSpace(fileInfo[FileId])
	if fileId != "" && fileId != "1" && f.svc.Files != nil {
		if rec, err := f.svc.Files.Get(uid, fileId, false); err == nil {
			fileInfo[FilePath] = rec.AbsPath
			if fileInfo[FileName] == "" {
				fileInfo[FileName] = rec.OriginalName
			}
			return rec.AbsPath, rec.ID, nil
		}
	}
	rel := strings.TrimSpace(fileInfo[FilePath])
	if rel == "" {
		rel = strings.TrimSpace(fileInfo[FileName])
	}
	abs, err = filesecure.Resolve(f.svc.Config.Upload.SavePath, filepath.Base(strings.ReplaceAll(rel, `\`, "/")))
	if err != nil {
		return "", "", missingFile(filePartial(fileInfo, ""))
	}
	if _, err := os.Stat(abs); err != nil {
		return "", "", missingFile(filePartial(fileInfo, ""))
	}
	return abs, filepath.Base(abs), nil
}
