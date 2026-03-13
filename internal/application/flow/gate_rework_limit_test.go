package flow

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

// TestGateReworkLimit_DefaultBlocksAfter3Rounds: gate always rejects → blocks after 3 rework rounds.
func TestGateReworkLimit_DefaultBlocksAfter3Rounds(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Type == core.StepGate {
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				IssueID:        step.IssueID,
				ResultMarkdown: "Review feedback",
				Metadata:       map[string]any{"verdict": "reject", "reason": "conflict"},
			})
			return err
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "rework-limit", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "impl",
		Type:       core.StepExec,
		Status:     core.StepPending,
		Position:   0,
		MaxRetries: 10, // high limit so upstream doesn't block first
	})
	gateID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:  issueID,
		Name:     "review",
		Type:     core.StepGate,
		Status:   core.StepPending,
		Position: 1,
		// No max_rework_rounds config → default 3
	})

	// Run should stop because gate reaches rework limit → blocked → engine sees "stuck".
	err := eng.Run(ctx, issueID)
	if err == nil || !strings.Contains(err.Error(), "stuck") {
		t.Fatalf("expected 'stuck' error when gate is blocked, got: %v", err)
	}

	gate, _ := store.GetStep(ctx, gateID)
	if gate.Status != core.StepBlocked {
		t.Fatalf("expected gate status=blocked, got %s", gate.Status)
	}

	// Verify rework_count via signal count (single source of truth).
	rejectCount, _ := store.CountStepSignals(ctx, gateID, core.SignalReject)
	if rejectCount < 3 {
		t.Fatalf("expected at least 3 reject signals, got %d", rejectCount)
	}
}

// TestGateReworkLimit_CustomLimit: max_rework_rounds=1 blocks after 1 round.
func TestGateReworkLimit_CustomLimit(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var gateCount int32
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Type == core.StepGate {
			atomic.AddInt32(&gateCount, 1)
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				IssueID:        step.IssueID,
				ResultMarkdown: "Review feedback",
				Metadata:       map[string]any{"verdict": "reject", "reason": "always reject"},
			})
			return err
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "rework-limit-custom", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "impl",
		Type:       core.StepExec,
		Status:     core.StepPending,
		Position:   0,
		MaxRetries: 10,
	})
	gateID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:  issueID,
		Name:     "review",
		Type:     core.StepGate,
		Status:   core.StepPending,
		Position: 1,
		Config: map[string]any{
			"max_rework_rounds": float64(1),
		},
	})

	err := eng.Run(ctx, issueID)
	if err == nil || !strings.Contains(err.Error(), "stuck") {
		t.Fatalf("expected 'stuck' error when gate is blocked, got: %v", err)
	}

	gate, _ := store.GetStep(ctx, gateID)
	if gate.Status != core.StepBlocked {
		t.Fatalf("expected gate status=blocked, got %s", gate.Status)
	}

	// Gate should have been evaluated twice: round 1 reject (rework_count→1) + round 2 reject (rework_count=1 >= max=1 → blocked).
	if gateCount != 2 {
		t.Fatalf("expected 2 gate evaluations, got %d", gateCount)
	}
}

// TestGateReworkLimit_PassBeforeLimit: gate rejects once then passes → no blocking.
func TestGateReworkLimit_PassBeforeLimit(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var gateCount int32
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Type == core.StepGate {
			n := atomic.AddInt32(&gateCount, 1)
			verdict := "reject"
			if n > 1 {
				verdict = "pass"
			}
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				IssueID:        step.IssueID,
				ResultMarkdown: "Review feedback",
				Metadata:       map[string]any{"verdict": verdict, "reason": fmt.Sprintf("round %d", n)},
			})
			return err
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "rework-passes", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "impl",
		Type:       core.StepExec,
		Status:     core.StepPending,
		Position:   0,
		MaxRetries: 5,
	})
	gateID, _ := store.CreateStep(ctx, &core.Step{
		IssueID:  issueID,
		Name:     "review",
		Type:     core.StepGate,
		Status:   core.StepPending,
		Position: 1,
		Config: map[string]any{
			"max_rework_rounds": float64(3),
		},
	})

	err := eng.Run(ctx, issueID)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	gate, _ := store.GetStep(ctx, gateID)
	if gate.Status != core.StepDone {
		t.Fatalf("expected gate status=done, got %s", gate.Status)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected issue done, got %s", issue.Status)
	}
}

// TestGateReworkLimit_EventPublished: EventGateReworkLimitReached is published when limit reached.
func TestGateReworkLimit_EventPublished(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Type == core.StepGate {
			_, err := store.CreateArtifact(ctx, &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				IssueID:        step.IssueID,
				ResultMarkdown: "Review feedback",
				Metadata:       map[string]any{"verdict": "reject", "reason": "conflict"},
			})
			return err
		}
		return nil
	}

	// Subscribe to rework limit events before running.
	sub := bus.Subscribe(core.SubscribeOpts{
		Types:      []core.EventType{core.EventGateReworkLimitReached},
		BufferSize: 10,
	})
	defer sub.Cancel()

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "rework-event", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{
		IssueID:    issueID,
		Name:       "impl",
		Type:       core.StepExec,
		Status:     core.StepPending,
		Position:   0,
		MaxRetries: 10,
	})
	store.CreateStep(ctx, &core.Step{
		IssueID:  issueID,
		Name:     "review",
		Type:     core.StepGate,
		Status:   core.StepPending,
		Position: 1,
		Config: map[string]any{
			"max_rework_rounds": float64(2),
		},
	})

	_ = eng.Run(ctx, issueID)

	// Drain events and check for rework limit event.
	found := false
	for {
		select {
		case ev := <-sub.C:
			if ev.Type == core.EventGateReworkLimitReached {
				found = true
				reworkCount, _ := ev.Data["rework_count"].(int)
				maxRounds, _ := ev.Data["max_rework_rounds"].(int)
				if reworkCount != 2 {
					t.Fatalf("expected rework_count=2 in event, got %d", reworkCount)
				}
				if maxRounds != 2 {
					t.Fatalf("expected max_rework_rounds=2 in event, got %d", maxRounds)
				}
			}
		default:
			goto done
		}
	}
done:
	if !found {
		t.Fatal("expected EventGateReworkLimitReached to be published")
	}
}
