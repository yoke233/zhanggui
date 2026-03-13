package flow

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Store is the application-facing persistence port required by issue orchestration.
// It intentionally exposes only the sub-stores used by the issue application layer.
type Store interface {
	core.ProjectStore
	core.ResourceBindingStore
	core.IssueStore
	core.StepStore
	core.ExecutionStore
	core.ArtifactStore
	core.BriefingStore
	core.FeatureManifestStore
	core.StepSignalStore
}

// EventStore is the persistence port required for persisting emitted events.
type EventStore interface {
	core.EventStore
}

// EventPublisher is the minimal outbound event port required by issue orchestration.
type EventPublisher interface {
	Publish(ctx context.Context, event core.Event)
}

// EventBus is the subscribe-capable event port used by background consumers.
type EventBus interface {
	EventPublisher
	Subscribe(opts core.SubscribeOpts) *core.Subscription
}

// WorkspaceProvider prepares and releases isolated workspaces for an issue run.
type WorkspaceProvider interface {
	Prepare(ctx context.Context, project *core.Project, bindings []*core.ResourceBinding, issueID int64) (*core.Workspace, error)
	Release(ctx context.Context, ws *core.Workspace) error
}
