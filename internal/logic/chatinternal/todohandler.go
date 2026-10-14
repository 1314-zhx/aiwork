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

type TodoHandle struct {
	*baseChat
}

func NewTodoHandle(svc *svc.ServiceContext) *TodoHandle {
	return &TodoHandle{
		baseChat: NewOneShotChat(svc, []tools.Tool{
			toolx.NewTodoAdd(svc),
			toolx.NewTodoFind(svc),
		}),
	}
}

func (t *TodoHandle) Name() string {
	return "todo"
}

func (t *TodoHandle) Description() string {
	return "suitable for todo processing, such as todo creation, query, modification, dele tion, etc"
}
