package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/web"
)

// isTransientChunkEvent returns true for high-frequency streaming chunk events
// that should NOT be persisted to run_events (they are broadcast via WS only).
func isTransientChunkEvent(evt core.Event) bool {
	if evt.Type != core.EventAgentOutput {
		return false
	}
	switch evt.Data["type"] {
	case "agent_message_chunk", "agent_thought_chunk", "user_message_chunk",
		"available_commands_update", "current_mode_update",
		"config_option_update", "session_info_update":
		return true
	}
	return false
}

// --- Independent event subscribers (replace bridge goroutine) ---

type eventHandler interface {
	Stop(ctx context.Context) error
}

func stopHandlers(ctx context.Context, handlers ...eventHandler) {
	for _, h := range handlers {
		if h != nil {
			_ = h.Stop(ctx)
		}
	}
}

// wsBroadcaster subscribes to all events and pushes them to the WebSocket hub.
type wsBroadcaster struct {
	hub    *web.Hub
	bus    core.EventBus
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newWSBroadcaster(hub *web.Hub, bus core.EventBus) *wsBroadcaster {
	return &wsBroadcaster{hub: hub, bus: bus}
}

func (b *wsBroadcaster) Start(ctx context.Context) error {
	sub, err := b.bus.Subscribe(core.WithName("ws-broadcaster"))
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer sub.Unsubscribe()
		for {
			select {
			case <-runCtx.Done():
				return
			case evt, ok := <-sub.C:
				if !ok {
					return
				}
				b.hub.BroadcastCoreEvent(evt)
			}
		}
	}()
	return nil
}

func (b *wsBroadcaster) Stop(_ context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	return nil
}

// eventPersister subscribes to all events and persists non-transient run events.
type eventPersister struct {
	store  core.Store
	bus    core.EventBus
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newEventPersister(store core.Store, bus core.EventBus) *eventPersister {
	return &eventPersister{store: store, bus: bus}
}

func (p *eventPersister) Start(ctx context.Context) error {
	sub, err := p.bus.Subscribe(core.WithName("event-persister"))
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer sub.Unsubscribe()
		for {
			select {
			case <-runCtx.Done():
				return
			case evt, ok := <-sub.C:
				if !ok {
					return
				}
				if evt.RunID != "" && !isTransientChunkEvent(evt) {
					if err := p.store.SaveRunEvent(core.RunEvent{
						RunID:     evt.RunID,
						ProjectID: evt.ProjectID,
						IssueID:   evt.IssueID,
						EventType: string(evt.Type),
						Stage:     string(evt.Stage),
						Agent:     evt.Agent,
						Data:      evt.Data,
						Error:     evt.Error,
						CreatedAt: evt.Timestamp,
					}); err != nil {
						slog.Warn("failed to persist run event", "run_id", evt.RunID, "type", evt.Type, "error", err)
					}
				}
			}
		}
	}()
	return nil
}

func (p *eventPersister) Stop(_ context.Context) error {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	return nil
}
