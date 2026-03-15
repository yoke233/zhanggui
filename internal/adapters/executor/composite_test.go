package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	runtimeapp "github.com/yoke233/ai-workflow/internal/application/runtime"
	"github.com/yoke233/ai-workflow/internal/core"
)

type recordingSink struct {
	updates []acpclient.SessionUpdate
	err     error
}

func (s *recordingSink) HandleSessionUpdate(_ context.Context, update acpclient.SessionUpdate) error {
	s.updates = append(s.updates, update)
	return s.err
}

func TestNewMultiSink(t *testing.T) {
	if got := newMultiSink(nil, nil); got != nil {
		t.Fatalf("newMultiSink(nil,nil) = %#v, want nil", got)
	}

	sinkA := &recordingSink{}
	if got := newMultiSink(sinkA); got != sinkA {
		t.Fatalf("newMultiSink(single) = %#v, want same sink", got)
	}

	sinkB := &recordingSink{err: errors.New("ignored")}
	combined := newMultiSink(nil, sinkA, sinkB)
	if combined == nil {
		t.Fatal("expected combined sink")
	}
	if err := combined.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{Type: "agent_message", Text: "hello"}); err != nil {
		t.Fatalf("combined HandleSessionUpdate() error = %v", err)
	}
	if len(sinkA.updates) != 1 || len(sinkB.updates) != 1 {
		t.Fatalf("unexpected sink updates: sinkA=%d sinkB=%d", len(sinkA.updates), len(sinkB.updates))
	}
}

func TestNewCompositeActionExecutor(t *testing.T) {
	t.Run("nil step", func(t *testing.T) {
		execFn := NewCompositeActionExecutor(CompositeStepExecutorConfig{})
		if err := execFn(context.Background(), nil, nil); err == nil {
			t.Fatal("expected nil step to fail")
		}
	})

	t.Run("unknown builtin", func(t *testing.T) {
		execFn := NewCompositeActionExecutor(CompositeStepExecutorConfig{})
		err := execFn(context.Background(), &core.Action{Config: map[string]any{"builtin": "unknown"}}, nil)
		if err == nil || err.Error() != "unknown builtin executor: unknown" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing acp executor", func(t *testing.T) {
		execFn := NewCompositeActionExecutor(CompositeStepExecutorConfig{})
		err := execFn(context.Background(), &core.Action{}, nil)
		if err == nil || err.Error() != "ACP executor is not configured" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("falls back to acp executor", func(t *testing.T) {
		var called bool
		var gotStep *core.Action
		var gotRun *core.Run
		acpExec := flowapp.ActionExecutor(func(_ context.Context, step *core.Action, run *core.Run) error {
			called = true
			gotStep = step
			gotRun = run
			return nil
		})
		execFn := NewCompositeActionExecutor(CompositeStepExecutorConfig{ACPExecutor: acpExec})
		step := &core.Action{Name: "exec"}
		run := &core.Run{ID: 3}
		if err := execFn(context.Background(), step, run); err != nil {
			t.Fatalf("execFn() error = %v", err)
		}
		if !called || gotStep != step || gotRun != run {
			t.Fatalf("ACP executor not called as expected: called=%v step=%p run=%p", called, gotStep, gotRun)
		}
	})
}

var _ runtimeapp.EventSink = (*recordingSink)(nil)
