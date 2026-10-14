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

type Knowledge struct {
	*baseChat
}

func NewKnowledge(svc *svc.ServiceContext) *Knowledge {
	return &Knowledge{NewBaseChat(svc, []tools.Tool{
		toolx.NewKnowledgeUpdate(svc),
		toolx.NewKnowledgeRetrievalQA(svc),
	})}
}

func (t *Knowledge) Name() string {
	return "knowledge"
}

func (t *Knowledge) Description() string {
	return `This is the company's knowledge base.
Can answer employee consultation questions about company systems such as approval process, leave matters, attendance matters, employee manuals and other office content.
Can also be used for updating the knowledge base.
For policy questions the incoming text may already be an answer-like retrieval passage. Always call knowledge_retrieval_qa with that passage. Never treat the passage as the final company rule.`
}
