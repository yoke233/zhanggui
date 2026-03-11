package core

import (
	"context"
	"time"
)

// IssueStatus represents the lifecycle state of an Issue.
type IssueStatus string

const (
	IssueOpen       IssueStatus = "open"
	IssueAccepted   IssueStatus = "accepted"
	IssueInProgress IssueStatus = "in_progress"
	IssueDone       IssueStatus = "done"
	IssueClosed     IssueStatus = "closed"
)

// IssuePriority represents the urgency of an Issue.
type IssuePriority string

const (
	PriorityLow    IssuePriority = "low"
	PriorityMedium IssuePriority = "medium"
	PriorityHigh   IssuePriority = "high"
	PriorityUrgent IssuePriority = "urgent"
)

// Issue is a requirement or task that can optionally belong to a Project
// and be converted into a Flow for execution.
type Issue struct {
	ID        int64             `json:"id"`
	ProjectID *int64            `json:"project_id,omitempty"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Status    IssueStatus       `json:"status"`
	Priority  IssuePriority     `json:"priority"`
	Labels    []string          `json:"labels,omitempty"`
	FlowID    *int64            `json:"flow_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// IssueFilter constrains Issue queries.
type IssueFilter struct {
	ProjectID *int64
	Status    *IssueStatus
	Priority  *IssuePriority
	Limit     int
	Offset    int
}

// IssueStore persists Issue aggregates.
type IssueStore interface {
	CreateIssue(ctx context.Context, issue *Issue) (int64, error)
	GetIssue(ctx context.Context, id int64) (*Issue, error)
	ListIssues(ctx context.Context, filter IssueFilter) ([]*Issue, error)
	UpdateIssue(ctx context.Context, issue *Issue) error
	UpdateIssueStatus(ctx context.Context, id int64, status IssueStatus) error
	DeleteIssue(ctx context.Context, id int64) error
}
