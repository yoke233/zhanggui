package engine

import (
	"context"
	"log/slog"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// EventPersister subscribes to all events on the EventBus and writes them to EventStore.
// Transient streaming events (individual chunks) are skipped — only aggregated
// events (agent_message, agent_thought, tool_call, done) are persisted.
type EventPersister struct {
	store core.EventStore
	bus   core.EventBus
	sub   *core.Subscription
}

// NewEventPersister creates an EventPersister.
func NewEventPersister(store core.EventStore, bus core.EventBus) *EventPersister {
	return &EventPersister{store: store, bus: bus}
}

// Start subscribes to all events and begins persisting in a background goroutine.
func (p *EventPersister) Start(ctx context.Context) error {
	p.sub = p.bus.Subscribe(core.SubscribeOpts{BufferSize: 256})
	go p.loop(ctx)
	return nil
}

func (p *EventPersister) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-p.sub.C:
			if !ok {
				return
			}
			// Skip transient chunk events — only persist aggregated content.
			if core.IsTransientAgentEvent(ev) {
				continue
			}
			if _, err := p.store.CreateEvent(ctx, &ev); err != nil {
				slog.Warn("v2 event persister: store event failed",
					"type", ev.Type, "flow_id", ev.FlowID, "error", err)
			}
		}
	}
}

// Stop cancels the subscription and stops the background goroutine.
func (p *EventPersister) Stop() {
	if p.sub != nil {
		p.sub.Cancel()
	}
}
