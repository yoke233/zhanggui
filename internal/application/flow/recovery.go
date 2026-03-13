package flow

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yoke233/ai-workflow/internal/core"
)

// RecoverInterruptedWorkItems scans the store for work items that were running or queued
// when the process last stopped, resets their in-progress actions, and re-submits
// them to the scheduler for execution.
//
// Call this once during bootstrap, after the scheduler has been started.
func RecoverInterruptedWorkItems(ctx context.Context, store Store, scheduler *WorkItemScheduler) (int, error) {
	return recoverWorkItemsByStatus(ctx, store, scheduler, []core.WorkItemStatus{core.WorkItemQueued, core.WorkItemRunning})
}

// RecoverQueuedWorkItems re-enqueues work items that were queued before the process stopped.
func RecoverQueuedWorkItems(ctx context.Context, store Store, scheduler *WorkItemScheduler) (int, error) {
	return recoverWorkItemsByStatus(ctx, store, scheduler, []core.WorkItemStatus{core.WorkItemQueued})
}

// RecoverInterruptedIssues is an alias for backward compatibility.
func RecoverInterruptedIssues(ctx context.Context, store Store, scheduler *WorkItemScheduler) (int, error) {
	return RecoverInterruptedWorkItems(ctx, store, scheduler)
}

// RecoverInterruptedFlows is an alias for backward compatibility.
func RecoverInterruptedFlows(ctx context.Context, store Store, scheduler *WorkItemScheduler) (int, error) {
	return RecoverInterruptedWorkItems(ctx, store, scheduler)
}

// RecoverQueuedIssues is an alias for backward compatibility.
func RecoverQueuedIssues(ctx context.Context, store Store, scheduler *WorkItemScheduler) (int, error) {
	return RecoverQueuedWorkItems(ctx, store, scheduler)
}

// RecoverQueuedFlows is an alias for backward compatibility.
func RecoverQueuedFlows(ctx context.Context, store Store, scheduler *WorkItemScheduler) (int, error) {
	return RecoverQueuedWorkItems(ctx, store, scheduler)
}

func recoverWorkItemsByStatus(ctx context.Context, store Store, scheduler *WorkItemScheduler, statuses []core.WorkItemStatus) (int, error) {
	recovered := 0

	for _, status := range statuses {
		workItems, err := store.ListWorkItems(ctx, core.WorkItemFilter{Status: &status, Limit: 1000})
		if err != nil {
			return recovered, fmt.Errorf("list %s work items: %w", status, err)
		}

		for _, wi := range workItems {
			if err := recoverWorkItem(ctx, store, scheduler, wi); err != nil {
				slog.Error("recovery: failed to recover work item", "work_item_id", wi.ID, "status", wi.Status, "error", err)
				continue
			}
			recovered++
			slog.Info("recovery: re-queued work item", "work_item_id", wi.ID, "original_status", wi.Status)
		}
	}

	return recovered, nil
}

// recoverWorkItem resets a single interrupted work item so it can be re-executed.
func recoverWorkItem(ctx context.Context, store Store, scheduler *WorkItemScheduler, workItem *core.WorkItem) error {
	// Reset in-progress actions back to pending.
	actions, err := store.ListActionsByWorkItem(ctx, workItem.ID)
	if err != nil {
		return fmt.Errorf("list actions for work item %d: %w", workItem.ID, err)
	}

	for _, action := range actions {
		switch action.Status {
		case core.ActionRunning, core.ActionWaitingGate:
			// These were mid-execution when the process died. Reset to pending
			// so they will be re-dispatched.
			if err := store.UpdateActionStatus(ctx, action.ID, core.ActionPending); err != nil {
				return fmt.Errorf("reset action %d: %w", action.ID, err)
			}
			slog.Info("recovery: reset action to pending", "action_id", action.ID, "was", action.Status)
		case core.ActionReady:
			// Ready but not started — reset to pending so promotion re-evaluates.
			if err := store.UpdateActionStatus(ctx, action.ID, core.ActionPending); err != nil {
				return fmt.Errorf("reset action %d: %w", action.ID, err)
			}
		}
		// ActionDone, ActionFailed, ActionCancelled, ActionPending, ActionBlocked — keep as-is.
	}

	// Also cancel any running runs (they are stale from the old process).
	for _, action := range actions {
		runs, err := store.ListRunsByAction(ctx, action.ID)
		if err != nil {
			continue
		}
		for _, run := range runs {
			if run.Status == core.RunRunning || run.Status == core.RunCreated {
				run.Status = core.RunFailed
				run.ErrorMessage = "process restarted during execution"
				run.ErrorKind = core.ErrKindTransient
				_ = store.UpdateRun(ctx, run)
			}
		}
	}

	// Reset work item status to open so it can be submitted.
	if err := store.UpdateWorkItemStatus(ctx, workItem.ID, core.WorkItemOpen); err != nil {
		return fmt.Errorf("reset work item %d to open: %w", workItem.ID, err)
	}

	// Submit to scheduler queue.
	return scheduler.Submit(ctx, workItem.ID)
}
