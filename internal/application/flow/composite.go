package flow

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ExpandComposite creates a child WorkItem for a composite Action and links them.
// The caller provides the child actions to populate the child WorkItem.
func (e *WorkItemEngine) ExpandComposite(ctx context.Context, action *core.Action, childActions []*core.Action) (int64, error) {
	if action.Type != core.ActionPlan {
		return 0, fmt.Errorf("action %d is not composite (type=%s)", action.ID, action.Type)
	}

	childWorkItem := &core.WorkItem{
		Title:  fmt.Sprintf("%s/sub", action.Name),
		Status: core.WorkItemOpen,
	}
	// Inherit ProjectID from parent work item.
	parentWorkItem, err := e.store.GetWorkItem(ctx, action.WorkItemID)
	if err == nil && parentWorkItem.ProjectID != nil {
		childWorkItem.ProjectID = parentWorkItem.ProjectID
	}
	childWorkItemID, err := e.store.CreateWorkItem(ctx, childWorkItem)
	if err != nil {
		return 0, fmt.Errorf("create child work item: %w", err)
	}

	for i, ca := range childActions {
		ca.WorkItemID = childWorkItemID
		ca.Status = core.ActionPending
		ca.Position = i + 1
		if _, err := e.store.CreateAction(ctx, ca); err != nil {
			return 0, fmt.Errorf("create child action %s: %w", ca.Name, err)
		}
	}

	// Store the child work item ID in the action's Config for tracking.
	if action.Config == nil {
		action.Config = map[string]any{}
	}
	action.Config["child_work_item_id"] = childWorkItemID
	if err := e.store.UpdateAction(ctx, action); err != nil {
		return 0, fmt.Errorf("persist child work item link for action %d: %w", action.ID, err)
	}

	return childWorkItemID, nil
}

// childWorkItemID retrieves the child work item ID from a composite action's config.
func childWorkItemID(action *core.Action) *int64 {
	if action.Config == nil {
		return nil
	}
	v, ok := action.Config["child_work_item_id"]
	if !ok {
		return nil
	}
	if id, ok := toInt64(v); ok {
		return &id
	}
	return nil
}

// executeComposite handles composite action execution:
// expand child actions → create child work item → run child work item → propagate result.
func (e *WorkItemEngine) executeComposite(ctx context.Context, action *core.Action) error {
	// If child work item exists but is in a terminal state (e.g. after gate reject reset),
	// clear it so we create a fresh one.
	cID := childWorkItemID(action)
	if cID != nil {
		ci, err := e.store.GetWorkItem(ctx, *cID)
		if err == nil && (ci.Status == core.WorkItemDone || ci.Status == core.WorkItemFailed || ci.Status == core.WorkItemCancelled) {
			cID = nil
			delete(action.Config, "child_work_item_id")
		}
	}

	// If no child work item exists, expand.
	if cID == nil {
		if e.expander == nil {
			_ = e.transitionAction(ctx, action, core.ActionFailed)
			return fmt.Errorf("composite action %d: no expander configured and no child work item", action.ID)
		}

		children, err := e.expander.Expand(ctx, action)
		if err != nil {
			_ = e.transitionAction(ctx, action, core.ActionFailed)
			return fmt.Errorf("expand composite action %d: %w", action.ID, err)
		}

		newID, err := e.ExpandComposite(ctx, action, children)
		if err != nil {
			_ = e.transitionAction(ctx, action, core.ActionFailed)
			return fmt.Errorf("create child work item for action %d: %w", action.ID, err)
		}
		cID = &newID
	}

	childWIID := *cID

	e.bus.Publish(ctx, core.Event{
		Type:       core.EventWorkItemStarted,
		WorkItemID: childWIID,
		ActionID:   action.ID,
		Timestamp:  time.Now().UTC(),
		Data:       map[string]any{"parent_work_item_id": action.WorkItemID},
	})

	// Run child work item. This blocks until the child work item completes.
	if err := e.Run(ctx, childWIID); err != nil {
		// Child work item failed — check retry budget on the composite action.
		if action.RetryCount < action.MaxRetries {
			action.RetryCount++
			action.Status = core.ActionPending
			delete(action.Config, "child_work_item_id") // clear link so next attempt creates fresh child work item
			return e.store.UpdateAction(ctx, action)
		}

		_ = e.transitionAction(ctx, action, core.ActionFailed)
		return fmt.Errorf("composite action %d child work item failed: %w", action.ID, err)
	}

	// Child work item succeeded → parent action done.
	return e.transitionAction(ctx, action, core.ActionDone)
}
