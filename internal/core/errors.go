package core

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrInvalidTransition  = errors.New("invalid state transition")
	ErrCycleDetected      = errors.New("cycle detected in action DAG")
	ErrWorkItemNotRunnable = errors.New("work item is not runnable")
	ErrActionNotReady     = errors.New("action is not ready")
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
	ErrGateRejected       = errors.New("gate rejected")
	ErrAgentActionDenied  = errors.New("agent action not permitted")
	ErrNoMatchingAgent    = errors.New("no agent matches action requirements")
	ErrMissingResult    = errors.New("run completed without result")
	ErrDuplicateEntryKey = errors.New("duplicate feature entry key in project")
	ErrTokenBudgetExceeded  = errors.New("token budget exceeded")
)
