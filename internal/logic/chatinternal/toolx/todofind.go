/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package toolx

import (
	"aiworkc/internal/domain"
	"aiworkc/internal/svc"
	"aiworkc/pkg/curl"
	"aiworkc/pkg/langchain/outputparserx"
	"aiworkc/pkg/token"
	"context"
	"fmt"
	"github.com/tmc/langchaingo/callbacks"
)

type TodoFind struct {
	svc          *svc.ServiceContext
	callback     callbacks.Handler
	outputparser outputparserx.Structured
}

func NewTodoFind(svc *svc.ServiceContext) *TodoFind {
	return &TodoFind{
		svc:      svc,
		callback: svc.Callbacks,
		outputparser: outputparserx.NewStructured([]outputparserx.ResponseSchema{
			{
				Name:        "id",
				Description: "todo id",
				Type:        "string",
			},
			{
				Name:        "startTime",
				Description: "start time, data application time stamp, such as 1720921573. none is empty",
				Type:        "int64",
			},
			{
				Name:        "endTime",
				Description: "end time, data application time stamp, such as 1720921573. none is empty",
				Type:        "int64",
			},
			{
				Name:        "userId",
				Description: "user id",
				Type:        "string",
			},
		}),
	}
}

func (t *TodoFind) Name() string {
	return "todo_find"
}

func (t *TodoFind) ParameterSchema() any {
	return t.outputparser.JSONSchema()
}

func (t *TodoFind) Description() string {
	return `
	a todo find interface.
	use when you need to find a todo.
	If the condition is null, return {}
	Reply in Chinese.` + t.outputparser.GetFormatInstructions()
}

func (t *TodoFind) Call(ctx context.Context, input string) (string, error) {
	if t.callback != nil {
		t.callback.HandleText(ctx, "todo find start : "+input)
	}

	out, err := t.outputparser.Parse(input)
	if err != nil {
		fmt.Println("todo find ", err.Error())
		return "", err
	}

	data := out.(map[string]any)
	if data == nil {
		data = make(map[string]any)
	}
	data["userId"] = token.GetUId(ctx)
	data["count"] = 10
	conversionTime("startTime", data)
	conversionTime("endTime", data)

	res, err := curl.GetRequest(token.GetTokenStr(ctx), t.svc.Config.Host+"/v1/todo/list", data)

	return ResParser(res, domain.TodoFind, err)
}

func conversionTime(filed string, data map[string]any) {
	if v, ok := data[filed]; ok {
		tmp := v.(float64)
		data[filed] = int64(tmp)
	}
}
