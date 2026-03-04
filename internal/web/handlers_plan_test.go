package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestV2IssueAPIAvailable(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-v2-issue",
		Name:     "v2-issue",
		RepoPath: filepath.Join(t.TempDir(), "repo-v2-issue"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := store.CreateChatSession(&core.ChatSession{
		ID:        "chat-v2-issue-1",
		ProjectID: project.ID,
		Messages:  []core.ChatMessage{},
	}); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}

	issue := core.Issue{
		ID:         "issue-v2-1",
		ProjectID:  project.ID,
		SessionID:  "chat-v2-issue-1",
		Title:      "v2 issue",
		Body:       "v2 issue body",
		Template:   "standard",
		State:      core.IssueStateOpen,
		Status:     core.IssueStatusDraft,
		FailPolicy: core.FailBlock,
	}
	if err := store.CreateIssue(&issue); err != nil {
		t.Fatalf("seed issue: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	getResp, err := http.Get(ts.URL + "/api/v2/issues/" + issue.ID)
	if err != nil {
		t.Fatalf("GET /api/v2/issues/{id}: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /api/v2/issues/{id}, got %d", getResp.StatusCode)
	}

	var gotIssue core.Issue
	if err := json.NewDecoder(getResp.Body).Decode(&gotIssue); err != nil {
		t.Fatalf("decode issue response: %v", err)
	}
	if gotIssue.ID != issue.ID {
		t.Fatalf("issue id = %q, want %q", gotIssue.ID, issue.ID)
	}

	listResp, err := http.Get(ts.URL + "/api/v2/issues?project_id=" + project.ID + "&limit=10&offset=0")
	if err != nil {
		t.Fatalf("GET /api/v2/issues: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /api/v2/issues, got %d", listResp.StatusCode)
	}

	var listed issueListResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode issue list response: %v", err)
	}
	if listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != issue.ID {
		t.Fatalf("unexpected list response: %#v", listed)
	}
}

func TestV2WorkflowProfileAPIAvailable(t *testing.T) {
	store := newTestStore(t)
	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	listResp, err := http.Get(ts.URL + "/api/v2/workflow-profiles")
	if err != nil {
		t.Fatalf("GET /api/v2/workflow-profiles: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	var listed workflowProfileListResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode workflow profile list: %v", err)
	}
	if len(listed.Items) != 3 {
		t.Fatalf("workflow profile count = %d, want 3", len(listed.Items))
	}

	getResp, err := http.Get(ts.URL + "/api/v2/workflow-profiles/strict")
	if err != nil {
		t.Fatalf("GET /api/v2/workflow-profiles/{type}: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	var strict workflowProfileDescriptor
	if err := json.NewDecoder(getResp.Body).Decode(&strict); err != nil {
		t.Fatalf("decode strict workflow profile: %v", err)
	}
	if strict.Type != core.WorkflowProfileStrict {
		t.Fatalf("profile type = %q, want %q", strict.Type, core.WorkflowProfileStrict)
	}
}

func TestV2RunAPIAvailable(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-v2-run",
		Name:     "v2-run",
		RepoPath: filepath.Join(t.TempDir(), "repo-v2-run"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	now := time.Now()
	Run := &core.Run{
		ID:              "run-v2-1",
		ProjectID:       project.ID,
		Name:            "run-v2",
		Template:        "standard",
		Status:          core.StatusInProgress,
		CurrentStage:    core.StageImplement,
		Stages:          []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		Artifacts:       map[string]string{},
		Config:          map[string]any{"workflow_profile": "strict"},
		IssueID:         "issue-v2-run-1",
		MaxTotalRetries: 5,
		CreatedAt:       now,
		UpdatedAt:       now,
		StartedAt:       now,
	}
	if err := store.SaveRun(Run); err != nil {
		t.Fatalf("seed run Run: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	getResp, err := http.Get(ts.URL + "/api/v2/runs/" + Run.ID)
	if err != nil {
		t.Fatalf("GET /api/v2/runs/{id}: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /api/v2/runs/{id}, got %d", getResp.StatusCode)
	}

	var gotRun workflowRunResponse
	if err := json.NewDecoder(getResp.Body).Decode(&gotRun); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	if gotRun.ID != Run.ID {
		t.Fatalf("run id = %q, want %q", gotRun.ID, Run.ID)
	}
	if gotRun.Profile != core.WorkflowProfileStrict {
		t.Fatalf("run profile = %q, want %q", gotRun.Profile, core.WorkflowProfileStrict)
	}
	if gotRun.Status != core.StatusInProgress {
		t.Fatalf("run status = %q, want %q", gotRun.Status, core.StatusInProgress)
	}

	listResp, err := http.Get(ts.URL + "/api/v2/runs?project_id=" + project.ID + "&limit=10&offset=0")
	if err != nil {
		t.Fatalf("GET /api/v2/runs: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /api/v2/runs, got %d", listResp.StatusCode)
	}

	var listed workflowRunListResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode run list response: %v", err)
	}
	if listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != Run.ID {
		t.Fatalf("unexpected run list response: %#v", listed)
	}
}

func TestNoPlansRouteInV1(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-v1-no-plans",
		Name:     "v1-no-plans",
		RepoPath: filepath.Join(t.TempDir(), "repo-v1-no-plans"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/" + project.ID + "/plans")
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/plans: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when plans route removed, got %d", resp.StatusCode)
	}
}
