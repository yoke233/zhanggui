package web

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
)

type testRunExecutor struct {
	applyActionFn func(ctx context.Context, action core.RunAction) error
}

func (e *testRunExecutor) ApplyAction(ctx context.Context, action core.RunAction) error {
	if e.applyActionFn == nil {
		return nil
	}
	return e.applyActionFn(ctx, action)
}
