package core

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrInvalidTransition  = errors.New("invalid state transition")
	ErrCycleDetected      = errors.New("cycle detected in step DAG")
	ErrFlowNotRunnable    = errors.New("flow is not runnable")
	ErrStepNotReady       = errors.New("step is not ready")
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
	ErrGateRejected       = errors.New("gate rejected")
	ErrActionDenied       = errors.New("action not permitted")
	ErrNoMatchingAgent    = errors.New("no agent matches step requirements")
	ErrMissingArtifact    = errors.New("execution completed without artifact")
)
