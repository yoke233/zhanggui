package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// RecoverInterruptedFlows scans the store for flows that were running or queued
// when the process last stopped, resets their in-progress steps, and re-submits
// them to the scheduler for execution.
//
// Call this once during bootstrap, after the scheduler has been started.
func RecoverInterruptedFlows(ctx context.Context, store core.Store, scheduler *FlowScheduler) (int, error) {
	recovered := 0

	for _, status := range []core.FlowStatus{core.FlowQueued, core.FlowRunning} {
		flows, err := store.ListFlows(ctx, core.FlowFilter{Status: &status, Limit: 1000})
		if err != nil {
			return recovered, fmt.Errorf("list %s flows: %w", status, err)
		}

		for _, flow := range flows {
			if err := recoverFlow(ctx, store, scheduler, flow); err != nil {
				slog.Error("recovery: failed to recover flow", "flow_id", flow.ID, "status", flow.Status, "error", err)
				continue
			}
			recovered++
			slog.Info("recovery: re-queued flow", "flow_id", flow.ID, "original_status", flow.Status)
		}
	}

	return recovered, nil
}

// recoverFlow resets a single interrupted flow so it can be re-executed.
func recoverFlow(ctx context.Context, store core.Store, scheduler *FlowScheduler, flow *core.Flow) error {
	// Reset in-progress steps back to pending.
	steps, err := store.ListStepsByFlow(ctx, flow.ID)
	if err != nil {
		return fmt.Errorf("list steps for flow %d: %w", flow.ID, err)
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
			// Ready but not started — reset to pending so DAG promotion re-evaluates.
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

	// Reset flow status to pending so it can be submitted.
	if err := store.UpdateFlowStatus(ctx, flow.ID, core.FlowPending); err != nil {
		return fmt.Errorf("reset flow %d to pending: %w", flow.ID, err)
	}

	// Submit to scheduler queue.
	return scheduler.Submit(ctx, flow.ID)
}
