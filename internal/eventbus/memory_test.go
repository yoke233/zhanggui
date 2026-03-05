package eventbus

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestMemoryBusPubSub(t *testing.T) {
	bus := New()
	defer bus.Close()

	sub, err := bus.Subscribe(core.WithName("test"))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	evt := core.Event{Type: core.EventStageStart, RunID: "p1", Timestamp: time.Now()}
	if err := bus.Publish(context.Background(), evt); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-sub.C:
		if got.RunID != "p1" {
			t.Fatalf("expected p1, got %s", got.RunID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMemoryBusMultipleSubscribers(t *testing.T) {
	bus := New()
	defer bus.Close()

	sub1, _ := bus.Subscribe(core.WithName("sub1"))
	sub2, _ := bus.Subscribe(core.WithName("sub2"))
	defer sub1.Unsubscribe()
	defer sub2.Unsubscribe()

	evt := core.Event{Type: core.EventAgentOutput, RunID: "p2", Timestamp: time.Now()}
	_ = bus.Publish(context.Background(), evt)

	for _, sub := range []*core.Subscription{sub1, sub2} {
		select {
		case got := <-sub.C:
			if got.RunID != "p2" {
				t.Fatalf("expected p2, got %s", got.RunID)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestMemoryBusTypeFilter(t *testing.T) {
	bus := New()
	defer bus.Close()

	// Subscribe only to stage_start events
	sub, _ := bus.Subscribe(
		core.WithName("filtered"),
		core.WithTypes(core.EventStageStart),
	)
	defer sub.Unsubscribe()

	// Publish a non-matching event
	_ = bus.Publish(context.Background(), core.Event{Type: core.EventRunDone, RunID: "r1", Timestamp: time.Now()})
	// Publish a matching event
	_ = bus.Publish(context.Background(), core.Event{Type: core.EventStageStart, RunID: "r2", Timestamp: time.Now()})

	select {
	case got := <-sub.C:
		if got.Type != core.EventStageStart {
			t.Fatalf("expected stage_start, got %s", got.Type)
		}
		if got.RunID != "r2" {
			t.Fatalf("expected r2, got %s", got.RunID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for filtered event")
	}

	// Ensure no extra events
	select {
	case extra := <-sub.C:
		t.Fatalf("unexpected extra event: %s", extra.Type)
	case <-time.After(50 * time.Millisecond):
		// good
	}
}

func TestMemoryBusDroppedEventLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	bus := New()
	bus.log = logger
	defer bus.Close()

	// Tiny buffer to force drops
	sub, _ := bus.Subscribe(core.WithName("slow-consumer"), core.WithBufferSize(1))
	defer sub.Unsubscribe()

	// Fill the buffer
	_ = bus.Publish(context.Background(), core.Event{Type: core.EventRunDone, RunID: "fill1", Timestamp: time.Now()})
	// This should be dropped
	_ = bus.Publish(context.Background(), core.Event{Type: core.EventRunDone, RunID: "fill2", Timestamp: time.Now()})

	logOutput := buf.String()
	if !strings.Contains(logOutput, "event dropped") {
		t.Fatalf("expected 'event dropped' warning in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "slow-consumer") {
		t.Fatalf("expected subscriber name 'slow-consumer' in log, got: %s", logOutput)
	}
}

func TestMemoryBusUnsubscribe(t *testing.T) {
	bus := New()
	defer bus.Close()

	sub, _ := bus.Subscribe(core.WithName("unsub-test"))
	sub.Unsubscribe()

	// Channel should be closed after unsubscribe
	_, ok := <-sub.C
	if ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}
}

func TestMemoryBusClose(t *testing.T) {
	bus := New()

	sub, _ := bus.Subscribe(core.WithName("close-test"))
	_ = bus.Close()

	// Channel should be closed
	_, ok := <-sub.C
	if ok {
		t.Fatal("expected channel to be closed after bus.Close")
	}

	// Publish after close should not panic
	err := bus.Publish(context.Background(), core.Event{Type: core.EventRunDone, Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("publish after close should return nil, got %v", err)
	}
}

func TestMemoryBusSubscriberIsolation(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	bus := New()
	bus.log = logger
	defer bus.Close()

	// One slow subscriber with buffer=1
	slow, _ := bus.Subscribe(core.WithName("slow"), core.WithBufferSize(1))
	defer slow.Unsubscribe()

	// One fast subscriber with default buffer
	fast, _ := bus.Subscribe(core.WithName("fast"))
	defer fast.Unsubscribe()

	// Fill slow subscriber's buffer
	_ = bus.Publish(context.Background(), core.Event{Type: core.EventRunDone, RunID: "e1", Timestamp: time.Now()})
	// This should drop for slow but deliver to fast
	_ = bus.Publish(context.Background(), core.Event{Type: core.EventRunDone, RunID: "e2", Timestamp: time.Now()})

	// Fast subscriber should get both
	for _, expected := range []string{"e1", "e2"} {
		select {
		case got := <-fast.C:
			if got.RunID != expected {
				t.Fatalf("fast: expected %s, got %s", expected, got.RunID)
			}
		case <-time.After(time.Second):
			t.Fatalf("fast: timeout waiting for %s", expected)
		}
	}

	// Slow subscriber should get only the first
	select {
	case got := <-slow.C:
		if got.RunID != "e1" {
			t.Fatalf("slow: expected e1, got %s", got.RunID)
		}
	case <-time.After(time.Second):
		t.Fatal("slow: timeout waiting for e1")
	}

	// Verify drop was logged
	if !strings.Contains(buf.String(), "event dropped") {
		t.Fatal("expected 'event dropped' in log for slow subscriber")
	}
}
