/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package toolx

import (
	"context"
	"encoding/json"
	"aiworkc/internal/domain"
	"aiworkc/internal/svc"
	"aiworkc/pkg/langchain/image"
)

type Img struct {
	svc *svc.ServiceContext
	*image.Image
}

func NewImg(svc *svc.ServiceContext) *Img {
	return &Img{
		svc:   svc,
		Image: image.NewImage(svc.OpenaiClient, svc.Config.Upload.SavePath),
	}
}

func (i *Img) Name() string {
	return "img"
}

func (i *Img) Description() string {
	return "image generation. use when the user wants an image"
}

func (i *Img) Call(ctx context.Context, input string) (string, error) {

	url, err := i.Gen(ctx, input, "")
	if err != nil {
		return "", err
	}

	data := domain.ChatResp{
		ChatType: domain.ImgAndText,
		Data: domain.ImgAndTextResp{
			Text: "",
			Url:  i.svc.Config.Upload.Host + "/" + url,
		},
	}

	body, err := json.Marshal(map[string]any{
		"chatType": domain.ImgAndText,
		"data":     data,
	})
	if err != nil {
		return "", err
	}

	return "Final Answer: " + string(body) + "\n", nil
}
