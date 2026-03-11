package core

import "time"

// FlowStatus represents the lifecycle state of a Flow.
type FlowStatus string

const (
	FlowPending   FlowStatus = "pending"
	FlowQueued    FlowStatus = "queued"
	FlowRunning   FlowStatus = "running"
	FlowBlocked   FlowStatus = "blocked"
	FlowFailed    FlowStatus = "failed"
	FlowDone      FlowStatus = "done"
	FlowCancelled FlowStatus = "cancelled"
)

// Flow is the top-level orchestration unit. It contains a DAG of Steps.
// Entry steps are those whose DependsOn is empty — derived by the engine, not stored.
type Flow struct {
	ID           int64             `json:"id"`
	ProjectID    *int64            `json:"project_id,omitempty"`
	Name         string            `json:"name"`
	Status       FlowStatus        `json:"status"`
	ParentStepID *int64            `json:"parent_step_id,omitempty"` // sub-Flow points to parent composite Step
	Metadata     map[string]string `json:"metadata,omitempty"`
	ArchivedAt   *time.Time        `json:"archived_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}
