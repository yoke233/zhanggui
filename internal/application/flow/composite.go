package flow

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ExpandComposite creates a child Issue for a composite Step and links them.
// The caller provides the child steps to populate the child Issue.
func (e *IssueEngine) ExpandComposite(ctx context.Context, step *core.Step, childSteps []*core.Step) (int64, error) {
	if step.Type != core.StepComposite {
		return 0, fmt.Errorf("step %d is not composite (type=%s)", step.ID, step.Type)
	}

	childIssue := &core.Issue{
		Title:  fmt.Sprintf("%s/sub", step.Name),
		Status: core.IssueOpen,
	}
	// Inherit ProjectID from parent issue.
	parentIssue, err := e.store.GetIssue(ctx, step.IssueID)
	if err == nil && parentIssue.ProjectID != nil {
		childIssue.ProjectID = parentIssue.ProjectID
	}
	childIssueID, err := e.store.CreateIssue(ctx, childIssue)
	if err != nil {
		return 0, fmt.Errorf("create child issue: %w", err)
	}

	for i, cs := range childSteps {
		cs.IssueID = childIssueID
		cs.Status = core.StepPending
		cs.Position = i + 1
		if _, err := e.store.CreateStep(ctx, cs); err != nil {
			return 0, fmt.Errorf("create child step %s: %w", cs.Name, err)
		}
	}

	// Store the child issue ID in the step's Config for tracking.
	if step.Config == nil {
		step.Config = map[string]any{}
	}
	step.Config["child_issue_id"] = childIssueID
	if err := e.store.UpdateStep(ctx, step); err != nil {
		return 0, fmt.Errorf("persist child issue link for step %d: %w", step.ID, err)
	}

	return childIssueID, nil
}

// childIssueID retrieves the child issue ID from a composite step's config.
func childIssueID(step *core.Step) *int64 {
	if step.Config == nil {
		return nil
	}
	v, ok := step.Config["child_issue_id"]
	if !ok {
		return nil
	}
	if id, ok := toInt64(v); ok {
		return &id
	}
	return nil
}

// executeComposite handles composite step execution:
// expand child steps → create child issue → run child issue → propagate result.
func (e *IssueEngine) executeComposite(ctx context.Context, step *core.Step) error {
	// If child issue exists but is in a terminal state (e.g. after gate reject reset),
	// clear it so we create a fresh one.
	cID := childIssueID(step)
	if cID != nil {
		ci, err := e.store.GetIssue(ctx, *cID)
		if err == nil && (ci.Status == core.IssueDone || ci.Status == core.IssueFailed || ci.Status == core.IssueCancelled) {
			cID = nil
			delete(step.Config, "child_issue_id")
		}
	}

	// If no child issue exists, expand.
	if cID == nil {
		if e.expander == nil {
			_ = e.transitionStep(ctx, step, core.StepFailed)
			return fmt.Errorf("composite step %d: no expander configured and no child issue", step.ID)
		}

		children, err := e.expander.Expand(ctx, step)
		if err != nil {
			_ = e.transitionStep(ctx, step, core.StepFailed)
			return fmt.Errorf("expand composite step %d: %w", step.ID, err)
		}

		newID, err := e.ExpandComposite(ctx, step, children)
		if err != nil {
			_ = e.transitionStep(ctx, step, core.StepFailed)
			return fmt.Errorf("create child issue for step %d: %w", step.ID, err)
		}
		cID = &newID
	}

	childIssID := *cID

	e.bus.Publish(ctx, core.Event{
		Type:      core.EventIssueStarted,
		IssueID:   childIssID,
		StepID:    step.ID,
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"parent_issue_id": step.IssueID},
	})

	// Run child issue. This blocks until the child issue completes.
	if err := e.Run(ctx, childIssID); err != nil {
		// Child issue failed — check retry budget on the composite step.
		if step.RetryCount < step.MaxRetries {
			step.RetryCount++
			step.Status = core.StepPending
			delete(step.Config, "child_issue_id") // clear link so next attempt creates fresh child issue
			return e.store.UpdateStep(ctx, step)
		}

		_ = e.transitionStep(ctx, step, core.StepFailed)
		return fmt.Errorf("composite step %d child issue failed: %w", step.ID, err)
	}

	// Child issue succeeded → parent step done.
	return e.transitionStep(ctx, step, core.StepDone)
}
