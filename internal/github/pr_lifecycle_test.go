package github

import (
	"context"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func TestPRLifecycle_ImplementComplete_CreatesDraftPR(t *testing.T) {
	store := newPRLifecycleTestStore(t)
	defer store.Close()

	projectID := seedPRLifecycleProject(t, store)
	Run := seedPRLifecycleRun(t, store, projectID, "pipe-pr-create", map[string]any{
		"base_branch": "main",
	})

	scm := &fakePRLifecycleSCM{
		createPRURL: "https://github.com/acme/ai-workflow/pull/321",
	}
	lifecycle := NewPRLifecycle(store, scm)

	prURL, err := lifecycle.OnImplementComplete(context.Background(), Run.ID)
	if err != nil {
		t.Fatalf("OnImplementComplete() error = %v", err)
	}
	if prURL != scm.createPRURL {
		t.Fatalf("expected pr url %q, got %q", scm.createPRURL, prURL)
	}
	if scm.createCalls != 1 {
		t.Fatalf("expected CreatePR called once, got %d", scm.createCalls)
	}
	if !scm.DraftValue() {
		t.Fatalf("expected draft PR creation")
	}

	updated, err := store.GetRun(Run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got := readIntConfigValue(updated.Config, "pr_number"); got != 321 {
		t.Fatalf("expected pr_number=321, got %d", got)
	}
}

func TestPRLifecycle_MergeApproved_ConvertReadyThenMerge(t *testing.T) {
	store := newPRLifecycleTestStore(t)
	defer store.Close()

	projectID := seedPRLifecycleProject(t, store)
	Run := seedPRLifecycleRun(t, store, projectID, "pipe-pr-merge", map[string]any{
		"pr_number": 910,
	})

	scm := &fakePRLifecycleSCM{}
	lifecycle := NewPRLifecycle(store, scm)

	if err := lifecycle.OnMergeApproved(context.Background(), Run.ID); err != nil {
		t.Fatalf("OnMergeApproved() error = %v", err)
	}

	if len(scm.callOrder) != 2 {
		t.Fatalf("expected 2 scm calls, got %d", len(scm.callOrder))
	}
	if scm.callOrder[0] != "convert_ready" || scm.callOrder[1] != "merge_pr" {
		t.Fatalf("unexpected scm call order: %#v", scm.callOrder)
	}
	if scm.lastMergeReq.Number != 910 {
		t.Fatalf("expected merge pr number 910, got %d", scm.lastMergeReq.Number)
	}
}

func TestPRLifecycle_PullRequestClosedMerged_RunDone(t *testing.T) {
	store := newPRLifecycleTestStore(t)
	defer store.Close()

	projectID := seedPRLifecycleProject(t, store)
	Run := seedPRLifecycleRun(t, store, projectID, "pipe-pr-closed-merged", map[string]any{
		"pr_number": 501,
	})
	Run.Status = core.StatusRunning
	if err := store.SaveRun(Run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	lifecycle := NewPRLifecycle(store, &fakePRLifecycleSCM{})
	if err := lifecycle.OnPullRequestClosed(context.Background(), projectID, 501, true); err != nil {
		t.Fatalf("OnPullRequestClosed() error = %v", err)
	}

	updated, err := store.GetRun(Run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if updated.Status != core.StatusDone {
		t.Fatalf("expected Run done, got %s", updated.Status)
	}
	if updated.FinishedAt.IsZero() {
		t.Fatal("expected finished_at to be set")
	}
}

func TestPRLifecycle_PullRequestClosedNotMerged_RunFailed(t *testing.T) {
	store := newPRLifecycleTestStore(t)
	defer store.Close()

	projectID := seedPRLifecycleProject(t, store)
	Run := seedPRLifecycleRun(t, store, projectID, "pipe-pr-closed-unmerged", map[string]any{
		"pr_number": 777,
	})
	Run.Status = core.StatusRunning
	if err := store.SaveRun(Run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	lifecycle := NewPRLifecycle(store, &fakePRLifecycleSCM{})
	if err := lifecycle.OnPullRequestClosed(context.Background(), projectID, 777, false); err != nil {
		t.Fatalf("OnPullRequestClosed() error = %v", err)
	}

	updated, err := store.GetRun(Run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if updated.Status != core.StatusFailed {
		t.Fatalf("expected Run failed, got %s", updated.Status)
	}
	if updated.ErrorMessage == "" {
		t.Fatal("expected failure message to be recorded")
	}
}

func newPRLifecycleTestStore(t *testing.T) *storesqlite.SQLiteStore {
	t.Helper()
	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	return store
}

func seedPRLifecycleProject(t *testing.T, store core.Store) string {
	t.Helper()
	project := &core.Project{
		ID:       "proj-pr-lifecycle",
		Name:     "proj-pr-lifecycle",
		RepoPath: t.TempDir(),
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	return project.ID
}

func seedPRLifecycleRun(
	t *testing.T,
	store core.Store,
	projectID string,
	id string,
	config map[string]any,
) *core.Run {
	t.Helper()
	if config == nil {
		config = map[string]any{}
	}
	Run := &core.Run{
		ID:              id,
		ProjectID:       projectID,
		Name:            id,
		Description:     "Run for pr lifecycle",
		Template:        "standard",
		Status:          core.StatusRunning,
		CurrentStage:    core.StageImplement,
		BranchName:      "ai-flow/" + id,
		Stages:          []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		Artifacts:       map[string]string{},
		Config:          config,
		MaxTotalRetries: 5,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := store.SaveRun(Run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}
	return Run
}

func readIntConfigValue(config map[string]any, key string) int {
	if config == nil {
		return 0
	}
	raw, ok := config[key]
	if !ok {
		return 0
	}
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

type fakePRLifecycleSCM struct {
	createPRURL   string
	createCalls   int
	lastCreateReq core.PullRequest
	lastMergeReq  core.PullRequestMerge
	callOrder     []string
}

func (f *fakePRLifecycleSCM) CreatePR(_ context.Context, req core.PullRequest) (string, error) {
	f.createCalls++
	f.lastCreateReq = req
	f.callOrder = append(f.callOrder, "create_pr")
	return f.createPRURL, nil
}

func (f *fakePRLifecycleSCM) ConvertToReady(_ context.Context, _ int) error {
	f.callOrder = append(f.callOrder, "convert_ready")
	return nil
}

func (f *fakePRLifecycleSCM) MergePR(_ context.Context, req core.PullRequestMerge) error {
	f.lastMergeReq = req
	f.callOrder = append(f.callOrder, "merge_pr")
	return nil
}

func (f *fakePRLifecycleSCM) DraftValue() bool {
	if f.lastCreateReq.Draft == nil {
		return false
	}
	return *f.lastCreateReq.Draft
}
