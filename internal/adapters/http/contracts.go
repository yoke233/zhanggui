package api

import (
	"context"

	chatapp "github.com/yoke233/ai-workflow/internal/application/chat"
	planningapp "github.com/yoke233/ai-workflow/internal/application/planning"
	"github.com/yoke233/ai-workflow/internal/core"
)

// Store is the persistence contract required by the HTTP adapter.
type Store interface {
	core.ProjectStore
	core.ResourceBindingStore
	core.IssueStore
	core.FlowStore
	core.StepStore
	core.ExecutionStore
	core.ArtifactStore
	core.BriefingStore
	core.AgentContextStore
	core.EventStore
	core.ExecutionProbeStore
	core.DAGTemplateStore
	Close() error
}

// EventBus is the event contract required by the HTTP adapter.
type EventBus interface {
	Publish(ctx context.Context, event core.Event)
	Subscribe(opts core.SubscribeOpts) *core.Subscription
}

// LeadChatService is the chat contract required by the HTTP adapter.
type LeadChatService interface {
	Chat(ctx context.Context, req chatapp.Request) (*chatapp.Response, error)
	StartChat(ctx context.Context, req chatapp.Request) (*chatapp.AcceptedResponse, error)
	ListSessions(ctx context.Context) ([]chatapp.SessionSummary, error)
	GetSession(ctx context.Context, sessionID string) (*chatapp.SessionDetail, error)
	CancelChat(sessionID string) error
	CloseSession(sessionID string)
	IsSessionAlive(sessionID string) bool
	IsSessionRunning(sessionID string) bool
}

// DAGGenerator is the planning contract required by the HTTP adapter.
type DAGGenerator interface {
	Generate(ctx context.Context, taskDescription string) (*planningapp.GeneratedDAG, error)
	Materialize(ctx context.Context, store core.Store, flowID int64, dag *planningapp.GeneratedDAG) ([]*core.Step, error)
}
