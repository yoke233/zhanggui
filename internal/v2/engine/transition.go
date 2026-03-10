package engine

import "github.com/yoke233/ai-workflow/internal/v2/core"

// validFlowTransitions defines legal Flow status transitions.
var validFlowTransitions = map[core.FlowStatus][]core.FlowStatus{
	core.FlowPending: {core.FlowQueued, core.FlowRunning, core.FlowCancelled},
	core.FlowQueued:  {core.FlowRunning, core.FlowCancelled},
	core.FlowRunning: {core.FlowBlocked, core.FlowFailed, core.FlowDone, core.FlowCancelled},
	core.FlowBlocked: {core.FlowRunning, core.FlowFailed, core.FlowCancelled},
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

// ValidFlowTransition checks if transitioning from → to is legal.
func ValidFlowTransition(from, to core.FlowStatus) bool {
	return contains(validFlowTransitions[from], to)
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
