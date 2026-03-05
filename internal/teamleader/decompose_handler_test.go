package teamleader

import (
	"context"
	"errors"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- applyIssueApprove decomposition branch tests ---

func TestApplyIssueApprove_EpicGoesToDecomposing(t *testing.T) {
	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	store.CreateProject(&core.Project{ID: "proj-1", Name: "svc", RepoPath: t.TempDir()})
	store.CreateIssue(&core.Issue{
		ID: "epic-1", ProjectID: "proj-1", Title: "Build feature X",
		Template: "epic", State: core.IssueStateOpen, Status: core.IssueStatusReviewing,
	})

	var published []core.Event
	pub := &capturePublisher{events: &published}

	mgr, err := NewManager(store, nil, nil, &fakeManagerScheduler{},
		WithEventPublisher(pub))
	if err != nil {
		t.Fatal(err)
	}

	issue, _ := store.GetIssue("epic-1")
	result, err := mgr.applyIssueApprove(context.Background(), issue, "looks good")
	if err != nil {
		t.Fatalf("applyIssueApprove: %v", err)
	}
	if result.Status != core.IssueStatusDecomposing {
		t.Errorf("status = %q, want decomposing", result.Status)
	}

	// Verify event published
	found := false
	for _, evt := range published {
		if evt.Type == core.EventIssueDecomposing && evt.IssueID == "epic-1" {
			found = true
		}
	}
	if !found {
		t.Error("EventIssueDecomposing not published")
	}
}

func TestApplyIssueApprove_StandardGoesToQueued(t *testing.T) {
	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	store.CreateProject(&core.Project{ID: "proj-1", Name: "svc", RepoPath: t.TempDir()})
	store.CreateIssue(&core.Issue{
		ID: "task-1", ProjectID: "proj-1", Title: "Fix bug",
		Template: "standard", State: core.IssueStateOpen, Status: core.IssueStatusReviewing,
	})

	mgr, err := NewManager(store, nil, nil, &fakeManagerScheduler{})
	if err != nil {
		t.Fatal(err)
	}

	issue, _ := store.GetIssue("task-1")
	result, err := mgr.applyIssueApprove(context.Background(), issue, "approved")
	if err != nil {
		t.Fatalf("applyIssueApprove: %v", err)
	}
	if result.Status != core.IssueStatusQueued {
		t.Errorf("status = %q, want queued", result.Status)
	}
}

func TestApplyIssueApprove_ChildEpicGoesToQueued(t *testing.T) {
	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	store.CreateProject(&core.Project{ID: "proj-1", Name: "svc", RepoPath: t.TempDir()})
	// Child issue with epic template but has ParentID — should NOT decompose again
	store.CreateIssue(&core.Issue{
		ID: "child-1", ProjectID: "proj-1", Title: "Sub-task",
		Template: "standard", ParentID: "epic-parent",
		State: core.IssueStateOpen, Status: core.IssueStatusReviewing,
	})

	mgr, err := NewManager(store, nil, nil, &fakeManagerScheduler{})
	if err != nil {
		t.Fatal(err)
	}

	issue, _ := store.GetIssue("child-1")
	result, err := mgr.applyIssueApprove(context.Background(), issue, "")
	if err != nil {
		t.Fatalf("applyIssueApprove: %v", err)
	}
	if result.Status != core.IssueStatusQueued {
		t.Errorf("status = %q, want queued (child should not decompose)", result.Status)
	}
}

func TestApplyIssueApprove_DecomposeLabelTriggersDecomposition(t *testing.T) {
	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	store.CreateProject(&core.Project{ID: "proj-1", Name: "svc", RepoPath: t.TempDir()})
	store.CreateIssue(&core.Issue{
		ID: "labeled-1", ProjectID: "proj-1", Title: "Big task",
		Template: "standard", Labels: []string{"decompose"},
		State: core.IssueStateOpen, Status: core.IssueStatusReviewing,
	})

	var published []core.Event
	mgr, err := NewManager(store, nil, nil, &fakeManagerScheduler{},
		WithEventPublisher(&capturePublisher{events: &published}))
	if err != nil {
		t.Fatal(err)
	}

	issue, _ := store.GetIssue("labeled-1")
	result, err := mgr.applyIssueApprove(context.Background(), issue, "")
	if err != nil {
		t.Fatalf("applyIssueApprove: %v", err)
	}
	if result.Status != core.IssueStatusDecomposing {
		t.Errorf("status = %q, want decomposing", result.Status)
	}
}

// --- DecomposeHandler tests ---

func TestDecomposeHandler_CreatesChildIssues(t *testing.T) {
	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	store.CreateProject(&core.Project{ID: "proj-1", Name: "svc", RepoPath: t.TempDir()})
	store.CreateIssue(&core.Issue{
		ID: "epic-1", ProjectID: "proj-1", SessionID: "",
		Title: "Build feature", Template: "epic",
		State: core.IssueStateOpen, Status: core.IssueStatusDecomposing,
		AutoMerge: true, FailPolicy: core.FailBlock,
	})

	var published []core.Event
	pub := &capturePublisher{events: &published}

	handler := NewDecomposeHandler(store, pub, func(_ context.Context, parent *core.Issue) ([]DecomposeSpec, error) {
		return []DecomposeSpec{
			{Title: "Backend API", Body: "Implement REST endpoints", Template: "standard", Priority: 1},
			{Title: "Frontend UI", Body: "Build React components", Template: "standard", Priority: 2},
		}, nil
	})

	handler.OnEvent(context.Background(), core.Event{
		Type:    core.EventIssueDecomposing,
		IssueID: "epic-1",
	})

	// Parent should be decomposed
	parent, _ := store.GetIssue("epic-1")
	if parent.Status != core.IssueStatusDecomposed {
		t.Errorf("parent status = %q, want decomposed", parent.Status)
	}

	// Children should exist
	children, err := store.GetChildIssues("epic-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 {
		t.Fatalf("children = %d, want 2", len(children))
	}
	if children[0].ParentID != "epic-1" {
		t.Errorf("child[0].ParentID = %q, want epic-1", children[0].ParentID)
	}
	if children[0].Status != core.IssueStatusDraft {
		t.Errorf("child[0].Status = %q, want draft", children[0].Status)
	}
	if children[0].AutoMerge != true {
		t.Error("child should inherit parent's AutoMerge")
	}

	// EventIssueDecomposed should be published
	found := false
	for _, evt := range published {
		if evt.Type == core.EventIssueDecomposed && evt.IssueID == "epic-1" {
			found = true
		}
	}
	if !found {
		t.Error("EventIssueDecomposed not published")
	}
}

func TestDecomposeHandler_AutoSubmitsChildrenForReview(t *testing.T) {
	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	store.CreateProject(&core.Project{ID: "proj-1", Name: "svc", RepoPath: t.TempDir()})
	store.CreateIssue(&core.Issue{
		ID: "epic-review", ProjectID: "proj-1",
		Title: "Reviewable epic", Template: "epic",
		State: core.IssueStateOpen, Status: core.IssueStatusDecomposing,
		AutoMerge: true, FailPolicy: core.FailBlock,
	})

	var published []core.Event
	reviewer := &fakeDecomposeReviewer{}

	handler := NewDecomposeHandler(store, &capturePublisher{events: &published}, func(_ context.Context, _ *core.Issue) ([]DecomposeSpec, error) {
		return []DecomposeSpec{
			{Title: "Task A", Body: "do A", Template: "standard", Priority: 1},
			{Title: "Task B", Body: "do B", Template: "standard", Priority: 2},
		}, nil
	})
	handler.SetReviewSubmitter(reviewer)

	handler.OnEvent(context.Background(), core.Event{
		Type:    core.EventIssueDecomposing,
		IssueID: "epic-review",
	})

	if reviewer.calls != 1 {
		t.Errorf("reviewer.SubmitForReview calls = %d, want 1", reviewer.calls)
	}
	if len(reviewer.lastIDs) != 2 {
		t.Errorf("reviewer submitted %d issue IDs, want 2", len(reviewer.lastIDs))
	}
}

func TestDecomposeHandler_AgentFailureMarksParentFailed(t *testing.T) {
	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	store.CreateProject(&core.Project{ID: "proj-1", Name: "svc", RepoPath: t.TempDir()})
	store.CreateIssue(&core.Issue{
		ID: "epic-2", ProjectID: "proj-1", Title: "Doomed epic",
		Template: "epic", State: core.IssueStateOpen, Status: core.IssueStatusDecomposing,
	})

	var published []core.Event
	handler := NewDecomposeHandler(store, &capturePublisher{events: &published},
		func(_ context.Context, _ *core.Issue) ([]DecomposeSpec, error) {
			return nil, errors.New("LLM context overflow")
		})

	handler.OnEvent(context.Background(), core.Event{
		Type:    core.EventIssueDecomposing,
		IssueID: "epic-2",
	})

	parent, _ := store.GetIssue("epic-2")
	if parent.Status != core.IssueStatusFailed {
		t.Errorf("parent status = %q, want failed", parent.Status)
	}

	found := false
	for _, evt := range published {
		if evt.Type == core.EventIssueFailed && evt.IssueID == "epic-2" {
			found = true
		}
	}
	if !found {
		t.Error("EventIssueFailed not published on decompose failure")
	}
}

func TestDecomposeHandler_ZeroSpecsMarksParentFailed(t *testing.T) {
	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	store.CreateProject(&core.Project{ID: "proj-1", Name: "svc", RepoPath: t.TempDir()})
	store.CreateIssue(&core.Issue{
		ID: "epic-3", ProjectID: "proj-1", Title: "Empty epic",
		Template: "epic", State: core.IssueStateOpen, Status: core.IssueStatusDecomposing,
	})

	handler := NewDecomposeHandler(store, &capturePublisher{events: &[]core.Event{}},
		func(_ context.Context, _ *core.Issue) ([]DecomposeSpec, error) {
			return nil, nil
		})

	handler.OnEvent(context.Background(), core.Event{
		Type:    core.EventIssueDecomposing,
		IssueID: "epic-3",
	})

	parent, _ := store.GetIssue("epic-3")
	if parent.Status != core.IssueStatusFailed {
		t.Errorf("parent status = %q, want failed", parent.Status)
	}
}

func TestDecomposeHandler_IgnoresNonDecomposingIssue(t *testing.T) {
	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	store.CreateProject(&core.Project{ID: "proj-1", Name: "svc", RepoPath: t.TempDir()})
	store.CreateIssue(&core.Issue{
		ID: "epic-4", ProjectID: "proj-1", Title: "Already done",
		Template: "epic", State: core.IssueStateOpen, Status: core.IssueStatusDone,
	})

	called := false
	handler := NewDecomposeHandler(store, &capturePublisher{events: &[]core.Event{}},
		func(_ context.Context, _ *core.Issue) ([]DecomposeSpec, error) {
			called = true
			return nil, nil
		})

	handler.OnEvent(context.Background(), core.Event{
		Type:    core.EventIssueDecomposing,
		IssueID: "epic-4",
	})

	if called {
		t.Error("decompose func should not be called for non-decomposing issue")
	}
}

// --- NeedsDecomposition unit tests ---

func TestNeedsDecomposition(t *testing.T) {
	tests := []struct {
		name     string
		issue    core.Issue
		expected bool
	}{
		{"epic template", core.Issue{Template: "epic"}, true},
		{"standard template", core.Issue{Template: "standard"}, false},
		{"decompose label", core.Issue{Template: "standard", Labels: []string{"decompose"}}, true},
		{"DECOMPOSE label", core.Issue{Template: "standard", Labels: []string{" Decompose "}}, true},
		{"empty", core.Issue{Template: "standard"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.issue.NeedsDecomposition(); got != tt.expected {
				t.Errorf("NeedsDecomposition() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// --- test helpers ---

type capturePublisher struct {
	events *[]core.Event
}

func (p *capturePublisher) Publish(_ context.Context, evt core.Event) error {
	*p.events = append(*p.events, evt)
	return nil
}

func (p *capturePublisher) Subscribe(_ ...core.SubOption) (*core.Subscription, error) {
	return &core.Subscription{}, nil
}

func (p *capturePublisher) Close() error {
	return nil
}

type fakeDecomposeReviewer struct {
	calls   int
	lastIDs []string
}

func (r *fakeDecomposeReviewer) SubmitForReview(_ context.Context, issueIDs []string) error {
	r.calls++
	r.lastIDs = issueIDs
	return nil
}
