package tokenretry

import (
	"context"

	"github.com/tmc/langchaingo/schema"
)

type ctxKey struct{}

// Session records the last ReAct hop so a token failure can leave a retry ticket.
type Session struct {
	Handler string
	Steps   []schema.AgentStep
	Inputs  map[string]string
}

func WithSession(ctx context.Context) context.Context {
	if FromSession(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, &Session{})
}

func FromSession(ctx context.Context) *Session {
	s, _ := ctx.Value(ctxKey{}).(*Session)
	return s
}

func SetHandler(ctx context.Context, name string) {
	if s := FromSession(ctx); s != nil && name != "" {
		s.Handler = name
	}
}

func Record(ctx context.Context, steps []schema.AgentStep, inputs map[string]string) {
	s := FromSession(ctx)
	if s == nil {
		return
	}
	s.Steps = append([]schema.AgentStep(nil), steps...)
	if inputs != nil {
		copied := make(map[string]string, len(inputs))
		for k, v := range inputs {
			copied[k] = v
		}
		s.Inputs = copied
	}
}

func ResumeSteps(ctx context.Context) []schema.AgentStep {
	s := FromSession(ctx)
	if s == nil {
		return nil
	}
	return append([]schema.AgentStep(nil), s.Steps...)
}

func WithResume(ctx context.Context, handler string, steps []schema.AgentStep, inputs map[string]string) context.Context {
	ctx = WithSession(ctx)
	s := FromSession(ctx)
	s.Handler = handler
	s.Steps = append([]schema.AgentStep(nil), steps...)
	if inputs != nil {
		copied := make(map[string]string, len(inputs))
		for k, v := range inputs {
			copied[k] = v
		}
		s.Inputs = copied
	}
	return ctx
}
