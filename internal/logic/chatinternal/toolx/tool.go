/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package toolx

import (
	"encoding/json"
	"errors"
	"aiworkc/internal/domain"
	"aiworkc/pkg/httpx"
)

var (
	Success = `executes successfully.`
)

// ConfirmDraftAnswer 把抽出的业务字段交给前端确认。调用方不得在此之前写库。
func ConfirmDraftAnswer(chatType domain.AiChatType, data any) (string, error) {
	body, err := json.Marshal(domain.ChatResp{
		ChatType: int(chatType),
		Data:     data,
	})
	if err != nil {
		return "", err
	}
	return "Final Answer: " + string(body) + "\n单据尚未写入数据库。请将上述 JSON 原样作为最终答案，等待用户点击提交。不要再次调用创建类工具。\n", nil
}

func ResParser(v []byte, chatType domain.AiChatType, err error) (string, error) {
	if err != nil {
		return "", err
	}

	var res httpx.Response
	if err := json.Unmarshal(v, &res); err != nil {
		return "", err
	}

	if res.Code != 200 {
		return "", errors.New(res.Msg)
	}

	switch chatType {
	case domain.TodoAdd:
		return Success, err
	}

	data := domain.ChatResp{
		ChatType: int(chatType),
		Data:     res.Data,
	}

	d, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return "Final Answer: " + string(d) + "\n", nil

}
