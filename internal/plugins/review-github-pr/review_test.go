package reviewgithubpr

import (
	"context"
	"testing"

	ghapi "github.com/google/go-github/v68/github"
	"github.com/user/ai-workflow/internal/core"
	ghsvc "github.com/user/ai-workflow/internal/github"
	storesqlite "github.com/user/ai-workflow/internal/plugins/store-sqlite"
)

func TestGitHubPRReview_Submit_CreatesReviewPR(t *testing.T) {
	store := newReviewGitHubStore(t)
	defer store.Close()

	client := &fakePRClient{
		createPR: &ghapi.PullRequest{
			Number:  ghapi.Int(77),
			HTMLURL: ghapi.String("https://github.com/acme/ai-workflow/pull/77"),
		},
	}

	gate := New(store, client)
	if err := gate.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	plan := &core.TaskPlan{ID: "plan-gh-review-1", Name: "plan-gh-review-1"}
	seedReviewPlan(t, store, plan.ID)
	reviewID, err := gate.Submit(context.Background(), plan)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if reviewID != plan.ID {
		t.Fatalf("Submit() reviewID = %q, want %q", reviewID, plan.ID)
	}
	if client.createCalls != 1 {
		t.Fatalf("expected CreatePR called once, got %d", client.createCalls)
	}

	records, err := store.GetReviewRecords(plan.ID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one review record, got %d", len(records))
	}
	if records[0].Verdict != "pending" {
		t.Fatalf("expected pending verdict, got %q", records[0].Verdict)
	}
}

func TestGitHubPRReview_Check_MapsReviewStates(t *testing.T) {
	store := newReviewGitHubStore(t)
	defer store.Close()

	gate := New(store, nil)
	if err := gate.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	planID := "plan-gh-review-2"
	seedReviewPlan(t, store, planID)
	seedReviewRecord(t, store, core.ReviewRecord{
		PlanID:   planID,
		Round:    1,
		Reviewer: reviewerName,
		Verdict:  "changes_requested",
	})

	result, err := gate.Check(context.Background(), planID)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Status != "changes_requested" {
		t.Fatalf("expected status changes_requested, got %q", result.Status)
	}
	if result.Decision != "fix" {
		t.Fatalf("expected decision fix, got %q", result.Decision)
	}
}

func TestGitHubPRReview_Cancel_ClosesPR(t *testing.T) {
	store := newReviewGitHubStore(t)
	defer store.Close()

	client := &fakePRClient{}
	gate := New(store, client)
	if err := gate.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	planID := "plan-gh-review-3"
	seedReviewPlan(t, store, planID)
	seedReviewRecord(t, store, core.ReviewRecord{
		PlanID:   planID,
		Round:    1,
		Reviewer: reviewerName,
		Verdict:  "pending",
		Fixes: []core.ProposedFix{
			{
				Description: "pr_number",
				Suggestion:  "88",
			},
		},
	})

	if err := gate.Cancel(context.Background(), planID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if client.updateCalls != 1 {
		t.Fatalf("expected UpdatePR called once, got %d", client.updateCalls)
	}
	if client.lastUpdateNumber != 88 {
		t.Fatalf("expected close pr number 88, got %d", client.lastUpdateNumber)
	}

	records, err := store.GetReviewRecords(planID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected cancel record appended, got %d records", len(records))
	}
	latest := records[len(records)-1]
	if latest.Verdict != "cancelled" {
		t.Fatalf("expected cancelled verdict, got %q", latest.Verdict)
	}
}

func newReviewGitHubStore(t *testing.T) *storesqlite.SQLiteStore {
	t.Helper()
	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	return store
}

func seedReviewRecord(t *testing.T, store core.Store, record core.ReviewRecord) {
	t.Helper()
	if err := store.SaveReviewRecord(&record); err != nil {
		t.Fatalf("SaveReviewRecord() error = %v", err)
	}
}

func seedReviewPlan(t *testing.T, store core.Store, planID string) {
	t.Helper()
	projectID := "proj-" + planID
	if err := store.CreateProject(&core.Project{
		ID:       projectID,
		Name:     projectID,
		RepoPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := store.SaveTaskPlan(&core.TaskPlan{
		ID:         planID,
		ProjectID:  projectID,
		Name:       planID,
		Status:     core.PlanReviewing,
		FailPolicy: core.FailBlock,
	}); err != nil {
		t.Fatalf("SaveTaskPlan() error = %v", err)
	}
}

type fakePRClient struct {
	createPR *ghapi.PullRequest

	createCalls      int
	updateCalls      int
	lastUpdateNumber int
	lastUpdateInput  ghsvc.UpdatePRInput
}

func (f *fakePRClient) CreatePR(context.Context, ghsvc.CreatePRInput) (*ghapi.PullRequest, error) {
	f.createCalls++
	if f.createPR == nil {
		return &ghapi.PullRequest{}, nil
	}
	return f.createPR, nil
}

func (f *fakePRClient) UpdatePR(_ context.Context, number int, input ghsvc.UpdatePRInput) (*ghapi.PullRequest, error) {
	f.updateCalls++
	f.lastUpdateNumber = number
	f.lastUpdateInput = input
	return &ghapi.PullRequest{}, nil
}
