package probe

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Store is the application-facing persistence port required by run probe use cases.
type Store interface {
	core.RunStore
	core.EventStore
	core.RunProbeStore
}

// EventPublisher is the minimal outbound event port required by probe workflows.
type EventPublisher interface {
	Publish(ctx context.Context, event core.Event)
}
