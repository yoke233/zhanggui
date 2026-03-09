package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestIssueTimeline_ReturnsTaskSteps(t *testing.T) {
	store := newTestStore(t)
	issue := seedAdminIssueFixture(t, store, "pipe-issue-timeline", core.IssueStatusDraft)

	if _, err := store.SaveTaskStep(&core.TaskStep{
		ID:        core.NewTaskStepID(),
		IssueID:   issue.ID,
		RunID:     issue.RunID,
		Action:    core.StepSubmittedForReview,
		AgentID:   "system",
		Note:      "submitted",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveTaskStep() error = %v", err)
	}

	srv := NewServer(Config{Store: store, Token: "test-token"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/projects/"+issue.ProjectID+"/issues/"+issue.ID+"/timeline", nil)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET timeline failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Steps []core.TaskStep `json:"steps"`
		Total int             `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.Total != 1 || len(payload.Steps) != 1 {
		t.Fatalf("unexpected payload: total=%d len=%d", payload.Total, len(payload.Steps))
	}
	if payload.Steps[0].Action != core.StepSubmittedForReview {
		t.Fatalf("step action = %s, want %s", payload.Steps[0].Action, core.StepSubmittedForReview)
	}
}
