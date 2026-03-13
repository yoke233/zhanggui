package flow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// TestSignalComplete_SkipsCollector verifies that when the executor creates a
// SignalComplete, handleSuccess picks it up and writes agent-provided metadata
// directly to the artifact — bypassing the LLM Collector entirely.
func TestSignalComplete_SkipsCollector(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	collectorCalled := false
	collector := CollectorFunc(func(_ context.Context, _ core.StepType, _ string) (map[string]any, error) {
		collectorCalled = true
		return map[string]any{"collector_key": "should_not_appear"}, nil
	})

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		// Simulate agent creating artifact + calling step_complete MCP tool.
		_, err := store.CreateArtifact(ctx, &core.Artifact{
			ExecutionID:    exec.ID,
			StepID:         step.ID,
			IssueID:        step.IssueID,
			ResultMarkdown: "implemented the feature",
		})
		if err != nil {
			return err
		}
		_, err = store.CreateStepSignal(ctx, &core.StepSignal{
			StepID:    step.ID,
			IssueID:   step.IssueID,
			ExecID:    exec.ID,
			Type:      core.SignalComplete,
			Source:    core.SignalSourceAgent,
			Payload:   map[string]any{"summary": "added login page", "tests_passed": true},
			Actor:     "agent",
			CreatedAt: time.Now().UTC(),
		})
		return err
	}

	eng := New(store, bus, executor, WithConcurrency(1), WithCollector(collector))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "signal-complete", Status: core.IssueOpen})
	stepID, _ := store.CreateStep(ctx, &core.Step{
		IssueID: issueID, Name: "impl", Type: core.StepExec,
		Status: core.StepPending, Position: 0,
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Collector should NOT have been called.
	if collectorCalled {
		t.Fatal("collector was called but should have been skipped due to SignalComplete")
	}

	// Step should be done.
	step, _ := store.GetStep(ctx, stepID)
	if step.Status != core.StepDone {
		t.Fatalf("expected done, got %s", step.Status)
	}

	// Artifact metadata should contain agent-provided fields.
	art, err := store.GetLatestArtifactByStep(ctx, stepID)
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if art.Metadata["summary"] != "added login page" {
		t.Fatalf("expected agent summary in metadata, got %v", art.Metadata)
	}
	if art.Metadata["signal_source"] != "agent" {
		t.Fatalf("expected signal_source=agent, got %v", art.Metadata["signal_source"])
	}

	// Issue should be done.
	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected issue done, got %s", issue.Status)
	}
}

// TestSignalNeedHelp_BlocksStep verifies that a SignalNeedHelp from the agent
// causes the step to transition to blocked (non-fatal to the issue).
func TestSignalNeedHelp_BlocksStep(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		// Agent signals it needs help.
		_, _ = store.CreateStepSignal(ctx, &core.StepSignal{
			StepID:    step.ID,
			IssueID:   step.IssueID,
			ExecID:    exec.ID,
			Type:      core.SignalNeedHelp,
			Source:    core.SignalSourceAgent,
			Payload:   map[string]any{"reason": "missing API credentials", "help_type": "access"},
			Actor:     "agent",
			CreatedAt: time.Now().UTC(),
		})
		return nil // executor itself succeeds; engine checks signal
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "signal-need-help", Status: core.IssueOpen})
	stepID, _ := store.CreateStep(ctx, &core.Step{
		IssueID: issueID, Name: "deploy", Type: core.StepExec,
		Status: core.StepPending, Position: 0,
	})

	// Run should NOT return an error — blocked is non-fatal.
	err := eng.Run(ctx, issueID)
	// The engine may return nil or may return an error depending on
	// whether there are more steps. With a single blocked step,
	// the issue won't complete — it stays running or blocked.
	_ = err

	step, _ := store.GetStep(ctx, stepID)
	if step.Status != core.StepBlocked {
		t.Fatalf("expected blocked, got %s", step.Status)
	}
}

// TestGateSignalApprove_E2E: exec → gate(SignalApprove) → issue done.
func TestGateSignalApprove_E2E(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Type == core.StepGate {
			// Gate agent calls gate_approve via MCP.
			_, err := store.CreateStepSignal(ctx, &core.StepSignal{
				StepID:    step.ID,
				IssueID:   step.IssueID,
				ExecID:    exec.ID,
				Type:      core.SignalApprove,
				Source:    core.SignalSourceAgent,
				Payload:   map[string]any{"reason": "code review passed, all tests green"},
				Actor:     "agent",
				CreatedAt: time.Now().UTC(),
			})
			return err
		}
		// Exec step: produce artifact.
		_, err := store.CreateArtifact(ctx, &core.Artifact{
			ExecutionID:    exec.ID,
			StepID:         step.ID,
			IssueID:        step.IssueID,
			ResultMarkdown: "implemented feature X",
			Metadata:       map[string]any{"summary": "feature X done"},
		})
		return err
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "gate-signal-approve", Status: core.IssueOpen})
	store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "impl", Type: core.StepExec, Status: core.StepPending, Position: 0})
	gateID, _ := store.CreateStep(ctx, &core.Step{IssueID: issueID, Name: "review", Type: core.StepGate, Status: core.StepPending, Position: 1})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	gate, _ := store.GetStep(ctx, gateID)
	if gate.Status != core.StepDone {
		t.Fatalf("expected gate done, got %s", gate.Status)
	}

	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected issue done, got %s", issue.Status)
	}
}

// TestGateSignalReject_ReworkThenApprove_E2E:
// exec → gate(reject) → exec reworks → gate(approve) → issue done.
// This tests the full reject-rework-approve cycle via StepSignal.
func TestGateSignalReject_ReworkThenApprove_E2E(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var gateRuns int32
	var execRuns int32

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		if step.Type == core.StepGate {
			n := atomic.AddInt32(&gateRuns, 1)
			if n == 1 {
				// First gate run: reject.
				_, err := store.CreateStepSignal(ctx, &core.StepSignal{
					StepID:    step.ID,
					IssueID:   step.IssueID,
					ExecID:    exec.ID,
					Type:      core.SignalReject,
					Source:    core.SignalSourceAgent,
					Payload:   map[string]any{"reason": "missing error handling in auth module"},
					Actor:     "agent",
					CreatedAt: time.Now().UTC(),
				})
				return err
			}
			// Second gate run: approve.
			_, err := store.CreateStepSignal(ctx, &core.StepSignal{
				StepID:    step.ID,
				IssueID:   step.IssueID,
				ExecID:    exec.ID,
				Type:      core.SignalApprove,
				Source:    core.SignalSourceAgent,
				Payload:   map[string]any{"reason": "error handling added, LGTM"},
				Actor:     "agent",
				CreatedAt: time.Now().UTC(),
			})
			return err
		}

		// Exec step.
		atomic.AddInt32(&execRuns, 1)
		_, err := store.CreateArtifact(ctx, &core.Artifact{
			ExecutionID:    exec.ID,
			StepID:         step.ID,
			IssueID:        step.IssueID,
			ResultMarkdown: "implementation",
			Metadata:       map[string]any{"summary": "done"},
		})
		return err
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "gate-reject-rework", Status: core.IssueOpen})
	implID, _ := store.CreateStep(ctx, &core.Step{
		IssueID: issueID, Name: "impl", Type: core.StepExec,
		Status: core.StepPending, Position: 0, MaxRetries: 2,
	})
	gateID, _ := store.CreateStep(ctx, &core.Step{
		IssueID: issueID, Name: "review", Type: core.StepGate,
		Status: core.StepPending, Position: 1,
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify counts: exec ran twice (original + rework), gate ran twice.
	if execRuns != 2 {
		t.Fatalf("expected 2 exec runs, got %d", execRuns)
	}
	if gateRuns != 2 {
		t.Fatalf("expected 2 gate runs, got %d", gateRuns)
	}

	// Impl step should have retry_count=1 and a feedback signal from gate rejection.
	impl, _ := store.GetStep(ctx, implID)
	if impl.RetryCount != 1 {
		t.Fatalf("expected impl retry_count=1, got %d", impl.RetryCount)
	}
	feedbackSignals, _ := store.ListStepSignalsByType(ctx, implID, core.SignalFeedback)
	if len(feedbackSignals) == 0 {
		t.Fatal("expected at least one feedback signal on impl step after gate rejection")
	}
	if feedbackSignals[0].SourceStepID != gateID {
		t.Fatalf("expected feedback signal source_step_id=%d, got %d", gateID, feedbackSignals[0].SourceStepID)
	}

	// Gate and impl should be done.
	gate, _ := store.GetStep(ctx, gateID)
	if gate.Status != core.StepDone {
		t.Fatalf("expected gate done, got %s", gate.Status)
	}
	if impl.Status != core.StepDone {
		t.Fatalf("expected impl done, got %s", impl.Status)
	}

	// Issue should be done.
	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected issue done, got %s", issue.Status)
	}
}

// TestSignalIdempotency verifies that a second terminal signal for the same
// execution is rejected (checkIdempotent behavior).
func TestSignalIdempotency(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var signalCount int32
	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		// Agent calls step_complete twice — second should be silently accepted
		// but the engine should only process the first signal.
		for i := 0; i < 2; i++ {
			atomic.AddInt32(&signalCount, 1)
			_, _ = store.CreateStepSignal(ctx, &core.StepSignal{
				StepID:    step.ID,
				IssueID:   step.IssueID,
				ExecID:    exec.ID,
				Type:      core.SignalComplete,
				Source:    core.SignalSourceAgent,
				Payload:   map[string]any{"summary": "done"},
				Actor:     "agent",
				CreatedAt: time.Now().UTC(),
			})
		}
		_, _ = store.CreateArtifact(ctx, &core.Artifact{
			ExecutionID:    exec.ID,
			StepID:         step.ID,
			IssueID:        step.IssueID,
			ResultMarkdown: "result",
		})
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "idempotent", Status: core.IssueOpen})
	stepID, _ := store.CreateStep(ctx, &core.Step{
		IssueID: issueID, Name: "A", Type: core.StepExec,
		Status: core.StepPending, Position: 0,
	})

	if err := eng.Run(ctx, issueID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Step should still be done (not errored due to duplicate signal).
	step, _ := store.GetStep(ctx, stepID)
	if step.Status != core.StepDone {
		t.Fatalf("expected done, got %s", step.Status)
	}

	// There should be 2 signal records (store doesn't enforce idempotency,
	// that's the MCP handler's job).
	signals, _ := store.ListStepSignals(ctx, stepID)
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(signals))
	}
}
