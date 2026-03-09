package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

type stubDecomposePlanner struct {
	planFn func(ctx context.Context, projectID, prompt string) (*core.DecomposeProposal, error)
}

type stubDecomposeBadRequestError struct {
	message string
}

func (e *stubDecomposeBadRequestError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *stubDecomposeBadRequestError) BadRequest() bool {
	return true
}

func (s *stubDecomposePlanner) Plan(ctx context.Context, projectID, prompt string) (*core.DecomposeProposal, error) {
	return s.planFn(ctx, projectID, prompt)
}

type stubProposalIssueCreator struct {
	createIssuesFn         func(ctx context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error)
	confirmCreatedIssuesFn func(ctx context.Context, issueIDs []string, feedback string) ([]*core.Issue, error)
}

func (s *stubProposalIssueCreator) CreateIssues(ctx context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
	return s.createIssuesFn(ctx, input)
}

func (s *stubProposalIssueCreator) ConfirmCreatedIssues(ctx context.Context, issueIDs []string, feedback string) ([]*core.Issue, error) {
	if s.confirmCreatedIssuesFn == nil {
		out := make([]*core.Issue, 0, len(issueIDs))
		for _, issueID := range issueIDs {
			out = append(out, &core.Issue{ID: issueID})
		}
		return out, nil
	}
	return s.confirmCreatedIssuesFn(ctx, issueIDs, feedback)
}

type stubManagerScheduler struct{}

func (s *stubManagerScheduler) Start(_ context.Context) error { return nil }

func (s *stubManagerScheduler) Stop(_ context.Context) error { return nil }

func (s *stubManagerScheduler) RecoverExecutingIssues(_ context.Context) error { return nil }

func (s *stubManagerScheduler) StartIssue(_ context.Context, _ *core.Issue) error { return nil }

type stubReviewSubmitter struct{}

func (s *stubReviewSubmitter) Submit(_ context.Context, _ []*core.Issue) error { return nil }

func persistStubCreatedIssues(t *testing.T, store core.Store, input teamleader.CreateIssuesInput) []*core.Issue {
	t.Helper()
	if strings.TrimSpace(input.SessionID) != "" {
		if _, err := store.GetChatSession(input.SessionID); err != nil {
			if !isNotFoundError(err) {
				t.Fatalf("GetChatSession(%s): %v", input.SessionID, err)
			}
			if err := store.CreateChatSession(&core.ChatSession{
				ID:        input.SessionID,
				ProjectID: input.ProjectID,
				AgentName: "test",
				Messages:  []core.ChatMessage{},
			}); err != nil {
				t.Fatalf("CreateChatSession(%s): %v", input.SessionID, err)
			}
		}
	}
	out := make([]*core.Issue, 0, len(input.Issues))
	for _, spec := range input.Issues {
		issue := &core.Issue{
			ID:           spec.ID,
			ProjectID:    input.ProjectID,
			SessionID:    input.SessionID,
			Title:        spec.Title,
			Body:         spec.Body,
			Labels:       append([]string(nil), spec.Labels...),
			DependsOn:    append([]string(nil), spec.DependsOn...),
			Blocks:       append([]string(nil), spec.Blocks...),
			Template:     "standard",
			AutoMerge:    false,
			ChildrenMode: spec.ChildrenMode,
			State:        core.IssueStateOpen,
			Status:       core.IssueStatusDraft,
			FailPolicy:   core.FailBlock,
		}
		if spec.Template != "" {
			issue.Template = spec.Template
		}
		if spec.AutoMerge != nil {
			issue.AutoMerge = *spec.AutoMerge
		}
		if err := store.CreateIssue(issue); err != nil {
			t.Fatalf("CreateIssue(%s): %v", issue.ID, err)
		}
		out = append(out, issue)
	}
	return out
}

type flakyProposalIssueCreator struct {
	base             ProposalIssueCreator
	failConfirmCalls int
	createCallCount  int
	confirmCallCount int
	lastCreatedInput teamleader.CreateIssuesInput
}

func (f *flakyProposalIssueCreator) CreateIssues(ctx context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
	f.createCallCount++
	f.lastCreatedInput = input
	return f.base.CreateIssues(ctx, input)
}

func (f *flakyProposalIssueCreator) ConfirmCreatedIssues(ctx context.Context, issueIDs []string, feedback string) ([]*core.Issue, error) {
	f.confirmCallCount++
	if f.confirmCallCount <= f.failConfirmCalls {
		return nil, errors.New("temporary confirm failure")
	}
	return f.base.ConfirmCreatedIssues(ctx, issueIDs, feedback)
}

func TestDecomposeAPI_ReturnsProposal(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-decompose", Name: "proj-decompose", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{
		Store: store,
		DecomposePlanner: &stubDecomposePlanner{planFn: func(_ context.Context, projectID, prompt string) (*core.DecomposeProposal, error) {
			if projectID != project.ID {
				t.Fatalf("projectID = %q", projectID)
			}
			if prompt != "?????" {
				t.Fatalf("prompt = %q", prompt)
			}
			return &core.DecomposeProposal{
				ID:        projectID + "-prop",
				ProjectID: projectID,
				Prompt:    prompt,
				Summary:   "??????",
				Items:     []core.ProposalItem{{TempID: "A", Title: "?? schema"}},
			}, nil
		}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{"prompt": "?????"})
	resp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST decompose: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var proposal core.DecomposeProposal
	if err := json.NewDecoder(resp.Body).Decode(&proposal); err != nil {
		t.Fatalf("decode proposal: %v", err)
	}
	if proposal.ProjectID != project.ID {
		t.Fatalf("proposal project_id = %q", proposal.ProjectID)
	}
	if len(proposal.Items) != 1 || proposal.Items[0].TempID != "A" {
		t.Fatalf("proposal items = %#v", proposal.Items)
	}
}

func TestDecomposeAPI_ReturnsBadRequestForPlannerClientError(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-decompose-bad-request", Name: "proj-decompose-bad-request", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{
		Store: store,
		DecomposePlanner: &stubDecomposePlanner{planFn: func(_ context.Context, projectID, prompt string) (*core.DecomposeProposal, error) {
			return nil, &stubDecomposeBadRequestError{message: "prompt is too broad"}
		}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{"prompt": "build everything"})
	resp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST decompose: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if apiErr.Code != "DECOMPOSE_FAILED" {
		t.Fatalf("api error code = %q", apiErr.Code)
	}
}

func TestDecomposeAPI_ReturnsGatewayTimeoutForPlannerTimeout(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-decompose-timeout", Name: "proj-decompose-timeout", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{
		Store: store,
		DecomposePlanner: &stubDecomposePlanner{planFn: func(_ context.Context, projectID, prompt string) (*core.DecomposeProposal, error) {
			return nil, context.DeadlineExceeded
		}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{"prompt": "build signup"})
	resp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST decompose: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusGatewayTimeout)
	}
	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if apiErr.Code != "DECOMPOSE_UPSTREAM_TIMEOUT" {
		t.Fatalf("api error code = %q", apiErr.Code)
	}
}

func TestDecomposeAPI_ReturnsServiceUnavailableForPlannerConfigError(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-decompose-config", Name: "proj-decompose-config", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{
		Store: store,
		DecomposePlanner: &stubDecomposePlanner{planFn: func(_ context.Context, projectID, prompt string) (*core.DecomposeProposal, error) {
			return nil, errors.New("provider credentials are required")
		}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{"prompt": "build signup"})
	resp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST decompose: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if apiErr.Code != "DECOMPOSE_UPSTREAM_UNAVAILABLE" {
		t.Fatalf("api error code = %q", apiErr.Code)
	}
}

func TestConfirmDecomposeAPI_ResolvesDependenciesViaCreator(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-confirm", Name: "proj-confirm", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	creatorCalled := false
	confirmCalled := false
	srv := NewServer(Config{
		Store: store,
		ProposalIssueCreator: &stubProposalIssueCreator{createIssuesFn: func(_ context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
			creatorCalled = true
			if input.ProjectID != project.ID {
				t.Fatalf("projectID = %q", input.ProjectID)
			}
			if input.SessionID != decomposeSessionID("prop-1") {
				t.Fatalf("sessionID = %q", input.SessionID)
			}
			if len(input.Issues) != 2 {
				t.Fatalf("issues len = %d", len(input.Issues))
			}
			if input.Issues[0].ChildrenMode != core.ChildrenModeSequential {
				t.Fatalf("issue A children_mode = %q", input.Issues[0].ChildrenMode)
			}
			if input.Issues[1].ChildrenMode != core.ChildrenModeParallel {
				t.Fatalf("issue B children_mode = %q", input.Issues[1].ChildrenMode)
			}
			if got := input.Issues[0].Blocks; len(got) != 1 || got[0] != "issue-b" {
				t.Fatalf("resolved blocks = %#v", got)
			}
			if got := input.Issues[1].DependsOn; len(got) != 1 || got[0] != "issue-a" {
				t.Fatalf("resolved depends_on = %#v", got)
			}
			return persistStubCreatedIssues(t, store, input), nil
		}, confirmCreatedIssuesFn: func(_ context.Context, issueIDs []string, feedback string) ([]*core.Issue, error) {
			confirmCalled = true
			if feedback != "confirmed from decompose proposal" {
				t.Fatalf("feedback = %q", feedback)
			}
			if len(issueIDs) != 2 || issueIDs[0] != "issue-a" || issueIDs[1] != "issue-b" {
				t.Fatalf("issueIDs = %#v", issueIDs)
			}
			return []*core.Issue{{ID: "issue-a"}, {ID: "issue-b"}}, nil
		}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{
		"proposal_id": "prop-1",
		"issues": []map[string]any{
			{"temp_id": "A", "title": "?? schema", "body": "", "depends_on": []string{}, "labels": []string{}, "children_mode": "sequential"},
			{"temp_id": "B", "title": "?? API", "body": "", "depends_on": []string{"A"}, "labels": []string{}, "children_mode": "parallel"},
		},
		"issue_ids": map[string]string{"A": "issue-a", "B": "issue-b"},
	})
	resp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose/confirm", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d body=%s", resp.StatusCode, http.StatusCreated, string(body))
	}
	if !creatorCalled {
		t.Fatal("expected proposal issue creator to be called")
	}
	if !confirmCalled {
		t.Fatal("expected ConfirmCreatedIssues to be called")
	}
	var body struct {
		CreatedIssues []struct {
			TempID  string `json:"temp_id"`
			IssueID string `json:"issue_id"`
		} `json:"created_issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.CreatedIssues) != 2 || body.CreatedIssues[1].IssueID != "issue-b" {
		t.Fatalf("created issues = %#v", body.CreatedIssues)
	}
}

func TestConfirmDecomposeAPI_PreservesDependenciesWhenIDsAreGenerated(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-generated", Name: "proj-generated", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	creatorCalled := false
	confirmCalled := false
	srv := NewServer(Config{
		Store: store,
		ProposalIssueCreator: &stubProposalIssueCreator{createIssuesFn: func(_ context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
			creatorCalled = true
			if len(input.Issues) != 2 {
				t.Fatalf("issues len = %d", len(input.Issues))
			}
			if input.Issues[0].ID == "" {
				t.Fatal("expected generated id for first issue")
			}
			if got := input.Issues[1].DependsOn; len(got) != 1 || got[0] != input.Issues[0].ID {
				t.Fatalf("resolved depends_on = %#v, want [%q]", got, input.Issues[0].ID)
			}
			return persistStubCreatedIssues(t, store, input), nil
		}, confirmCreatedIssuesFn: func(_ context.Context, issueIDs []string, feedback string) ([]*core.Issue, error) {
			confirmCalled = true
			if feedback != "confirmed from decompose proposal" {
				t.Fatalf("feedback = %q", feedback)
			}
			if len(issueIDs) != 2 {
				t.Fatalf("issueIDs = %#v", issueIDs)
			}
			return []*core.Issue{{ID: issueIDs[0]}, {ID: issueIDs[1]}}, nil
		}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{
		"proposal_id": "prop-2",
		"issues": []map[string]any{
			{"temp_id": "A", "title": "schema", "body": "", "depends_on": []string{}, "labels": []string{}},
			{"temp_id": "B", "title": "api", "body": "", "depends_on": []string{"A"}, "labels": []string{}},
		},
	})
	resp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose/confirm", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d body=%s", resp.StatusCode, http.StatusCreated, string(body))
	}
	if !creatorCalled {
		t.Fatal("expected proposal issue creator to be called")
	}
	if !confirmCalled {
		t.Fatal("expected ConfirmCreatedIssues to be called")
	}

	var body struct {
		CreatedIssues []struct {
			TempID  string `json:"temp_id"`
			IssueID string `json:"issue_id"`
		} `json:"created_issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.CreatedIssues) != 2 {
		t.Fatalf("created issues = %#v", body.CreatedIssues)
	}
	if body.CreatedIssues[0].IssueID == "" || body.CreatedIssues[1].IssueID == "" {
		t.Fatalf("expected generated issue ids, got %#v", body.CreatedIssues)
	}
}

func TestConfirmDecomposeAPI_GeneratesIssueIDsForDependenciesWhenOmitted(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-confirm-generated", Name: "proj-confirm-generated", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	creatorCalled := false
	confirmCalled := false
	srv := NewServer(Config{
		Store: store,
		ProposalIssueCreator: &stubProposalIssueCreator{createIssuesFn: func(_ context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
			creatorCalled = true
			if len(input.Issues) != 2 {
				t.Fatalf("issues len = %d", len(input.Issues))
			}
			if input.Issues[0].ID == "" || input.Issues[1].ID == "" {
				t.Fatalf("expected generated issue ids, got %#v", input.Issues)
			}
			if got := input.Issues[0].Blocks; len(got) != 1 || got[0] != input.Issues[1].ID {
				t.Fatalf("resolved blocks = %#v, want [%q]", got, input.Issues[1].ID)
			}
			if got := input.Issues[1].DependsOn; len(got) != 1 || got[0] != input.Issues[0].ID {
				t.Fatalf("resolved depends_on = %#v, want [%q]", got, input.Issues[0].ID)
			}
			return persistStubCreatedIssues(t, store, input), nil
		}, confirmCreatedIssuesFn: func(_ context.Context, issueIDs []string, feedback string) ([]*core.Issue, error) {
			confirmCalled = true
			if feedback != "confirmed from decompose proposal" {
				t.Fatalf("feedback = %q", feedback)
			}
			if len(issueIDs) != 2 {
				t.Fatalf("issueIDs = %#v", issueIDs)
			}
			return []*core.Issue{{ID: issueIDs[0]}, {ID: issueIDs[1]}}, nil
		}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{
		"proposal_id": "prop-2",
		"issues": []map[string]any{
			{"temp_id": "A", "title": "schema", "body": "", "depends_on": []string{}, "labels": []string{}},
			{"temp_id": "B", "title": "api", "body": "", "depends_on": []string{"A"}, "labels": []string{}},
		},
	})
	resp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose/confirm", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if !creatorCalled {
		t.Fatal("expected proposal issue creator to be called")
	}
	if !confirmCalled {
		t.Fatal("expected ConfirmCreatedIssues to be called")
	}
}

func TestConfirmDecomposeAPI_RejectsUnknownDependencyReference(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-confirm-invalid-dep", Name: "proj-confirm-invalid-dep", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	creatorCalled := false
	srv := NewServer(Config{
		Store: store,
		ProposalIssueCreator: &stubProposalIssueCreator{createIssuesFn: func(_ context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
			creatorCalled = true
			return nil, nil
		}, confirmCreatedIssuesFn: func(_ context.Context, issueIDs []string, feedback string) ([]*core.Issue, error) {
			t.Fatalf("ConfirmCreatedIssues should not be called, got issueIDs=%#v feedback=%q", issueIDs, feedback)
			return nil, nil
		}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{
		"proposal_id": "prop-invalid",
		"issues": []map[string]any{
			{"temp_id": "A", "title": "schema", "body": "", "depends_on": []string{"Z"}, "labels": []string{}},
		},
	})
	resp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose/confirm", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if creatorCalled {
		t.Fatal("proposal issue creator should not be called for invalid dependency")
	}
}

func TestConfirmDecomposeAPI_WritesCreatedAndQueuedTaskSteps(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-confirm-tasksteps", Name: "proj-confirm-tasksteps", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	manager, err := teamleader.NewManager(store, nil, &stubReviewSubmitter{}, &stubManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	srv := NewServer(Config{Store: store, ProposalIssueCreator: manager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{
		"proposal_id": "prop-tasksteps",
		"issues": []map[string]any{
			{"temp_id": "A", "title": "schema", "body": "", "depends_on": []string{}, "labels": []string{}},
			{"temp_id": "B", "title": "api", "body": "", "depends_on": []string{"A"}, "labels": []string{}},
		},
		"issue_ids": map[string]string{"A": "issue-a", "B": "issue-b"},
	})
	resp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose/confirm", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	for _, issueID := range []string{"issue-a", "issue-b"} {
		steps, err := store.ListTaskSteps(issueID)
		if err != nil {
			t.Fatalf("ListTaskSteps(%s): %v", issueID, err)
		}

		createdIndex := -1
		queuedIndex := -1
		for index, step := range steps {
			if step.Action == core.StepCreated && createdIndex < 0 {
				createdIndex = index
			}
			if step.Action == core.StepQueued && queuedIndex < 0 {
				queuedIndex = index
			}
		}
		if createdIndex < 0 {
			t.Fatalf("task steps for %s do not contain %s: %#v", issueID, core.StepCreated, steps)
		}
		if queuedIndex < 0 {
			t.Fatalf("task steps for %s do not contain %s: %#v", issueID, core.StepQueued, steps)
		}
		issue, err := store.GetIssue(issueID)
		if err != nil {
			t.Fatalf("GetIssue(%s): %v", issueID, err)
		}
		if issue.Status != core.IssueStatusQueued {
			t.Fatalf("issue %s status = %s, want %s", issueID, issue.Status, core.IssueStatusQueued)
		}
		if issue.SessionID != decomposeSessionID("prop-tasksteps") {
			t.Fatalf("issue %s session_id = %q", issueID, issue.SessionID)
		}
	}
}

func TestDecomposeAPI_RejectsUnknownProject(t *testing.T) {
	store := newTestStore(t)
	plannerCalled := false
	srv := NewServer(Config{
		Store: store,
		DecomposePlanner: &stubDecomposePlanner{planFn: func(_ context.Context, projectID, prompt string) (*core.DecomposeProposal, error) {
			plannerCalled = true
			return nil, nil
		}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{"prompt": "build signup"})
	resp, err := http.Post(ts.URL+"/api/v1/projects/missing-project/decompose", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST decompose: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if apiErr.Code != "PROJECT_NOT_FOUND" {
		t.Fatalf("api error code = %q", apiErr.Code)
	}
	if plannerCalled {
		t.Fatal("planner should not be called when project is missing")
	}
}

func TestConfirmDecomposeAPI_RejectsUnknownProject(t *testing.T) {
	store := newTestStore(t)
	creatorCalled := false
	srv := NewServer(Config{
		Store: store,
		ProposalIssueCreator: &stubProposalIssueCreator{
			createIssuesFn: func(_ context.Context, input teamleader.CreateIssuesInput) ([]*core.Issue, error) {
				creatorCalled = true
				return nil, nil
			},
		},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{
		"proposal_id": "prop-missing",
		"issues": []map[string]any{
			{"temp_id": "A", "title": "schema", "body": "", "depends_on": []string{}, "labels": []string{}},
		},
	})
	resp, err := http.Post(ts.URL+"/api/v1/projects/missing-project/decompose/confirm", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if apiErr.Code != "PROJECT_NOT_FOUND" {
		t.Fatalf("api error code = %q", apiErr.Code)
	}
	if creatorCalled {
		t.Fatal("creator should not be called when project is missing")
	}
}

func TestConfirmDecomposeAPI_RetryReusesStableIDsAndDoesNotDuplicateIssues(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{ID: "proj-confirm-retry", Name: "proj-confirm-retry", RepoPath: filepath.Join(t.TempDir(), "repo")}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	manager, err := teamleader.NewManager(store, nil, &stubReviewSubmitter{}, &stubManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	creator := &flakyProposalIssueCreator{
		base:             manager,
		failConfirmCalls: 1,
	}
	srv := NewServer(Config{Store: store, ProposalIssueCreator: creator})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, _ := json.Marshal(map[string]any{
		"proposal_id": "prop-retry",
		"issues": []map[string]any{
			{"temp_id": "A", "title": "schema", "body": "", "depends_on": []string{}, "labels": []string{}},
			{"temp_id": "B", "title": "api", "body": "", "depends_on": []string{"A"}, "labels": []string{}},
		},
	})

	firstResp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose/confirm", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("first POST confirm: %v", err)
	}
	if firstResp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(firstResp.Body)
		firstResp.Body.Close()
		t.Fatalf("first status = %d, want %d body=%s", firstResp.StatusCode, http.StatusInternalServerError, string(body))
	}
	firstResp.Body.Close()
	if creator.createCallCount != 1 {
		t.Fatalf("create call count after first attempt = %d, want 1", creator.createCallCount)
	}
	if creator.lastCreatedInput.SessionID != decomposeSessionID("prop-retry") {
		t.Fatalf("session_id = %q", creator.lastCreatedInput.SessionID)
	}

	issues, total, err := store.ListIssues(project.ID, core.IssueFilter{Limit: 20, Offset: 0})
	if err != nil {
		t.Fatalf("ListIssues after first attempt: %v", err)
	}
	if total != 2 || len(issues) != 2 {
		t.Fatalf("issues after first attempt total=%d len=%d", total, len(issues))
	}
	for _, issue := range issues {
		if issue.Status != core.IssueStatusDraft {
			t.Fatalf("issue %s status after first attempt = %s, want draft", issue.ID, issue.Status)
		}
		if issue.SessionID != decomposeSessionID("prop-retry") {
			t.Fatalf("issue %s session_id = %q", issue.ID, issue.SessionID)
		}
	}

	secondResp, err := http.Post(ts.URL+"/api/v1/projects/"+project.ID+"/decompose/confirm", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("second POST confirm: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusCreated {
		t.Fatalf("second status = %d, want %d", secondResp.StatusCode, http.StatusCreated)
	}
	if creator.createCallCount != 1 {
		t.Fatalf("create call count after retry = %d, want still 1", creator.createCallCount)
	}

	var payload confirmDecomposeResponse
	if err := json.NewDecoder(secondResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if len(payload.CreatedIssues) != 2 {
		t.Fatalf("created issues len = %d, want 2", len(payload.CreatedIssues))
	}
	if payload.CreatedIssues[0].IssueID != decomposeIssueID("prop-retry", "A") {
		t.Fatalf("issue A id = %q", payload.CreatedIssues[0].IssueID)
	}
	if payload.CreatedIssues[1].IssueID != decomposeIssueID("prop-retry", "B") {
		t.Fatalf("issue B id = %q", payload.CreatedIssues[1].IssueID)
	}

	issues, total, err = store.ListIssues(project.ID, core.IssueFilter{Limit: 20, Offset: 0})
	if err != nil {
		t.Fatalf("ListIssues after retry: %v", err)
	}
	if total != 2 || len(issues) != 2 {
		t.Fatalf("issues after retry total=%d len=%d", total, len(issues))
	}

	for _, issueID := range []string{
		decomposeIssueID("prop-retry", "A"),
		decomposeIssueID("prop-retry", "B"),
	} {
		issue, err := store.GetIssue(issueID)
		if err != nil {
			t.Fatalf("GetIssue(%s): %v", issueID, err)
		}
		if issue.Status != core.IssueStatusQueued {
			t.Fatalf("issue %s status after retry = %s, want queued", issueID, issue.Status)
		}
	}
}
