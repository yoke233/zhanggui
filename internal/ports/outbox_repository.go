package ports

import (
	"context"
	"errors"
)

var ErrIssueNotFound = errors.New("outbox issue not found")

type OutboxIssueFilter struct {
	IncludeClosed bool
	Assignee      string
	IncludeLabels []string
	ExcludeLabels []string
}

type OutboxIssue struct {
	IssueID   uint64
	Title     string
	Body      string
	Assignee  *string
	IsClosed  bool
	CreatedAt string
	UpdatedAt string
	ClosedAt  *string
}

type OutboxEvent struct {
	EventID   uint64
	IssueID   uint64
	Actor     string
	Body      string
	CreatedAt string
}

type OutboxEventCreate struct {
	IssueID   uint64
	Actor     string
	Body      string
	CreatedAt string
}

type OutboxQualityEvent struct {
	QualityEventID  uint64
	IssueID         uint64
	IdempotencyKey  string
	Source          string
	ExternalEventID string
	Category        string
	Result          string
	Actor           string
	Summary         string
	EvidenceJSON    string
	PayloadJSON     string
	IngestedAt      string
}

type OutboxQualityEventCreate struct {
	IssueID         uint64
	IdempotencyKey  string
	Source          string
	ExternalEventID string
	Category        string
	Result          string
	Actor           string
	Summary         string
	EvidenceJSON    string
	PayloadJSON     string
	IngestedAt      string
}

type OutboxReadRepository interface {
	ListIssues(ctx context.Context, filter OutboxIssueFilter) ([]OutboxIssue, error)
	GetIssue(ctx context.Context, issueID uint64) (OutboxIssue, error)
	ListIssueLabels(ctx context.Context, issueID uint64) ([]string, error)
	ListIssueEvents(ctx context.Context, issueID uint64) ([]OutboxEvent, error)
	ListEventsAfter(ctx context.Context, afterEventID uint64, limit int) ([]OutboxEvent, error)
	ListQualityEvents(ctx context.Context, issueID uint64, limit int) ([]OutboxQualityEvent, error)
}

type OutboxRepository interface {
	OutboxReadRepository
	CreateIssue(ctx context.Context, issue OutboxIssue, labels []string) (OutboxIssue, error)
	SetIssueAssignee(ctx context.Context, issueID uint64, assignee string, updatedAt string) error
	UpdateIssueUpdatedAt(ctx context.Context, issueID uint64, updatedAt string) error
	MarkIssueClosed(ctx context.Context, issueID uint64, closedAt string) error
	ReplaceStateLabel(ctx context.Context, issueID uint64, stateLabel string) error
	AddIssueLabel(ctx context.Context, issueID uint64, label string) error
	RemoveIssueLabel(ctx context.Context, issueID uint64, label string) error
	HasIssueLabel(ctx context.Context, issueID uint64, label string) (bool, error)
	AppendEvent(ctx context.Context, input OutboxEventCreate) error
	CreateQualityEvent(ctx context.Context, input OutboxQualityEventCreate) (bool, error)
}
