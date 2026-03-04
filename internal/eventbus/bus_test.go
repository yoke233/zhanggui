package eventbus

import (
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestBusPubSub(t *testing.T) {
	bus := New()
	defer bus.Close()

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	evt := core.Event{Type: core.EventStageStart, RunID: "p1", Timestamp: time.Now()}
	bus.Publish(evt)

	select {
	case got := <-ch:
		if got.RunID != "p1" {
			t.Fatalf("expected p1, got %s", got.RunID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	bus := New()
	defer bus.Close()

	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe()
	defer bus.Unsubscribe(ch1)
	defer bus.Unsubscribe(ch2)

	evt := core.Event{Type: core.EventAgentOutput, RunID: "p2", Timestamp: time.Now()}
	bus.Publish(evt)

	for _, ch := range []<-chan core.Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.RunID != "p2" {
				t.Fatalf("expected p2, got %s", got.RunID)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}
