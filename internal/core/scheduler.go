package core

import "context"

// Scheduler coordinates pipeline queueing and lifecycle control.
type Scheduler interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Enqueue(pipelineID string) error
}
