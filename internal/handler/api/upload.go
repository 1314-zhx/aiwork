package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"aiworkc/internal/domain"
	"aiworkc/internal/logic"
	"aiworkc/internal/svc"
	"aiworkc/pkg/audit"
	"aiworkc/pkg/filesecure"
	"aiworkc/pkg/httpx"
	"aiworkc/pkg/token"
	"github.com/gin-gonic/gin"
	"github.com/segmentio/ksuid"
	"go.mongodb.org/mongo-driver/mongo"
)

type Upload struct {
	svcCtx *svc.ServiceContext
	chat   logic.Chat
}

func NewUpload(svcCtx *svc.ServiceContext, chat logic.Chat) *Upload {
	return &Upload{
		svcCtx: svcCtx,
		chat:   chat,
	}
}

func (h *Upload) InitRegister(engine *gin.Engine) {
	g := engine.Group("v1/upload", h.svcCtx.Jwt.Handler)
	g.POST("/file", h.File)
	g.POST("/multiplefiles", h.Multiplefiles)
}

func (h *Upload) File(ctx *gin.Context) {
	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	defer file.Close()

	buf := bytes.NewBuffer(nil)
	limited := io.LimitReader(file, filesecure.MaxBytes+1)
	if _, err := io.Copy(buf, limited); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	reqCtx := ctx.Request.Context()
	var ext string
	if h.svcCtx != nil && h.svcCtx.Whitelist != nil {
		row, findErr := h.svcCtx.Whitelist.FindFileExt(reqCtx, filesecure.ExtOf(header.Filename))
		if findErr != nil {
			if errors.Is(findErr, mongo.ErrNoDocuments) {
				httpx.FailWithErr(ctx, filesecure.ErrBadType)
				return
			}
			httpx.FailWithErr(ctx, findErr)
			return
		}
		ext, err = filesecure.CheckAllowed(header.Filename, buf.Bytes(), row.Magic)
	} else {
		ext, err = filesecure.Check(header.Filename, buf.Bytes())
	}
	if err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}

	filename := ksuid.New().String() + ext
	absPath, err := filesecure.Resolve(h.svcCtx.Config.Upload.SavePath, filename)
	if err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	newFile, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}
	defer newFile.Close()
	if _, err := newFile.Write(buf.Bytes()); err != nil {
		httpx.FailWithErr(ctx, err)
		return
	}

	uid := token.GetUId(ctx.Request.Context())
	h.svcCtx.Files.Put(filesecure.Record{
		ID:           filename,
		OwnerID:      uid,
		AbsPath:      absPath,
		Ext:          ext,
		Size:         int64(buf.Len()),
		OriginalName: header.Filename,
	})
	audit.Event(ctx.Request.Context(), "upload.file", "uid", uid, "fileId", filename, "size", buf.Len())

	resp := domain.FileResp{
		Host:     h.svcCtx.Config.Host,
		File:     filename,
		Filename: filename,
		FileId:   filename,
	}

	chat := ctx.Request.FormValue("chat")
	if len(chat) > 0 {
		if err := h.chat.File(ctx.Request.Context(), []*domain.FileResp{&resp}); err != nil {
			httpx.FailWithErr(ctx, err)
			return
		}
	}
	httpx.OkWithData(ctx, resp)
}

func (h *Upload) Multiplefiles(ctx *gin.Context) {
	httpx.FailWithErr(ctx, fmt.Errorf("not implemented"))
}
