package flow

import (
	"context"
	"fmt"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

// TestHandleMergeConflictBlock_DirtyReturnsTrue: dirty merge error → handled (returns true).
func TestHandleMergeConflictBlock_DirtyReturnsTrue(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()
	eng := New(store, bus, nil, WithConcurrency(1))

	// Subscribe to events.
	sub := bus.Subscribe(core.SubscribeOpts{
		Types:      []core.EventType{core.EventGateAwaitingHuman},
		BufferSize: 10,
	})
	defer sub.Cancel()

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "conflict-test", Status: core.WorkItemRunning})
	actionID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "gate",
		Type:       core.ActionGate,
		Status:     core.ActionRunning,
		Position:   0,
	})
	action, _ := store.GetAction(ctx, actionID)

	mergeErr := &MergeError{
		Provider:       "github",
		Number:         42,
		URL:            "https://github.com/test/repo/pull/42",
		Message:        "This branch has conflicts",
		MergeableState: "dirty",
	}

	handled := eng.handleMergeConflictBlock(ctx, action, mergeErr)
	if !handled {
		t.Fatal("expected handleMergeConflictBlock to return true for dirty merge error")
	}

	// Verify action was blocked and has a context signal for merge conflict.
	updated, _ := store.GetAction(ctx, actionID)
	if updated.Status != core.ActionBlocked {
		t.Fatalf("expected action status=blocked, got %s", updated.Status)
	}
	ctxSignals, _ := store.ListActionSignalsByType(ctx, actionID, core.SignalContext)
	if len(ctxSignals) == 0 {
		t.Fatal("expected at least one context signal for merge conflict")
	}
	if ctxSignals[0].Summary != "merge_conflict" {
		t.Fatalf("expected context signal summary=merge_conflict, got %q", ctxSignals[0].Summary)
	}

	// Verify EventGateAwaitingHuman was published.
	found := false
	for {
		select {
		case ev := <-sub.C:
			if ev.Type == core.EventGateAwaitingHuman {
				found = true
			}
		default:
			goto done
		}
	}
done:
	if !found {
		t.Fatal("expected EventGateAwaitingHuman event")
	}
}

// TestHandleMergeConflictBlock_BehindReturnsFalse: "behind" merge error → not handled (returns false).
func TestHandleMergeConflictBlock_BehindReturnsFalse(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()
	eng := New(store, bus, nil, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "behind-test", Status: core.WorkItemRunning})
	actionID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "gate",
		Type:       core.ActionGate,
		Status:     core.ActionRunning,
		Position:   0,
	})
	action, _ := store.GetAction(ctx, actionID)

	mergeErr := &MergeError{
		Provider:       "github",
		Number:         42,
		Message:        "Branch is out of date",
		MergeableState: "behind",
	}

	handled := eng.handleMergeConflictBlock(ctx, action, mergeErr)
	if handled {
		t.Fatal("expected handleMergeConflictBlock to return false for 'behind' merge error")
	}

	// Action status should remain running (not blocked).
	updated, _ := store.GetAction(ctx, actionID)
	if updated.Status != core.ActionRunning {
		t.Fatalf("expected action status=running (unchanged), got %s", updated.Status)
	}
}

// TestHandleMergeConflictBlock_NonMergeErrorReturnsFalse: generic error → not handled.
func TestHandleMergeConflictBlock_NonMergeErrorReturnsFalse(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()
	eng := New(store, bus, nil, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "generic-err", Status: core.WorkItemRunning})
	actionID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "gate",
		Type:       core.ActionGate,
		Status:     core.ActionRunning,
		Position:   0,
	})
	action, _ := store.GetAction(ctx, actionID)

	genericErr := fmt.Errorf("workspace is required for merge")

	handled := eng.handleMergeConflictBlock(ctx, action, genericErr)
	if handled {
		t.Fatal("expected handleMergeConflictBlock to return false for generic error")
	}
}
