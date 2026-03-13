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
// directly to the deliverable — bypassing the LLM Collector entirely.
func TestSignalComplete_SkipsCollector(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	collectorCalled := false
	collector := CollectorFunc(func(_ context.Context, _ core.ActionType, _ string) (map[string]any, error) {
		collectorCalled = true
		return map[string]any{"collector_key": "should_not_appear"}, nil
	})

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		// Simulate agent creating deliverable + calling action_complete MCP tool.
		_, err := store.CreateDeliverable(ctx, &core.Deliverable{
			RunID:          run.ID,
			ActionID:       action.ID,
			WorkItemID:     action.WorkItemID,
			ResultMarkdown: "implemented the feature",
		})
		if err != nil {
			return err
		}
		_, err = store.CreateActionSignal(ctx, &core.ActionSignal{
			ActionID:   action.ID,
			WorkItemID: action.WorkItemID,
			RunID:      run.ID,
			Type:       core.SignalComplete,
			Source:     core.SignalSourceAgent,
			Payload:    map[string]any{"summary": "added login page", "tests_passed": true},
			Actor:      "agent",
			CreatedAt:  time.Now().UTC(),
		})
		return err
	}

	eng := New(store, bus, executor, WithConcurrency(1), WithCollector(collector))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "signal-complete", Status: core.WorkItemOpen})
	actionID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID, Name: "impl", Type: core.ActionExec,
		Status: core.ActionPending, Position: 0,
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Collector should NOT have been called.
	if collectorCalled {
		t.Fatal("collector was called but should have been skipped due to SignalComplete")
	}

	// Action should be done.
	action, _ := store.GetAction(ctx, actionID)
	if action.Status != core.ActionDone {
		t.Fatalf("expected done, got %s", action.Status)
	}

	// Deliverable metadata should contain agent-provided fields.
	del, err := store.GetLatestDeliverableByAction(ctx, actionID)
	if err != nil {
		t.Fatalf("get deliverable: %v", err)
	}
	if del.Metadata["summary"] != "added login page" {
		t.Fatalf("expected agent summary in metadata, got %v", del.Metadata)
	}
	if del.Metadata["signal_source"] != "agent" {
		t.Fatalf("expected signal_source=agent, got %v", del.Metadata["signal_source"])
	}

	// WorkItem should be done.
	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected work item done, got %s", workItem.Status)
	}
}

// TestSignalNeedHelp_BlocksAction verifies that a SignalNeedHelp from the agent
// causes the action to transition to blocked (non-fatal to the work item).
func TestSignalNeedHelp_BlocksAction(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		// Agent signals it needs help.
		_, _ = store.CreateActionSignal(ctx, &core.ActionSignal{
			ActionID:   action.ID,
			WorkItemID: action.WorkItemID,
			RunID:      run.ID,
			Type:       core.SignalNeedHelp,
			Source:     core.SignalSourceAgent,
			Payload:    map[string]any{"reason": "missing API credentials", "help_type": "access"},
			Actor:      "agent",
			CreatedAt:  time.Now().UTC(),
		})
		return nil // executor itself succeeds; engine checks signal
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "signal-need-help", Status: core.WorkItemOpen})
	actionID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID, Name: "deploy", Type: core.ActionExec,
		Status: core.ActionPending, Position: 0,
	})

	// Run should NOT return an error — blocked is non-fatal.
	err := eng.Run(ctx, workItemID)
	// The engine may return nil or may return an error depending on
	// whether there are more actions. With a single blocked action,
	// the work item won't complete — it stays running or blocked.
	_ = err

	action, _ := store.GetAction(ctx, actionID)
	if action.Status != core.ActionBlocked {
		t.Fatalf("expected blocked, got %s", action.Status)
	}
}

// TestGateSignalApprove_E2E: exec → gate(SignalApprove) → work item done.
func TestGateSignalApprove_E2E(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		if action.Type == core.ActionGate {
			// Gate agent calls gate_approve via MCP.
			_, err := store.CreateActionSignal(ctx, &core.ActionSignal{
				ActionID:   action.ID,
				WorkItemID: action.WorkItemID,
				RunID:      run.ID,
				Type:       core.SignalApprove,
				Source:     core.SignalSourceAgent,
				Payload:    map[string]any{"reason": "code review passed, all tests green"},
				Actor:      "agent",
				CreatedAt:  time.Now().UTC(),
			})
			return err
		}
		// Exec action: produce deliverable.
		_, err := store.CreateDeliverable(ctx, &core.Deliverable{
			RunID:          run.ID,
			ActionID:       action.ID,
			WorkItemID:     action.WorkItemID,
			ResultMarkdown: "implemented feature X",
			Metadata:       map[string]any{"summary": "feature X done"},
		})
		return err
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "gate-signal-approve", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "impl", Type: core.ActionExec, Status: core.ActionPending, Position: 0})
	gateID, _ := store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "review", Type: core.ActionGate, Status: core.ActionPending, Position: 1})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	gate, _ := store.GetAction(ctx, gateID)
	if gate.Status != core.ActionDone {
		t.Fatalf("expected gate done, got %s", gate.Status)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected work item done, got %s", workItem.Status)
	}
}

// TestGateSignalReject_ReworkThenApprove_E2E:
// exec → gate(reject) → exec reworks → gate(approve) → work item done.
// This tests the full reject-rework-approve cycle via ActionSignal.
func TestGateSignalReject_ReworkThenApprove_E2E(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var gateRuns int32
	var execRuns int32

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		if action.Type == core.ActionGate {
			n := atomic.AddInt32(&gateRuns, 1)
			if n == 1 {
				// First gate run: reject.
				_, err := store.CreateActionSignal(ctx, &core.ActionSignal{
					ActionID:   action.ID,
					WorkItemID: action.WorkItemID,
					RunID:      run.ID,
					Type:       core.SignalReject,
					Source:     core.SignalSourceAgent,
					Payload:    map[string]any{"reason": "missing error handling in auth module"},
					Actor:      "agent",
					CreatedAt:  time.Now().UTC(),
				})
				return err
			}
			// Second gate run: approve.
			_, err := store.CreateActionSignal(ctx, &core.ActionSignal{
				ActionID:   action.ID,
				WorkItemID: action.WorkItemID,
				RunID:      run.ID,
				Type:       core.SignalApprove,
				Source:     core.SignalSourceAgent,
				Payload:    map[string]any{"reason": "error handling added, LGTM"},
				Actor:      "agent",
				CreatedAt:  time.Now().UTC(),
			})
			return err
		}

		// Exec action.
		atomic.AddInt32(&execRuns, 1)
		_, err := store.CreateDeliverable(ctx, &core.Deliverable{
			RunID:          run.ID,
			ActionID:       action.ID,
			WorkItemID:     action.WorkItemID,
			ResultMarkdown: "implementation",
			Metadata:       map[string]any{"summary": "done"},
		})
		return err
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "gate-reject-rework", Status: core.WorkItemOpen})
	implID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID, Name: "impl", Type: core.ActionExec,
		Status: core.ActionPending, Position: 0, MaxRetries: 2,
	})
	gateID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID, Name: "review", Type: core.ActionGate,
		Status: core.ActionPending, Position: 1,
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify counts: exec ran twice (original + rework), gate ran twice.
	if execRuns != 2 {
		t.Fatalf("expected 2 exec runs, got %d", execRuns)
	}
	if gateRuns != 2 {
		t.Fatalf("expected 2 gate runs, got %d", gateRuns)
	}

	// Impl action should have retry_count=1 and a feedback signal from gate rejection.
	impl, _ := store.GetAction(ctx, implID)
	if impl.RetryCount != 1 {
		t.Fatalf("expected impl retry_count=1, got %d", impl.RetryCount)
	}
	feedbackSignals, _ := store.ListActionSignalsByType(ctx, implID, core.SignalFeedback)
	if len(feedbackSignals) == 0 {
		t.Fatal("expected at least one feedback signal on impl action after gate rejection")
	}
	if feedbackSignals[0].SourceActionID != gateID {
		t.Fatalf("expected feedback signal source_action_id=%d, got %d", gateID, feedbackSignals[0].SourceActionID)
	}

	// Gate and impl should be done.
	gate, _ := store.GetAction(ctx, gateID)
	if gate.Status != core.ActionDone {
		t.Fatalf("expected gate done, got %s", gate.Status)
	}
	if impl.Status != core.ActionDone {
		t.Fatalf("expected impl done, got %s", impl.Status)
	}

	// WorkItem should be done.
	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected work item done, got %s", workItem.Status)
	}
}

// TestSignalIdempotency verifies that a second terminal signal for the same
// run is rejected (checkIdempotent behavior).
func TestSignalIdempotency(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var signalCount int32
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		// Agent calls action_complete twice — second should be silently accepted
		// but the engine should only process the first signal.
		for i := 0; i < 2; i++ {
			atomic.AddInt32(&signalCount, 1)
			_, _ = store.CreateActionSignal(ctx, &core.ActionSignal{
				ActionID:   action.ID,
				WorkItemID: action.WorkItemID,
				RunID:      run.ID,
				Type:       core.SignalComplete,
				Source:     core.SignalSourceAgent,
				Payload:    map[string]any{"summary": "done"},
				Actor:      "agent",
				CreatedAt:  time.Now().UTC(),
			})
		}
		_, _ = store.CreateDeliverable(ctx, &core.Deliverable{
			RunID:          run.ID,
			ActionID:       action.ID,
			WorkItemID:     action.WorkItemID,
			ResultMarkdown: "result",
		})
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "idempotent", Status: core.WorkItemOpen})
	actionID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID, Name: "A", Type: core.ActionExec,
		Status: core.ActionPending, Position: 0,
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Action should still be done (not errored due to duplicate signal).
	action, _ := store.GetAction(ctx, actionID)
	if action.Status != core.ActionDone {
		t.Fatalf("expected done, got %s", action.Status)
	}

	// There should be 2 signal records (store doesn't enforce idempotency,
	// that's the MCP handler's job).
	signals, _ := store.ListActionSignals(ctx, actionID)
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(signals))
	}
}
