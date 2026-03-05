package teamleader

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- mock types ---

type mockStore struct {
	core.Store
	issue   *core.Issue
	run     *core.Run
	project *core.Project

	issueErr    error
	getIssueErr error
	runErr      error
	projectErr  error
}

func (m *mockStore) GetIssueByRun(runID string) (*core.Issue, error) {
	return m.issue, m.issueErr
}

func (m *mockStore) GetIssue(id string) (*core.Issue, error) {
	return m.issue, m.getIssueErr
}

func (m *mockStore) GetRun(id string) (*core.Run, error) {
	return m.run, m.runErr
}

func (m *mockStore) GetProject(id string) (*core.Project, error) {
	return m.project, m.projectErr
}

type mockBus struct {
	events []core.Event
}

func (m *mockBus) Publish(_ context.Context, evt core.Event) error {
	m.events = append(m.events, evt)
	return nil
}

func (m *mockBus) Subscribe(_ ...core.SubOption) (*core.Subscription, error) {
	return &core.Subscription{}, nil
}

func (m *mockBus) Close() error {
	return nil
}

type mockMerger struct {
	createURL string
	createErr error
	mergeErr  error
	called    struct{ create, merge bool }
}

func (m *mockMerger) OnImplementComplete(ctx context.Context, runID string) (string, error) {
	m.called.create = true
	return m.createURL, m.createErr
}

func (m *mockMerger) OnMergeApproved(ctx context.Context, runID string) error {
	m.called.merge = true
	return m.mergeErr
}

// --- helpers ---

func baseFixtures() (*mockStore, *mockBus, *mockMerger) {
	store := &mockStore{
		issue: &core.Issue{
			ID:        "issue-1",
			AutoMerge: true,
			RunID:     "run-1",
			Status:    core.IssueStatusMerging,
		},
		run: &core.Run{
			ID:         "run-1",
			ProjectID:  "proj-1",
			BranchName: "feat/x",
		},
		project: &core.Project{
			ID:       "proj-1",
			RepoPath: "/tmp/repo",
		},
	}
	bus := &mockBus{}
	merger := &mockMerger{createURL: "https://github.com/org/repo/pull/42"}
	return store, bus, merger
}

func noopTestGate(_ context.Context, _ string) error { return nil }

func issueMergingEvent() core.Event {
	return core.Event{
		Type:      core.EventIssueMerging,
		IssueID:   "issue-1",
		Timestamp: time.Now(),
	}
}

func runDoneEvent() core.Event {
	return core.Event{
		Type:      core.EventRunDone,
		RunID:     "run-1",
		Timestamp: time.Now(),
	}
}

// --- tests ---

func TestAutoMerge_HappyPath(t *testing.T) {
	store, bus, merger := baseFixtures()
	h := NewAutoMergeHandler(store, bus, merger)
	h.testGateFn = noopTestGate

	h.OnEvent(context.Background(), issueMergingEvent())

	if !merger.called.create {
		t.Fatal("expected OnImplementComplete to be called")
	}
	if !merger.called.merge {
		t.Fatal("expected OnMergeApproved to be called")
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	evt := bus.events[0]
	if evt.Type != core.EventIssueMerged {
		t.Fatalf("expected EventIssueMerged, got %s", evt.Type)
	}
	if evt.Data["pr_url"] != "https://github.com/org/repo/pull/42" {
		t.Fatalf("unexpected pr_url: %s", evt.Data["pr_url"])
	}
	if evt.Data["branch"] != "feat/x" {
		t.Fatalf("unexpected branch: %s", evt.Data["branch"])
	}
}

func TestAutoMerge_RunDoneFallbackPath(t *testing.T) {
	store, bus, merger := baseFixtures()
	store.issue.Status = core.IssueStatusExecuting
	h := NewAutoMergeHandler(store, bus, merger)
	h.testGateFn = noopTestGate

	h.OnEvent(context.Background(), runDoneEvent())

	if !merger.called.create {
		t.Fatal("expected OnImplementComplete to be called on run_done fallback")
	}
	if !merger.called.merge {
		t.Fatal("expected OnMergeApproved to be called on run_done fallback")
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	if bus.events[0].Type != core.EventIssueMerged {
		t.Fatalf("expected EventIssueMerged, got %s", bus.events[0].Type)
	}
}

func TestAutoMerge_DeduplicateRunDoneAndIssueMerging(t *testing.T) {
	store, bus, merger := baseFixtures()
	store.issue.Status = core.IssueStatusExecuting
	h := NewAutoMergeHandler(store, bus, merger)
	h.testGateFn = noopTestGate

	h.OnEvent(context.Background(), runDoneEvent())
	store.issue.Status = core.IssueStatusMerging
	h.OnEvent(context.Background(), issueMergingEvent())

	if len(bus.events) != 1 {
		t.Fatalf("expected one merge terminal event after dedupe, got %d", len(bus.events))
	}
}

func TestAutoMerge_NonAutoMerge(t *testing.T) {
	store, bus, merger := baseFixtures()
	store.issue.AutoMerge = false
	h := NewAutoMergeHandler(store, bus, merger)
	h.testGateFn = noopTestGate

	h.OnEvent(context.Background(), issueMergingEvent())

	if len(bus.events) != 0 {
		t.Fatalf("expected no events, got %d", len(bus.events))
	}
	if merger.called.create || merger.called.merge {
		t.Fatal("merger should not be called when AutoMerge is false")
	}
}

func TestAutoMerge_NonIssueMergingEvent(t *testing.T) {
	store, bus, merger := baseFixtures()
	h := NewAutoMergeHandler(store, bus, merger)
	h.testGateFn = noopTestGate

	h.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunFailed,
		RunID:     "run-1",
		Timestamp: time.Now(),
	})

	if len(bus.events) != 0 {
		t.Fatalf("expected no events for non-issue_merging, got %d", len(bus.events))
	}
}

func TestAutoMerge_MergerNil(t *testing.T) {
	store, bus, _ := baseFixtures()
	h := NewAutoMergeHandler(store, bus, nil)
	h.testGateFn = noopTestGate

	h.OnEvent(context.Background(), issueMergingEvent())

	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	evt := bus.events[0]
	if evt.Type != core.EventIssueMerged {
		t.Fatalf("expected EventIssueMerged, got %s", evt.Type)
	}
	if _, ok := evt.Data["pr_url"]; ok {
		t.Fatal("pr_url should not be set when merger is nil")
	}
}

func TestAutoMerge_PRCreateFailure(t *testing.T) {
	store, bus, merger := baseFixtures()
	merger.createErr = errors.New("github 500")
	h := NewAutoMergeHandler(store, bus, merger)
	h.testGateFn = noopTestGate

	h.OnEvent(context.Background(), issueMergingEvent())

	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	evt := bus.events[0]
	if evt.Type != core.EventMergeFailed {
		t.Fatalf("expected EventMergeFailed, got %s", evt.Type)
	}
	if evt.Data["phase"] != "auto_merge_create_pr" {
		t.Fatalf("expected phase auto_merge_create_pr, got %s", evt.Data["phase"])
	}
	if !merger.called.create {
		t.Fatal("expected OnImplementComplete to be called")
	}
	if merger.called.merge {
		t.Fatal("OnMergeApproved should not be called after create failure")
	}
}

func TestAutoMerge_PRMergeFailure(t *testing.T) {
	store, bus, merger := baseFixtures()
	merger.mergeErr = errors.New("merge conflict")
	h := NewAutoMergeHandler(store, bus, merger)
	h.testGateFn = noopTestGate

	h.OnEvent(context.Background(), issueMergingEvent())

	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	evt := bus.events[0]
	if evt.Type != core.EventIssueMergeConflict {
		t.Fatalf("expected EventIssueMergeConflict, got %s", evt.Type)
	}
	if evt.Data["phase"] != "auto_merge_merge_pr" {
		t.Fatalf("expected phase auto_merge_merge_pr, got %s", evt.Data["phase"])
	}
	if !merger.called.create {
		t.Fatal("expected OnImplementComplete to be called")
	}
	if !merger.called.merge {
		t.Fatal("expected OnMergeApproved to be called")
	}
}

func TestAutoMerge_PRMergeNonConflictFailure(t *testing.T) {
	store, bus, merger := baseFixtures()
	merger.mergeErr = errors.New("github 500 while merge")
	h := NewAutoMergeHandler(store, bus, merger)
	h.testGateFn = noopTestGate

	h.OnEvent(context.Background(), issueMergingEvent())

	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	evt := bus.events[0]
	if evt.Type != core.EventMergeFailed {
		t.Fatalf("expected EventMergeFailed, got %s", evt.Type)
	}
	if evt.Data["phase"] != "auto_merge_merge_pr" {
		t.Fatalf("expected phase auto_merge_merge_pr, got %s", evt.Data["phase"])
	}
}

func TestAutoMerge_TestGateFailurePublishesMergeFailed(t *testing.T) {
	store, bus, merger := baseFixtures()
	h := NewAutoMergeHandler(store, bus, merger)
	h.testGateFn = func(_ context.Context, _ string) error {
		return errors.New("go test failed")
	}

	h.OnEvent(context.Background(), issueMergingEvent())

	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	evt := bus.events[0]
	if evt.Type != core.EventMergeFailed {
		t.Fatalf("expected EventMergeFailed, got %s", evt.Type)
	}
	if evt.Data["phase"] != "auto_merge_test_gate" {
		t.Fatalf("expected phase auto_merge_test_gate, got %s", evt.Data["phase"])
	}
}
