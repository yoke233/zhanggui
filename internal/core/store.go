package core

import (
	"context"
	"time"
)

// ProjectStore persists Project aggregates.
type ProjectStore interface {
	CreateProject(ctx context.Context, p *Project) (int64, error)
	GetProject(ctx context.Context, id int64) (*Project, error)
	ListProjects(ctx context.Context, limit, offset int) ([]*Project, error)
	UpdateProject(ctx context.Context, p *Project) error
	DeleteProject(ctx context.Context, id int64) error
}

// ResourceBindingStore persists ResourceBinding records.
type ResourceBindingStore interface {
	CreateResourceBinding(ctx context.Context, rb *ResourceBinding) (int64, error)
	GetResourceBinding(ctx context.Context, id int64) (*ResourceBinding, error)
	ListResourceBindings(ctx context.Context, projectID int64) ([]*ResourceBinding, error)
	DeleteResourceBinding(ctx context.Context, id int64) error
}

// ActionStore persists Action aggregates.
type ActionStore interface {
	CreateAction(ctx context.Context, a *Action) (int64, error)
	GetAction(ctx context.Context, id int64) (*Action, error)
	ListActionsByWorkItem(ctx context.Context, workItemID int64) ([]*Action, error)
	UpdateActionStatus(ctx context.Context, id int64, status ActionStatus) error
	UpdateAction(ctx context.Context, a *Action) error
	DeleteAction(ctx context.Context, id int64) error
	BatchCreateActions(ctx context.Context, actions []*Action) error
	UpdateActionDependsOn(ctx context.Context, id int64, dependsOn []int64) error
}

// RunStore persists Run aggregates.
type RunStore interface {
	CreateRun(ctx context.Context, r *Run) (int64, error)
	GetRun(ctx context.Context, id int64) (*Run, error)
	ListRunsByAction(ctx context.Context, actionID int64) ([]*Run, error)
	ListRunsByStatus(ctx context.Context, status RunStatus) ([]*Run, error)
	UpdateRun(ctx context.Context, r *Run) error
}

// DeliverableStore persists Deliverable records.
type DeliverableStore interface {
	CreateDeliverable(ctx context.Context, d *Deliverable) (int64, error)
	GetDeliverable(ctx context.Context, id int64) (*Deliverable, error)
	GetLatestDeliverableByAction(ctx context.Context, actionID int64) (*Deliverable, error)
	ListDeliverablesByRun(ctx context.Context, runID int64) ([]*Deliverable, error)
	UpdateDeliverable(ctx context.Context, d *Deliverable) error
}

// AgentContextStore persists AgentContext records.
type AgentContextStore interface {
	CreateAgentContext(ctx context.Context, ac *AgentContext) (int64, error)
	GetAgentContext(ctx context.Context, id int64) (*AgentContext, error)
	FindAgentContext(ctx context.Context, agentID string, workItemID int64) (*AgentContext, error)
	UpdateAgentContext(ctx context.Context, ac *AgentContext) error
}

// EventStore persists domain events.
type EventStore interface {
	CreateEvent(ctx context.Context, e *Event) (int64, error)
	ListEvents(ctx context.Context, filter EventFilter) ([]*Event, error)
	GetLatestRunEventTime(ctx context.Context, runID int64, eventType EventType) (*time.Time, error)
}

// DAGTemplateStore persists DAGTemplate records.
type DAGTemplateStore interface {
	CreateDAGTemplate(ctx context.Context, t *DAGTemplate) (int64, error)
	GetDAGTemplate(ctx context.Context, id int64) (*DAGTemplate, error)
	ListDAGTemplates(ctx context.Context, filter DAGTemplateFilter) ([]*DAGTemplate, error)
	UpdateDAGTemplate(ctx context.Context, t *DAGTemplate) error
	DeleteDAGTemplate(ctx context.Context, id int64) error
}

// RunProbeStore persists probe records and run routing metadata.
type RunProbeStore interface {
	CreateRunProbe(ctx context.Context, probe *RunProbe) (int64, error)
	GetRunProbe(ctx context.Context, id int64) (*RunProbe, error)
	ListRunProbesByRun(ctx context.Context, runID int64) ([]*RunProbe, error)
	GetLatestRunProbe(ctx context.Context, runID int64) (*RunProbe, error)
	GetActiveRunProbe(ctx context.Context, runID int64) (*RunProbe, error)
	UpdateRunProbe(ctx context.Context, probe *RunProbe) error
	GetRunProbeRoute(ctx context.Context, runID int64) (*RunProbeRoute, error)
}

// Store is the aggregate interface combining all sub-stores.
type Store interface {
	ProjectStore
	ResourceBindingStore
	ResourceLocatorStore
	ActionResourceStore
	WorkItemStore
	ThreadStore
	ActionStore
	RunStore
	DeliverableStore
	AgentContextStore
	EventStore
	RunProbeStore
	AnalyticsStore
	DAGTemplateStore
	UsageStore
	ToolCallAuditStore
	FeatureManifestStore
	ActionSignalStore
	WorkItemAttachmentStore
	NotificationStore
	Close() error
}

// TransactionalStore allows callers to execute a multi-store workflow in a
// single transaction when the backing implementation supports it.
type TransactionalStore interface {
	InTx(ctx context.Context, fn func(store Store) error) error
}

// EventFilter constrains Event queries.
type EventFilter struct {
	WorkItemID *int64
	ActionID   *int64
	RunID      *int64
	ThreadID   *int64
	SessionID  string
	Types      []EventType
	Limit      int
	Offset     int
}
