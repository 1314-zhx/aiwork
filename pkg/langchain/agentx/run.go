package agentx

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/langchain/tokenretry"

	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/tools"
)

const intermediateStepsKey = "intermediateSteps"

// Run executes the strict agent, optionally resuming from saved observations.
func Run(ctx context.Context, agent *StrictReActAgent, maxIterations int, inputs map[string]string, startSteps []schema.AgentStep, errHandler *agents.ParserErrorHandler) (map[string]any, error) {
	if agent == nil {
		return nil, errors.New("agent is required")
	}
	if maxIterations <= 0 {
		maxIterations = 1
	}
	steps := append([]schema.AgentStep(nil), startSteps...)
	tokenretry.Record(ctx, steps, inputs)
	nameToTool := map[string]tools.Tool{}
	for _, tool := range agent.GetTools() {
		nameToTool[strings.ToUpper(tool.Name())] = tool
	}

	extra := 0
	for i := 0; i < maxIterations+extra; i++ {
		if len(steps) > 0 {
			if err := tokencost.FromContext(ctx).CanAfford(tokencost.AgentFirstMaxTokens); err != nil {
				tokenretry.Record(ctx, steps, inputs)
				return nil, err
			}
		}
		actions, finish, err := agent.Plan(ctx, steps, inputs)
		if errors.Is(err, agents.ErrUnableToParseOutput) && errHandler != nil {
			formatted := err.Error()
			if errHandler.Formatter != nil {
				formatted = errHandler.Formatter(formatted)
			}
			steps = append(steps, schema.AgentStep{Observation: formatted})
			tokenretry.Record(ctx, steps, inputs)
			continue
		}
		if err != nil {
			tokenretry.Record(ctx, steps, inputs)
			return nil, err
		}
		if len(actions) == 0 && finish == nil {
			return nil, agents.ErrAgentNoReturn
		}
		if finish != nil {
			if finish.ReturnValues == nil {
				finish.ReturnValues = map[string]any{}
			}
			finish.ReturnValues[intermediateStepsKey] = steps
			return finish.ReturnValues, nil
		}
		for _, action := range actions {
			if allow, gated := gateToolCall(steps, action); !allow {
				steps = append(steps, schema.AgentStep{Action: action, Observation: gated})
				tokenretry.Record(ctx, steps, inputs)
				if extra == 0 {
					extra = 1
				}
				continue
			}
			tool, ok := nameToTool[strings.ToUpper(action.Tool)]
			if !ok {
				steps = append(steps, schema.AgentStep{
					Action:      action,
					Observation: toolErrorObservation(fmt.Errorf("%s is not a valid tool, try another one", action.Tool)),
				})
				tokenretry.Record(ctx, steps, inputs)
				continue
			}
			observation, err := tool.Call(ctx, action.ToolInput)
			if err != nil {
				steps = append(steps, schema.AgentStep{Action: action, Observation: toolErrorObservation(err)})
				tokenretry.Record(ctx, steps, inputs)
				continue
			}
			steps = append(steps, schema.AgentStep{Action: action, Observation: observation})
			tokenretry.Record(ctx, steps, inputs)
		}
	}
	return map[string]any{intermediateStepsKey: steps}, agents.ErrNotFinished
}
