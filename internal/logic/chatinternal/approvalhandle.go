/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package chatinternal

import (
	"github.com/tmc/langchaingo/tools"
	"aiworkc/internal/logic/chatinternal/toolx"
	"aiworkc/internal/svc"
)

type ApprovalHandle struct {
	*baseChat
}

func NewApprovalHandle(svc *svc.ServiceContext) *ApprovalHandle {
	return &ApprovalHandle{
		baseChat: NewOneShotChat(svc, []tools.Tool{
			toolx.NewApprovalAdd(svc),
			toolx.NewApprovalFind(svc),
		}),
	}
}
func (t *ApprovalHandle) Name() string {
	return "approval"
}

func (t *ApprovalHandle) Description() string {
	return "This is about approval matters. Such as sick leave, personal leave, going out, etc.\n\n"
}
