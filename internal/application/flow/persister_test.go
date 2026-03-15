package flow

import (
	"context"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestEventPersister_PersistsEvents(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	persister := NewEventPersister(store, bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := persister.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer persister.Stop()

	// Publish some events.
	bus.Publish(ctx, core.Event{
		Type:       core.EventWorkItemStarted,
		WorkItemID: 1,
		Timestamp:  time.Now().UTC(),
	})
	bus.Publish(ctx, core.Event{
		Type:       core.EventActionReady,
		WorkItemID: 1,
		ActionID:   10,
		Timestamp:  time.Now().UTC(),
	})
	bus.Publish(ctx, core.Event{
		Type:       core.EventRunCreated,
		WorkItemID: 1,
		ActionID:   10,
		RunID:      100,
		Timestamp:  time.Now().UTC(),
		Data:       map[string]any{"agent": "claude"},
	})

	// Give the goroutine time to process.
	time.Sleep(100 * time.Millisecond)

	events, err := store.ListEvents(ctx, core.EventFilter{})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 persisted events, got %d", len(events))
	}
	if events[0].Type != core.EventWorkItemStarted {
		t.Errorf("event[0] type = %s, want work_item.started", events[0].Type)
	}
	if events[1].ActionID != 10 {
		t.Errorf("event[1] action_id = %d, want 10", events[1].ActionID)
	}
	if events[2].Data["agent"] != "claude" {
		t.Errorf("event[2] data agent = %v, want claude", events[2].Data["agent"])
	}
}

func TestEventPersister_StopsCleanly(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	persister := NewEventPersister(store, bus)
	ctx, cancel := context.WithCancel(context.Background())

	if err := persister.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Stop via cancel.
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Stop should be safe to call even after cancel.
	persister.Stop()
}

func TestEventPersister_ContextCancellation(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	persister := NewEventPersister(store, bus)
	ctx, cancel := context.WithCancel(context.Background())

	if err := persister.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Publish one event, then cancel.
	bus.Publish(ctx, core.Event{
		Type:       core.EventWorkItemStarted,
		WorkItemID: 1,
		Timestamp:  time.Now().UTC(),
	})
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Event should have been persisted before cancellation.
	events, err := store.ListEvents(context.Background(), core.EventFilter{})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 persisted event, got %d", len(events))
	}

	persister.Stop()
}

func TestEventPersister_SkipsTransientChunks(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	persister := NewEventPersister(store, bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := persister.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer persister.Stop()

	// Publish transient chunk events (should NOT be persisted).
	bus.Publish(ctx, core.Event{
		Type:       core.EventRunAgentOutput,
		WorkItemID: 1,
		Data:       map[string]any{"type": "agent_message_chunk", "content": "Hello "},
		Timestamp:  time.Now().UTC(),
	})
	bus.Publish(ctx, core.Event{
		Type:       core.EventRunAgentOutput,
		WorkItemID: 1,
		Data:       map[string]any{"type": "agent_thought_chunk", "content": "thinking..."},
		Timestamp:  time.Now().UTC(),
	})

	// Publish aggregated events (SHOULD be persisted).
	bus.Publish(ctx, core.Event{
		Type:       core.EventRunAgentOutput,
		WorkItemID: 1,
		Data:       map[string]any{"type": "agent_message", "content": "Hello world"},
		Timestamp:  time.Now().UTC(),
	})
	bus.Publish(ctx, core.Event{
		Type:       core.EventRunAgentOutput,
		WorkItemID: 1,
		Data:       map[string]any{"type": "tool_call", "content": "read file"},
		Timestamp:  time.Now().UTC(),
	})
	bus.Publish(ctx, core.Event{
		Type:       core.EventRunAgentOutput,
		WorkItemID: 1,
		Data:       map[string]any{"type": "done", "content": "finished"},
		Timestamp:  time.Now().UTC(),
	})

	// Also test chat output transient filtering.
	bus.Publish(ctx, core.Event{
		Type:      core.EventChatOutput,
		Data:      map[string]any{"type": "agent_message_chunk", "content": "chunk"},
		Timestamp: time.Now().UTC(),
	})
	bus.Publish(ctx, core.Event{
		Type:      core.EventChatOutput,
		Data:      map[string]any{"type": "agent_message", "content": "full"},
		Timestamp: time.Now().UTC(),
	})

	time.Sleep(150 * time.Millisecond)

	events, err := store.ListEvents(ctx, core.EventFilter{})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	// Should have: agent_message + tool_call + done + chat agent_message = 4
	if len(events) != 4 {
		t.Fatalf("expected 4 persisted events (skipping 3 transient chunks), got %d", len(events))
	}

	// Verify the persisted events are the right ones.
	types := make([]string, len(events))
	for i, ev := range events {
		types[i], _ = ev.Data["type"].(string)
	}
	expected := []string{"agent_message", "tool_call", "done", "agent_message"}
	for i, want := range expected {
		if types[i] != want {
			t.Errorf("event[%d] type = %s, want %s", i, types[i], want)
		}
	}
}

func newTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
