package github

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type reconnectEventPublisher interface {
	Publish(ctx context.Context, evt core.Event) error
}

type RunEventSyncer interface {
	SyncRunEvent(ctx context.Context, evt core.Event) error
}

// ReconnectSync handles recovery from degraded GitHub connectivity.
type ReconnectSync struct {
	publisher reconnectEventPublisher
	syncer    RunEventSyncer
	now       func() time.Time

	mu       sync.RWMutex
	degraded bool
}

func NewReconnectSync(publisher reconnectEventPublisher, syncer RunEventSyncer) *ReconnectSync {
	return &ReconnectSync{
		publisher: publisher,
		syncer:    syncer,
		now:       time.Now,
	}
}

func (r *ReconnectSync) MarkDegraded(err error) {
	if r == nil {
		return
	}
	if !isNetworkError(err) {
		return
	}
	r.mu.Lock()
	r.degraded = true
	r.mu.Unlock()
}

func (r *ReconnectSync) IsDegraded() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.degraded
}

func (r *ReconnectSync) OnRecovered(ctx context.Context, events []core.Event) error {
	if r == nil {
		return nil
	}

	if !r.IsDegraded() {
		return nil
	}

	r.mu.Lock()
	r.degraded = false
	r.mu.Unlock()

	if r.publisher != nil {
		r.publisher.Publish(ctx, core.Event{
			Type:      core.EventGitHubReconnected,
			Timestamp: r.now(),
		})
	}

	return r.ReplayLatestRunstateOnly(ctx, events)
}

func (r *ReconnectSync) ReplayLatestRunstateOnly(ctx context.Context, events []core.Event) error {
	if r == nil || r.syncer == nil || len(events) == 0 {
		return nil
	}

	latestByIssue := make(map[int]core.Event)
	for _, evt := range events {
		if !isReplayableRunstateEvent(evt.Type) {
			continue
		}
		issueNumber := parseIssueNumberFromEventData(evt.Data)
		if issueNumber <= 0 {
			continue
		}

		existing, ok := latestByIssue[issueNumber]
		if !ok || evt.Timestamp.After(existing.Timestamp) {
			latestByIssue[issueNumber] = evt
		}
	}

	issues := make([]int, 0, len(latestByIssue))
	for issueNumber := range latestByIssue {
		issues = append(issues, issueNumber)
	}
	sort.Ints(issues)

	for _, issueNumber := range issues {
		if err := r.syncer.SyncRunEvent(ctx, latestByIssue[issueNumber]); err != nil {
			return err
		}
	}
	return nil
}

func isReplayableRunstateEvent(eventType core.EventType) bool {
	switch eventType {
	case core.EventStageStart,
		core.EventStageComplete,
		core.EventHumanRequired,
		core.EventRunDone,
		core.EventRunFailed:
		return true
	default:
		return false
	}
}
