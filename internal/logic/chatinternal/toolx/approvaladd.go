/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package toolx

import (
	"context"
	"errors"

	"aiworkc/internal/logic/chatinternal/toolx/approval"
	"aiworkc/internal/model"
	"aiworkc/internal/svc"
	"aiworkc/pkg/langchain/outputparserx"
	"github.com/tmc/langchaingo/callbacks"
)

var errApprovalFieldsParse = errors.New("approval fields parse failed")

type ApprovalAdd struct {
	svc          *svc.ServiceContext
	Callback     callbacks.Handler
	outputparser outputparserx.Structured
}

func NewApprovalAdd(svc *svc.ServiceContext) *ApprovalAdd {
	return &ApprovalAdd{
		svc:      svc,
		Callback: svc.Callbacks,
		outputparser: outputparserx.NewStructured([]outputparserx.ResponseSchema{
			{
				Name: "type",
				Description: "approval type; enum : 1. General, 2. Leave Approval, 3. " +
					"Card replacement Approval, 4. Go out Approval",
				Type: "int",
			}, {
				Name:        "input",
				Description: "The user's original input",
			},
		}),
	}
}

func (a *ApprovalAdd) Name() string {
	return "approval_add"
}

func (a *ApprovalAdd) ParameterSchema() any {
	return a.outputparser.JSONSchema()
}

func (a *ApprovalAdd) Description() string {
	return `
	extract approval fields for user confirmation.
	do not write to database and do not claim the approval is saved.
	if fields cannot be parsed, return an error instead of a draft.
	Reply in Chinese.
` + a.outputparser.GetFormatInstructions()
}

func (a *ApprovalAdd) Call(ctx context.Context, input string) (string, error) {
	if a.Callback != nil {
		a.Callback.HandleText(ctx, "approval add start input : "+input)
	}

	out, err := a.outputparser.Parse(input)
	if err != nil {
		return "", err
	}
	data, ok := out.(map[string]any)
	if !ok {
		return "", errApprovalFieldsParse
	}

	var approvalType float64
	switch t := data["type"].(type) {
	case float64:
		approvalType = t
	case int:
		approvalType = float64(t)
	default:
		return "", errApprovalFieldsParse
	}
	if v, ok := data["input"].(string); ok && v != "" {
		input = v
	}

	ap, err := approval.NewApproval(a.svc, model.ApprovalType(approvalType))
	if err != nil {
		return "", err
	}

	return ap.Extract(ctx, input)
}
