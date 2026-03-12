package flow

import "github.com/yoke233/ai-workflow/internal/core"

// validIssueTransitions defines legal Issue status transitions.
var validIssueTransitions = map[core.IssueStatus][]core.IssueStatus{
	core.IssueOpen:     {core.IssueAccepted, core.IssueQueued, core.IssueRunning, core.IssueCancelled},
	core.IssueAccepted: {core.IssueQueued, core.IssueRunning, core.IssueCancelled},
	core.IssueQueued:   {core.IssueRunning, core.IssueCancelled},
	core.IssueRunning:  {core.IssueBlocked, core.IssueFailed, core.IssueDone, core.IssueCancelled},
	core.IssueBlocked:  {core.IssueRunning, core.IssueFailed, core.IssueCancelled},
}

// validStepTransitions defines legal Step status transitions.
var validStepTransitions = map[core.StepStatus][]core.StepStatus{
	core.StepPending:     {core.StepReady, core.StepCancelled},
	core.StepReady:       {core.StepRunning, core.StepCancelled},
	core.StepRunning:     {core.StepWaitingGate, core.StepDone, core.StepFailed, core.StepBlocked, core.StepPending, core.StepCancelled},
	core.StepWaitingGate: {core.StepDone, core.StepBlocked, core.StepFailed, core.StepPending, core.StepCancelled},
	core.StepBlocked:     {core.StepReady, core.StepPending, core.StepFailed, core.StepCancelled},
	core.StepFailed:      {core.StepPending, core.StepCancelled}, // retry → back to pending
	core.StepDone:        {core.StepPending},                     // gate reject → upstream retry
}

// validExecTransitions defines legal Execution status transitions.
var validExecTransitions = map[core.ExecutionStatus][]core.ExecutionStatus{
	core.ExecCreated: {core.ExecRunning, core.ExecCancelled},
	core.ExecRunning: {core.ExecSucceeded, core.ExecFailed, core.ExecCancelled},
}

// ValidIssueTransition checks if transitioning from → to is legal.
func ValidIssueTransition(from, to core.IssueStatus) bool {
	return contains(validIssueTransitions[from], to)
}

// ValidStepTransition checks if transitioning from → to is legal.
func ValidStepTransition(from, to core.StepStatus) bool {
	return contains(validStepTransitions[from], to)
}

// ValidExecTransition checks if transitioning from → to is legal.
func ValidExecTransition(from, to core.ExecutionStatus) bool {
	return contains(validExecTransitions[from], to)
}

func contains[T comparable](slice []T, val T) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}
