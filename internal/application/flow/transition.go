package flow

import "github.com/yoke233/ai-workflow/internal/core"

// validWorkItemTransitions defines legal WorkItem status transitions.
var validWorkItemTransitions = map[core.WorkItemStatus][]core.WorkItemStatus{
	core.WorkItemOpen:     {core.WorkItemAccepted, core.WorkItemQueued, core.WorkItemRunning, core.WorkItemCancelled},
	core.WorkItemAccepted: {core.WorkItemQueued, core.WorkItemRunning, core.WorkItemCancelled},
	core.WorkItemQueued:   {core.WorkItemRunning, core.WorkItemCancelled},
	core.WorkItemRunning:  {core.WorkItemBlocked, core.WorkItemFailed, core.WorkItemDone, core.WorkItemCancelled},
	core.WorkItemBlocked:  {core.WorkItemRunning, core.WorkItemFailed, core.WorkItemCancelled},
}

// validActionTransitions defines legal Action status transitions.
var validActionTransitions = map[core.ActionStatus][]core.ActionStatus{
	core.ActionPending:     {core.ActionReady, core.ActionCancelled},
	core.ActionReady:       {core.ActionRunning, core.ActionCancelled},
	core.ActionRunning:     {core.ActionWaitingGate, core.ActionDone, core.ActionFailed, core.ActionBlocked, core.ActionPending, core.ActionCancelled},
	core.ActionWaitingGate: {core.ActionDone, core.ActionBlocked, core.ActionFailed, core.ActionPending, core.ActionCancelled},
	core.ActionBlocked:     {core.ActionReady, core.ActionPending, core.ActionFailed, core.ActionCancelled},
	core.ActionFailed:      {core.ActionPending, core.ActionCancelled}, // retry → back to pending
	core.ActionDone:        {core.ActionPending},                       // gate reject → upstream retry
}

// validRunTransitions defines legal Run status transitions.
var validRunTransitions = map[core.RunStatus][]core.RunStatus{
	core.RunCreated: {core.RunRunning, core.RunCancelled},
	core.RunRunning: {core.RunSucceeded, core.RunFailed, core.RunCancelled},
}

// ValidWorkItemTransition checks if transitioning from → to is legal.
func ValidWorkItemTransition(from, to core.WorkItemStatus) bool {
	return contains(validWorkItemTransitions[from], to)
}

// ValidActionTransition checks if transitioning from → to is legal.
func ValidActionTransition(from, to core.ActionStatus) bool {
	return contains(validActionTransitions[from], to)
}

// ValidRunTransition checks if transitioning from → to is legal.
func ValidRunTransition(from, to core.RunStatus) bool {
	return contains(validRunTransitions[from], to)
}

func contains[T comparable](slice []T, val T) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}
