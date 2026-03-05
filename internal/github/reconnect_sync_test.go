package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestReconnectSync_OnRecovered_PublishesGitHubReconnected(t *testing.T) {
	publisher := &fakeReconnectPublisher{}
	syncer := &fakeRunEventSyncer{}
	reconnect := NewReconnectSync(publisher, syncer)

	reconnect.MarkDegraded(errors.New("dial tcp timeout"))
	if err := reconnect.OnRecovered(context.Background(), nil); err != nil {
		t.Fatalf("OnRecovered() error = %v", err)
	}

	if len(publisher.events) != 1 {
		t.Fatalf("expected one reconnected event, got %d", len(publisher.events))
	}
	if publisher.events[0].Type != core.EventGitHubReconnected {
		t.Fatalf("expected event %q, got %q", core.EventGitHubReconnected, publisher.events[0].Type)
	}
}

func TestReconnectSync_ReplaysLatestRunstateOnly(t *testing.T) {
	publisher := &fakeReconnectPublisher{}
	syncer := &fakeRunEventSyncer{}
	reconnect := NewReconnectSync(publisher, syncer)

	base := time.Now()
	events := []core.Event{
		{
			Type:      core.EventStageStart,
			Timestamp: base.Add(1 * time.Second),
			Data: map[string]string{
				"issue_number": "11",
			},
		},
		{
			Type:      core.EventRunDone,
			Timestamp: base.Add(3 * time.Second),
			Data: map[string]string{
				"issue_number": "11",
			},
		},
		{
			Type:      core.EventStageStart,
			Timestamp: base.Add(2 * time.Second),
			Data: map[string]string{
				"issue_number": "22",
			},
		},
		{
			Type:      core.EventHumanRequired,
			Timestamp: base.Add(4 * time.Second),
			Data: map[string]string{
				"issue_number": "22",
			},
		},
	}

	if err := reconnect.ReplayLatestRunstateOnly(context.Background(), events); err != nil {
		t.Fatalf("ReplayLatestRunstateOnly() error = %v", err)
	}

	if len(syncer.events) != 2 {
		t.Fatalf("expected latest events replayed per issue only, got %d", len(syncer.events))
	}
	if syncer.events[0].Type != core.EventRunDone {
		t.Fatalf("expected issue 11 latest event run_done, got %q", syncer.events[0].Type)
	}
	if syncer.events[1].Type != core.EventHumanRequired {
		t.Fatalf("expected issue 22 latest event human_required, got %q", syncer.events[1].Type)
	}
}

type fakeReconnectPublisher struct {
	events []core.Event
}

func (f *fakeReconnectPublisher) Publish(_ context.Context, evt core.Event) error {
	f.events = append(f.events, evt)
	return nil
}

type fakeRunEventSyncer struct {
	events []core.Event
}

func (f *fakeRunEventSyncer) SyncRunEvent(_ context.Context, evt core.Event) error {
	f.events = append(f.events, evt)
	return nil
}
