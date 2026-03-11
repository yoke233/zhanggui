package core

import (
	"context"
	"time"
)

// FlowStore persists Flow aggregates.
type FlowStore interface {
	CreateFlow(ctx context.Context, f *Flow) (int64, error)
	GetFlow(ctx context.Context, id int64) (*Flow, error)
	ListFlows(ctx context.Context, filter FlowFilter) ([]*Flow, error)
	UpdateFlowStatus(ctx context.Context, id int64, status FlowStatus) error
	PrepareFlowRun(ctx context.Context, id int64, queuedStatus FlowStatus) error
	SetFlowArchived(ctx context.Context, id int64, archived bool) error
}

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

// StepStore persists Step aggregates.
type StepStore interface {
	CreateStep(ctx context.Context, s *Step) (int64, error)
	GetStep(ctx context.Context, id int64) (*Step, error)
	ListStepsByFlow(ctx context.Context, flowID int64) ([]*Step, error)
	UpdateStepStatus(ctx context.Context, id int64, status StepStatus) error
	UpdateStep(ctx context.Context, s *Step) error
	DeleteStep(ctx context.Context, id int64) error
}

// ExecutionStore persists Execution aggregates.
type ExecutionStore interface {
	CreateExecution(ctx context.Context, e *Execution) (int64, error)
	GetExecution(ctx context.Context, id int64) (*Execution, error)
	ListExecutionsByStep(ctx context.Context, stepID int64) ([]*Execution, error)
	ListExecutionsByStatus(ctx context.Context, status ExecutionStatus) ([]*Execution, error)
	UpdateExecution(ctx context.Context, e *Execution) error
}

// ArtifactStore persists Artifact records.
type ArtifactStore interface {
	CreateArtifact(ctx context.Context, a *Artifact) (int64, error)
	GetArtifact(ctx context.Context, id int64) (*Artifact, error)
	GetLatestArtifactByStep(ctx context.Context, stepID int64) (*Artifact, error)
	ListArtifactsByExecution(ctx context.Context, execID int64) ([]*Artifact, error)
	UpdateArtifact(ctx context.Context, a *Artifact) error
}

// BriefingStore persists Briefing records.
type BriefingStore interface {
	CreateBriefing(ctx context.Context, b *Briefing) (int64, error)
	GetBriefing(ctx context.Context, id int64) (*Briefing, error)
	GetBriefingByStep(ctx context.Context, stepID int64) (*Briefing, error)
}

// AgentContextStore persists AgentContext records.
type AgentContextStore interface {
	CreateAgentContext(ctx context.Context, ac *AgentContext) (int64, error)
	GetAgentContext(ctx context.Context, id int64) (*AgentContext, error)
	FindAgentContext(ctx context.Context, agentID string, flowID int64) (*AgentContext, error)
	UpdateAgentContext(ctx context.Context, ac *AgentContext) error
}

// EventStore persists domain events.
type EventStore interface {
	CreateEvent(ctx context.Context, e *Event) (int64, error)
	ListEvents(ctx context.Context, filter EventFilter) ([]*Event, error)
	GetLatestExecutionEventTime(ctx context.Context, execID int64, eventType EventType) (*time.Time, error)
}

// DAGTemplateStore persists DAGTemplate records.
type DAGTemplateStore interface {
	CreateDAGTemplate(ctx context.Context, t *DAGTemplate) (int64, error)
	GetDAGTemplate(ctx context.Context, id int64) (*DAGTemplate, error)
	ListDAGTemplates(ctx context.Context, filter DAGTemplateFilter) ([]*DAGTemplate, error)
	UpdateDAGTemplate(ctx context.Context, t *DAGTemplate) error
	DeleteDAGTemplate(ctx context.Context, id int64) error
}

// ExecutionProbeStore persists probe records and execution routing metadata.
type ExecutionProbeStore interface {
	CreateExecutionProbe(ctx context.Context, probe *ExecutionProbe) (int64, error)
	GetExecutionProbe(ctx context.Context, id int64) (*ExecutionProbe, error)
	ListExecutionProbesByExecution(ctx context.Context, executionID int64) ([]*ExecutionProbe, error)
	GetLatestExecutionProbe(ctx context.Context, executionID int64) (*ExecutionProbe, error)
	GetActiveExecutionProbe(ctx context.Context, executionID int64) (*ExecutionProbe, error)
	UpdateExecutionProbe(ctx context.Context, probe *ExecutionProbe) error
	GetExecutionProbeRoute(ctx context.Context, executionID int64) (*ExecutionProbeRoute, error)
}

// Store is the aggregate interface combining all sub-stores.
type Store interface {
	ProjectStore
	ResourceBindingStore
	IssueStore
	FlowStore
	StepStore
	ExecutionStore
	ArtifactStore
	BriefingStore
	AgentContextStore
	EventStore
	ExecutionProbeStore
	AnalyticsStore
	DAGTemplateStore
	UsageStore
	Close() error
}

// FlowFilter constrains Flow queries.
type FlowFilter struct {
	ProjectID *int64
	Status    *FlowStatus
	Archived  *bool
	Limit     int
	Offset    int
}

// EventFilter constrains Event queries.
type EventFilter struct {
	FlowID *int64
	StepID *int64
	ExecID *int64
	SessionID string
	Types  []EventType
	Limit  int
	Offset int
}
