package workitemapp

import (
	"context"

	"github.com/yoke233/zhanggui/internal/core"
)

type ProjectReader interface {
	GetProject(ctx context.Context, id int64) (*core.Project, error)
}

type ResourceSpaceReader interface {
	GetResourceSpace(ctx context.Context, id int64) (*core.ResourceSpace, error)
}

type WorkItemReader interface {
	GetWorkItem(ctx context.Context, id int64) (*core.WorkItem, error)
}

type WorkItemWriter interface {
	CreateWorkItem(ctx context.Context, workItem *core.WorkItem) (int64, error)
	UpdateWorkItem(ctx context.Context, workItem *core.WorkItem) error
	PrepareWorkItemRun(ctx context.Context, id int64, queuedStatus core.WorkItemStatus) error
	SetWorkItemArchived(ctx context.Context, id int64, archived bool) error
	DeleteWorkItem(ctx context.Context, id int64) error
	UpdateWorkItemStatus(ctx context.Context, id int64, status core.WorkItemStatus) error
}

type ActionReader interface {
	ListActionsByWorkItem(ctx context.Context, workItemID int64) ([]*core.Action, error)
}

type ActionWriter interface {
	UpdateAction(ctx context.Context, action *core.Action) error
}

type ThreadLinkReader interface {
	ListThreadsByWorkItem(ctx context.Context, workItemID int64) ([]*core.ThreadWorkItemLink, error)
}

type AggregateDeletionStore interface {
	DeleteActionIODeclsByWorkItem(ctx context.Context, workItemID int64) error
	DeleteResourcesByWorkItem(ctx context.Context, workItemID int64) error
	DeleteRunsByWorkItem(ctx context.Context, workItemID int64) error
	DeleteActionSignalsByWorkItem(ctx context.Context, workItemID int64) error
	DeleteAgentContextsByWorkItem(ctx context.Context, workItemID int64) error
	DeleteEventsByWorkItem(ctx context.Context, workItemID int64) error
	DeleteJournalByWorkItem(ctx context.Context, workItemID int64) error
	DeleteThreadWorkItemLinksByWorkItem(ctx context.Context, workItemID int64) error
	DeleteActionsByWorkItem(ctx context.Context, workItemID int64) error
	DetachFeatureEntriesByWorkItem(ctx context.Context, workItemID int64) error
}

type Store interface {
	ProjectReader
	ResourceSpaceReader
	WorkItemReader
	WorkItemWriter
	ActionReader
	ActionWriter
	ThreadLinkReader
	AggregateDeletionStore
	core.DeliverableStore
}

type TxStore interface {
	Store
}

type Tx interface {
	InTx(ctx context.Context, fn func(ctx context.Context, store TxStore) error) error
}

type Scheduler interface {
	Submit(ctx context.Context, workItemID int64) error
	Cancel(ctx context.Context, workItemID int64) error
}

type Runner interface {
	Run(ctx context.Context, workItemID int64) error
	Cancel(ctx context.Context, workItemID int64) error
}

type EventPublisher interface {
	Publish(ctx context.Context, event core.Event)
}

type Bootstrapper interface {
	BootstrapPRWorkItem(ctx context.Context, workItemID int64) error
}

type CreateWorkItemInput struct {
	ProjectID          *int64
	ResourceSpaceID    *int64
	Title              string
	Body               string
	Priority           string
	ExecutorProfileID  string
	ReviewerProfileID  string
	ActiveProfileID    string
	SponsorProfileID   string
	CreatedByProfileID string
	ParentWorkItemID   *int64
	RootWorkItemID     *int64
	FinalDeliverableID *int64
	Labels             []string
	DependsOn          []int64
	EscalationPath     []string
	Metadata           map[string]any
}

type UpdateWorkItemInput struct {
	ID                 int64
	ProjectID          *int64
	ResourceSpaceID    *int64
	ParentWorkItemID   *int64
	RootWorkItemID     *int64
	FinalDeliverableID *int64
	Title              *string
	Body               *string
	Status             *string
	Priority           *string
	ExecutorProfileID  *string
	ReviewerProfileID  *string
	ActiveProfileID    *string
	SponsorProfileID   *string
	CreatedByProfileID *string
	Labels             *[]string
	DependsOn          *[]int64
	EscalationPath     *[]string
	Metadata           map[string]any
}

type RunWorkItemResult struct {
	Queued  bool
	Message string
}
