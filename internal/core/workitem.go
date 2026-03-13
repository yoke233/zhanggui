package core

import (
	"context"
	"time"
)

// WorkItemStatus represents the unified lifecycle state of a WorkItem.
// It covers both planning (open/accepted) and execution (queued/running/done/failed).
type WorkItemStatus string

const (
	WorkItemOpen      WorkItemStatus = "open"
	WorkItemAccepted  WorkItemStatus = "accepted"
	WorkItemQueued    WorkItemStatus = "queued"
	WorkItemRunning   WorkItemStatus = "running"
	WorkItemBlocked   WorkItemStatus = "blocked"
	WorkItemFailed    WorkItemStatus = "failed"
	WorkItemDone      WorkItemStatus = "done"
	WorkItemCancelled WorkItemStatus = "cancelled"
	WorkItemClosed    WorkItemStatus = "closed"
)

// WorkItemPriority represents the urgency of a WorkItem.
type WorkItemPriority string

const (
	PriorityLow    WorkItemPriority = "low"
	PriorityMedium WorkItemPriority = "medium"
	PriorityHigh   WorkItemPriority = "high"
	PriorityUrgent WorkItemPriority = "urgent"
)

// WorkItem is the unified work unit: it combines the planning intent (title, body,
// priority, labels) with the execution context (status lifecycle, actions, workspace).
//
// A WorkItem optionally belongs to a Project and can be bound to a specific
// ResourceBinding (repo) for workspace isolation.
type WorkItem struct {
	ID                int64  `json:"id"`
	ProjectID         *int64 `json:"project_id,omitempty"`
	ResourceBindingID *int64 `json:"resource_binding_id,omitempty"` // which repo/resource to work on

	// Planning fields
	Title    string           `json:"title"`
	Body     string           `json:"body"`
	Priority WorkItemPriority `json:"priority"`
	Labels   []string         `json:"labels,omitempty"`

	// Execution fields
	Status   WorkItemStatus `json:"status"`
	Metadata map[string]any `json:"metadata,omitempty"`

	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// WorkItemFilter constrains WorkItem queries.
type WorkItemFilter struct {
	ProjectID *int64
	Status    *WorkItemStatus
	Priority  *WorkItemPriority
	Archived  *bool
	Limit     int
	Offset    int
}

// WorkItemStore persists WorkItem aggregates.
type WorkItemStore interface {
	CreateWorkItem(ctx context.Context, w *WorkItem) (int64, error)
	GetWorkItem(ctx context.Context, id int64) (*WorkItem, error)
	ListWorkItems(ctx context.Context, filter WorkItemFilter) ([]*WorkItem, error)
	UpdateWorkItem(ctx context.Context, w *WorkItem) error
	UpdateWorkItemStatus(ctx context.Context, id int64, status WorkItemStatus) error
	UpdateWorkItemMetadata(ctx context.Context, id int64, metadata map[string]any) error
	PrepareWorkItemRun(ctx context.Context, id int64, queuedStatus WorkItemStatus) error
	SetWorkItemArchived(ctx context.Context, id int64, archived bool) error
	DeleteWorkItem(ctx context.Context, id int64) error
}
