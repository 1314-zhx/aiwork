package agentx

import (
	"context"
	"testing"

	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/langchain/tokenretry"

	"github.com/tmc/langchaingo/schema"
)

func TestRunStopsNextHopWhenFirstHopBudgetMissing(t *testing.T) {
	ctx := tokenretry.WithSession(context.Background())
	ctx = tokencost.WithBudget(ctx, tokencost.NewBudget(100))
	tokencost.FromContext(ctx).Add(80, 0)

	steps := []schema.AgentStep{{
		Action:      schema.AgentAction{Tool: "todo_find", ToolInput: `{}`},
		Observation: "found",
	}}
	_, err := Run(ctx, &StrictReActAgent{}, 2, map[string]string{"input": "q"}, steps, nil)
	if !tokencost.IsRequestLimit(err) {
		t.Fatalf("got %v", err)
	}
	sess := tokenretry.FromSession(ctx)
	if sess == nil || len(sess.Steps) != 1 || sess.Steps[0].Observation != "found" {
		t.Fatalf("steps not kept: %+v", sess)
	}
}
