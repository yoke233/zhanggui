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
	core.ThreadStore
	core.StepStore
	core.ExecutionStore
	core.ArtifactStore
	core.BriefingStore
	core.AgentContextStore
	core.EventStore
	core.ExecutionProbeStore
	core.AnalyticsStore
	core.DAGTemplateStore
	core.UsageStore
	core.FeatureManifestStore
	core.NotificationStore
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
	SetConfigOption(ctx context.Context, sessionID, configID, value string) ([]chatapp.ConfigOption, error)
	SetSessionMode(ctx context.Context, sessionID, modeID string) (*chatapp.SessionModeState, error)
	CancelChat(sessionID string) error
	CloseSession(sessionID string)
	DeleteSession(sessionID string)
	IsSessionAlive(sessionID string) bool
	IsSessionRunning(sessionID string) bool
}

// DAGGenerator is the planning contract required by the HTTP adapter.
type DAGGenerator interface {
	Generate(ctx context.Context, taskDescription string) (*planningapp.GeneratedDAG, error)
	Materialize(ctx context.Context, store core.Store, issueID int64, dag *planningapp.GeneratedDAG) ([]*core.Step, error)
}

// TextCompleter generates free-form text from a prompt (used for title generation, etc.).
type TextCompleter interface {
	CompleteText(ctx context.Context, prompt string) (string, error)
}

// ThreadAgentRuntime bridges Thread agent HTTP/WS endpoints to the ACP runtime.
type ThreadAgentRuntime interface {
	InviteAgent(ctx context.Context, threadID int64, profileID string) (*core.ThreadAgentSession, error)
	SendMessage(ctx context.Context, threadID int64, profileID string, message string) error
	RemoveAgent(ctx context.Context, threadID int64, agentSessionID int64) error
	ActiveAgentProfileIDs(threadID int64) []string
}
