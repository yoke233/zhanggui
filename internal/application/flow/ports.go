package flow

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Store is the application-facing persistence port required by work item orchestration.
// It intentionally exposes only the sub-stores used by the work item application layer.
type Store interface {
	core.ProjectStore
	core.ResourceSpaceStore
	core.ResourceStore
	core.ActionIODeclStore
	core.WorkItemStore
	core.ActionStore
	core.RunStore
	core.FeatureEntryStore
	core.ActionSignalStore
}

// EventStore is the persistence port required for persisting emitted events.
type EventStore interface {
	core.EventStore
	core.JournalStore
}

// EventPublisher is the minimal outbound event port required by work item orchestration.
type EventPublisher interface {
	Publish(ctx context.Context, event core.Event)
}

// EventBus is the subscribe-capable event port used by background consumers.
type EventBus interface {
	EventPublisher
	Subscribe(opts core.SubscribeOpts) *core.Subscription
}

// WorkspaceProvider prepares and releases isolated workspaces for a work item run.
type WorkspaceProvider interface {
	Prepare(ctx context.Context, project *core.Project, spaces []*core.ResourceSpace, workItemID int64) (*core.Workspace, error)
	Release(ctx context.Context, ws *core.Workspace) error
}
