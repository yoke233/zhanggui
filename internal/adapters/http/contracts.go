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
	core.ResourceSpaceStore
	core.ResourceStore
	core.ActionIODeclStore
	core.WorkItemStore
	core.ThreadStore
	core.ActionStore
	core.RunStore
	core.AgentContextStore
	core.EventStore
	core.AnalyticsStore
	core.DAGTemplateStore
	core.UsageStore
	core.FeatureEntryStore
	core.ActionSignalStore
	core.JournalStore
	core.NotificationStore
	core.InspectionStore
	core.ThreadTaskStore
	DeleteResourcesByThread(ctx context.Context, threadID int64) error
	GetThreadMessage(ctx context.Context, id int64) (*core.ThreadMessage, error)
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
	ResolvePermission(permissionID, optionID string, cancel bool) error
	CancelChat(sessionID string) error
	CloseSession(sessionID string)
	DeleteSession(sessionID string)
	IsSessionAlive(sessionID string) bool
	IsSessionRunning(sessionID string) bool
}

// DAGGenerator is the planning contract required by the HTTP adapter.
type DAGGenerator interface {
	Generate(ctx context.Context, input planningapp.GenerateInput) (*planningapp.GeneratedDAG, error)
	Materialize(ctx context.Context, store core.Store, issueID int64, dag *planningapp.GeneratedDAG) ([]*core.Action, error)
}

// TextCompleter generates free-form text from a prompt (used for title generation, etc.).
type TextCompleter interface {
	CompleteText(ctx context.Context, prompt string) (string, error)
}

// ThreadAgentRuntime bridges Thread agent HTTP/WS endpoints to the ACP runtime.
type ThreadAgentRuntime interface {
	InviteAgent(ctx context.Context, threadID int64, profileID string) (*core.ThreadMember, error)
	WaitAgentReady(ctx context.Context, threadID int64, profileID string) error
	SendMessage(ctx context.Context, threadID int64, profileID string, message string) error
	RemoveAgent(ctx context.Context, threadID int64, agentSessionID int64) error
	CleanupThread(ctx context.Context, threadID int64) error
	ActiveAgentProfileIDs(threadID int64) []string
}
