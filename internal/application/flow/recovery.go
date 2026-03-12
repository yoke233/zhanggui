package flow

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yoke233/ai-workflow/internal/core"
)

// RecoverInterruptedIssues scans the store for issues that were running or queued
// when the process last stopped, resets their in-progress steps, and re-submits
// them to the scheduler for execution.
//
// Call this once during bootstrap, after the scheduler has been started.
func RecoverInterruptedIssues(ctx context.Context, store Store, scheduler *IssueScheduler) (int, error) {
	return recoverIssuesByStatus(ctx, store, scheduler, []core.IssueStatus{core.IssueQueued, core.IssueRunning})
}

// RecoverQueuedIssues re-enqueues issues that were queued before the process stopped.
func RecoverQueuedIssues(ctx context.Context, store Store, scheduler *IssueScheduler) (int, error) {
	return recoverIssuesByStatus(ctx, store, scheduler, []core.IssueStatus{core.IssueQueued})
}

// RecoverInterruptedFlows is an alias for backward compatibility.
func RecoverInterruptedFlows(ctx context.Context, store Store, scheduler *IssueScheduler) (int, error) {
	return RecoverInterruptedIssues(ctx, store, scheduler)
}

// RecoverQueuedFlows is an alias for backward compatibility.
func RecoverQueuedFlows(ctx context.Context, store Store, scheduler *IssueScheduler) (int, error) {
	return RecoverQueuedIssues(ctx, store, scheduler)
}

func recoverIssuesByStatus(ctx context.Context, store Store, scheduler *IssueScheduler, statuses []core.IssueStatus) (int, error) {
	recovered := 0

	for _, status := range statuses {
		issues, err := store.ListIssues(ctx, core.IssueFilter{Status: &status, Limit: 1000})
		if err != nil {
			return recovered, fmt.Errorf("list %s issues: %w", status, err)
		}

		for _, issue := range issues {
			if err := recoverIssue(ctx, store, scheduler, issue); err != nil {
				slog.Error("recovery: failed to recover issue", "issue_id", issue.ID, "status", issue.Status, "error", err)
				continue
			}
			recovered++
			slog.Info("recovery: re-queued issue", "issue_id", issue.ID, "original_status", issue.Status)
		}
	}

	return recovered, nil
}

// recoverIssue resets a single interrupted issue so it can be re-executed.
func recoverIssue(ctx context.Context, store Store, scheduler *IssueScheduler, issue *core.Issue) error {
	// Reset in-progress steps back to pending.
	steps, err := store.ListStepsByIssue(ctx, issue.ID)
	if err != nil {
		return fmt.Errorf("list steps for issue %d: %w", issue.ID, err)
	}

	for _, step := range steps {
		switch step.Status {
		case core.StepRunning, core.StepWaitingGate:
			// These were mid-execution when the process died. Reset to pending
			// so they will be re-dispatched.
			if err := store.UpdateStepStatus(ctx, step.ID, core.StepPending); err != nil {
				return fmt.Errorf("reset step %d: %w", step.ID, err)
			}
			slog.Info("recovery: reset step to pending", "step_id", step.ID, "was", step.Status)
		case core.StepReady:
			// Ready but not started — reset to pending so promotion re-evaluates.
			if err := store.UpdateStepStatus(ctx, step.ID, core.StepPending); err != nil {
				return fmt.Errorf("reset step %d: %w", step.ID, err)
			}
		}
		// StepDone, StepFailed, StepCancelled, StepPending, StepBlocked — keep as-is.
	}

	// Also cancel any running executions (they are stale from the old process).
	for _, step := range steps {
		execs, err := store.ListExecutionsByStep(ctx, step.ID)
		if err != nil {
			continue
		}
		for _, exec := range execs {
			if exec.Status == core.ExecRunning || exec.Status == core.ExecCreated {
				exec.Status = core.ExecFailed
				exec.ErrorMessage = "process restarted during execution"
				exec.ErrorKind = core.ErrKindTransient
				_ = store.UpdateExecution(ctx, exec)
			}
		}
	}

	// Reset issue status to open so it can be submitted.
	if err := store.UpdateIssueStatus(ctx, issue.ID, core.IssueOpen); err != nil {
		return fmt.Errorf("reset issue %d to open: %w", issue.ID, err)
	}

	// Submit to scheduler queue.
	return scheduler.Submit(ctx, issue.ID)
}
