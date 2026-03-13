package flow

import "context"

// Runner is the minimal application contract required to execute and cancel work items.
type Runner interface {
	Run(ctx context.Context, workItemID int64) error
	Cancel(ctx context.Context, workItemID int64) error
}

// Scheduler is the minimal application contract required by transport adapters.
type Scheduler interface {
	Submit(ctx context.Context, workItemID int64) error
	Cancel(ctx context.Context, workItemID int64) error
	Stats() SchedulerStats
}
