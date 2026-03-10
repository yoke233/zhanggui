package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// ExpandComposite creates a sub-Flow for a composite Step and links them.
// The caller provides the child steps to populate the sub-Flow.
func (e *FlowEngine) ExpandComposite(ctx context.Context, step *core.Step, childSteps []*core.Step) (int64, error) {
	if step.Type != core.StepComposite {
		return 0, fmt.Errorf("step %d is not composite (type=%s)", step.ID, step.Type)
	}

	subFlow := &core.Flow{
		Name:         fmt.Sprintf("%s/sub", step.Name),
		Status:       core.FlowPending,
		ParentStepID: &step.ID,
	}
	// Inherit ProjectID from parent flow.
	parentFlow, err := e.store.GetFlow(ctx, step.FlowID)
	if err == nil && parentFlow.ProjectID != nil {
		subFlow.ProjectID = parentFlow.ProjectID
	}
	subFlowID, err := e.store.CreateFlow(ctx, subFlow)
	if err != nil {
		return 0, fmt.Errorf("create sub-flow: %w", err)
	}

	for _, cs := range childSteps {
		cs.FlowID = subFlowID
		cs.Status = core.StepPending
		if _, err := e.store.CreateStep(ctx, cs); err != nil {
			return 0, fmt.Errorf("create child step %s: %w", cs.Name, err)
		}
	}

	// Link parent step to sub-flow and persist.
	step.SubFlowID = &subFlowID
	if err := e.store.UpdateStep(ctx, step); err != nil {
		return 0, fmt.Errorf("persist sub-flow link for step %d: %w", step.ID, err)
	}

	return subFlowID, nil
}

// executeComposite handles composite step execution:
// expand child steps → create sub-flow → run sub-flow → propagate result.
func (e *FlowEngine) executeComposite(ctx context.Context, step *core.Step) error {
	// If sub-flow exists but is in a terminal state (e.g. after gate reject reset),
	// clear it so we create a fresh one.
	if step.SubFlowID != nil {
		sf, err := e.store.GetFlow(ctx, *step.SubFlowID)
		if err == nil && (sf.Status == core.FlowDone || sf.Status == core.FlowFailed || sf.Status == core.FlowCancelled) {
			step.SubFlowID = nil
		}
	}

	// If no sub-flow exists, expand.
	if step.SubFlowID == nil {
		if e.expander == nil {
			_ = e.transitionStep(ctx, step, core.StepFailed)
			return fmt.Errorf("composite step %d: no expander configured and no sub-flow", step.ID)
		}

		children, err := e.expander.Expand(ctx, step)
		if err != nil {
			_ = e.transitionStep(ctx, step, core.StepFailed)
			return fmt.Errorf("expand composite step %d: %w", step.ID, err)
		}

		if _, err := e.ExpandComposite(ctx, step, children); err != nil {
			_ = e.transitionStep(ctx, step, core.StepFailed)
			return fmt.Errorf("create sub-flow for step %d: %w", step.ID, err)
		}
	}

	subFlowID := *step.SubFlowID

	e.bus.Publish(ctx, core.Event{
		Type:      core.EventFlowStarted,
		FlowID:    subFlowID,
		StepID:    step.ID,
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"parent_flow_id": step.FlowID},
	})

	// Run sub-flow. This blocks until the sub-flow completes.
	if err := e.Run(ctx, subFlowID); err != nil {
		// Sub-flow failed — check retry budget on the composite step.
		if step.RetryCount < step.MaxRetries {
			step.RetryCount++
			step.Status = core.StepPending
			step.SubFlowID = nil // clear link so next attempt creates fresh sub-flow
			return e.store.UpdateStep(ctx, step)
		}

		_ = e.transitionStep(ctx, step, core.StepFailed)
		return fmt.Errorf("composite step %d sub-flow failed: %w", step.ID, err)
	}

	// Sub-flow succeeded → parent step done.
	return e.transitionStep(ctx, step, core.StepDone)
}
