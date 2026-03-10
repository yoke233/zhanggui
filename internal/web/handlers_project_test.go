package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestProjectsRequiresAuthWhenEnabled(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-auth",
		Name:     "auth-project",
		RepoPath: filepath.Join(t.TempDir(), "repo-auth"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{
		Store: store,
		Token: "secret-token",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects")
	if err != nil {
		t.Fatalf("GET /api/v1/projects without token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if err != nil {
		t.Fatalf("create auth request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/projects with token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d", resp.StatusCode)
	}
}

func TestCreateProjectThenGetProject(t *testing.T) {
	store := newTestStore(t)
	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := map[string]any{
		"name":      "demo-project",
		"repo_path": filepath.Join(t.TempDir(), "repo-demo"),
		"github": map[string]string{
			"owner": "acme",
			"repo":  "ai-workflow",
		},
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	resp, err := http.Post(ts.URL+"/api/v1/projects", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST /api/v1/projects: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created core.Project
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created project: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected created project id")
	}
	if created.Name != "demo-project" {
		t.Fatalf("expected name demo-project, got %s", created.Name)
	}

	getResp, err := http.Get(ts.URL + "/api/v1/projects/" + created.ID)
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{id}: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	var got core.Project
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get project: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected id %s, got %s", created.ID, got.ID)
	}
}

func TestCreateProjectRequestValidation(t *testing.T) {
	store := newTestStore(t)
	provisioner := &stubProjectRepoProvisioner{}
	srv := NewServer(Config{
		Store:                  store,
		ProjectRepoProvisioner: provisioner,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		name     string
		body     map[string]any
		wantCode string
	}{
		{
			name: "missing source type",
			body: map[string]any{
				"name": "proj-1",
			},
			wantCode: "SOURCE_TYPE_REQUIRED",
		},
		{
			name: "invalid source type",
			body: map[string]any{
				"name":        "proj-1",
				"source_type": "random_source",
			},
			wantCode: "INVALID_SOURCE_TYPE",
		},
		{
			name: "local path missing repo path",
			body: map[string]any{
				"name":        "proj-1",
				"source_type": string(projectSourceTypeLocalPath),
			},
			wantCode: "REPO_PATH_REQUIRED",
		},
		{
			name: "local new missing slug and name",
			body: map[string]any{
				"source_type": string(projectSourceTypeLocalNew),
			},
			wantCode: "SLUG_REQUIRED",
		},
		{
			name: "github clone missing remote_url",
			body: map[string]any{
				"name":        "proj-1",
				"source_type": string(projectSourceTypeGitHubClone),
			},
			wantCode: "REMOTE_URL_REQUIRED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			resp, err := http.Post(ts.URL+"/api/v1/projects/create-requests", "application/json", bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("POST create request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
			var out apiError
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatalf("decode error payload: %v", err)
			}
			if out.Code != tc.wantCode {
				t.Fatalf("expected code %s, got %s", tc.wantCode, out.Code)
			}
		})
	}
}

func TestCreateProjectRequestLifecycleSucceeded(t *testing.T) {
	store := newTestStore(t)
	repoPath := filepath.Join(t.TempDir(), "repo-local-path")
	provisioner := &stubProjectRepoProvisioner{
		delay: 80 * time.Millisecond,
	}
	srv := NewServer(Config{
		Store:                  store,
		ProjectRepoProvisioner: provisioner,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := map[string]any{
		"name":        "proj-async-local-path",
		"source_type": string(projectSourceTypeLocalPath),
		"repo_path":   repoPath,
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	resp, err := http.Post(ts.URL+"/api/v1/projects/create-requests", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST create request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	var accepted createProjectRequestAcceptedPayload
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode accepted payload: %v", err)
	}
	if strings.TrimSpace(accepted.RequestID) == "" {
		t.Fatal("expected request_id")
	}
	if accepted.Status != string(projectCreateRequestStatusPending) {
		t.Fatalf("expected pending status, got %s", accepted.Status)
	}

	finalState, seenStatuses := waitForCreateRequestTerminalState(t, ts.URL, accepted.RequestID, 5*time.Second)
	if finalState.Status != string(projectCreateRequestStatusSucceeded) {
		t.Fatalf("expected succeeded status, got %+v", finalState)
	}
	if finalState.Step != "complete" {
		t.Fatalf("expected final step %q, got %q", "complete", finalState.Step)
	}
	if finalState.Progress != 100 {
		t.Fatalf("expected final progress 100, got %d", finalState.Progress)
	}
	if _, ok := seenStatuses[string(projectCreateRequestStatusRunning)]; !ok {
		t.Fatalf("expected to observe running status, seen=%v", seenStatuses)
	}
	if finalState.ProjectID == "" {
		t.Fatal("expected project_id in succeeded state")
	}
	if finalState.RepoPath != repoPath {
		t.Fatalf("expected repo_path %s, got %s", repoPath, finalState.RepoPath)
	}

	getResp, err := http.Get(ts.URL + "/api/v1/projects/" + finalState.ProjectID)
	if err != nil {
		t.Fatalf("GET project: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for created project, got %d", getResp.StatusCode)
	}
	var created core.Project
	if err := json.NewDecoder(getResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created project: %v", err)
	}
	if created.Name != "proj-async-local-path" {
		t.Fatalf("expected project name %s, got %s", "proj-async-local-path", created.Name)
	}
	if created.RepoPath != repoPath {
		t.Fatalf("expected project repo_path %s, got %s", repoPath, created.RepoPath)
	}

	calls := provisioner.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 provisioner call, got %d", len(calls))
	}
	if calls[0].SourceType != string(projectSourceTypeLocalPath) {
		t.Fatalf("expected source type %s, got %s", projectSourceTypeLocalPath, calls[0].SourceType)
	}
	if calls[0].RepoPath != repoPath {
		t.Fatalf("expected repo_path %s in provisioner call, got %s", repoPath, calls[0].RepoPath)
	}
}

func TestCreateProjectRequestGitHubCloneUsesRemoteURL(t *testing.T) {
	store := newTestStore(t)
	repoPath := filepath.Join(t.TempDir(), "repo-github-clone")
	provisioner := &stubProjectRepoProvisioner{
		result: ProjectRepoProvisionResult{
			RepoPath:    repoPath,
			GitHubOwner: "acme",
			GitHubRepo:  "demo-repo",
		},
	}
	srv := NewServer(Config{
		Store:                  store,
		ProjectRepoProvisioner: provisioner,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := map[string]any{
		"source_type": string(projectSourceTypeGitHubClone),
		"remote_url":  "https://github.com/acme/demo-repo.git",
		"ref":         "main",
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	resp, err := http.Post(ts.URL+"/api/v1/projects/create-requests", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST create request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var accepted createProjectRequestAcceptedPayload
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode accepted payload: %v", err)
	}
	finalState, _ := waitForCreateRequestTerminalState(t, ts.URL, accepted.RequestID, 5*time.Second)
	if finalState.Status != string(projectCreateRequestStatusSucceeded) {
		t.Fatalf("expected succeeded status, got %+v", finalState)
	}
	if finalState.Progress != 100 {
		t.Fatalf("expected final progress 100, got %d", finalState.Progress)
	}

	calls := provisioner.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 provisioner call, got %d", len(calls))
	}
	if calls[0].SourceType != string(projectSourceTypeGitHubClone) {
		t.Fatalf("expected source type %s, got %s", projectSourceTypeGitHubClone, calls[0].SourceType)
	}
	if calls[0].RemoteURL != "https://github.com/acme/demo-repo.git" {
		t.Fatalf("expected remote_url %q, got %q", "https://github.com/acme/demo-repo.git", calls[0].RemoteURL)
	}
	if calls[0].Ref != "main" {
		t.Fatalf("expected ref %q, got %q", "main", calls[0].Ref)
	}

	project, err := store.GetProject(finalState.ProjectID)
	if err != nil {
		t.Fatalf("load created project: %v", err)
	}
	if project.Name != "demo-repo" {
		t.Fatalf("expected project name %q, got %q", "demo-repo", project.Name)
	}
	if project.RepoPath != repoPath {
		t.Fatalf("expected repo path %q, got %q", repoPath, project.RepoPath)
	}
}

func TestCreateProjectRequestLifecycleFailed(t *testing.T) {
	store := newTestStore(t)
	provisioner := &stubProjectRepoProvisioner{
		err: errors.New("provision failed"),
	}
	srv := NewServer(Config{
		Store:                  store,
		ProjectRepoProvisioner: provisioner,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := map[string]any{
		"name":        "proj-async-failed",
		"source_type": string(projectSourceTypeLocalPath),
		"repo_path":   filepath.Join(t.TempDir(), "repo-local-path-failed"),
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	resp, err := http.Post(ts.URL+"/api/v1/projects/create-requests", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST create request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	var accepted createProjectRequestAcceptedPayload
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode accepted payload: %v", err)
	}

	finalState, _ := waitForCreateRequestTerminalState(t, ts.URL, accepted.RequestID, 5*time.Second)
	if finalState.Status != string(projectCreateRequestStatusFailed) {
		t.Fatalf("expected failed status, got %+v", finalState)
	}
	if !strings.Contains(finalState.Error, "provision failed") {
		t.Fatalf("expected failure message to contain %q, got %q", "provision failed", finalState.Error)
	}
	if strings.TrimSpace(finalState.ProjectID) != "" {
		t.Fatalf("expected empty project_id for failed request, got %s", finalState.ProjectID)
	}

	projects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		t.Fatalf("list projects after failed create: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected no created projects after failure, got %d", len(projects))
	}
}

func TestGetProjectCreateRequestNotFound(t *testing.T) {
	store := newTestStore(t)
	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/create-requests/not-exists")
	if err != nil {
		t.Fatalf("GET create request by id: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	var out apiError
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if out.Code != "CREATE_REQUEST_NOT_FOUND" {
		t.Fatalf("expected code CREATE_REQUEST_NOT_FOUND, got %s", out.Code)
	}
}

func TestCreateProjectRequestBroadcastsProgressEvents(t *testing.T) {
	store := newTestStore(t)
	provisioner := &stubProjectRepoProvisioner{
		delay: 80 * time.Millisecond,
	}
	hub := NewHub()
	srv := NewServer(Config{
		Store:                  store,
		Hub:                    hub,
		ProjectRepoProvisioner: provisioner,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	if !waitForConnections(hub, 1, time.Second) {
		t.Fatal("ws connection did not register in hub")
	}

	reqBody := map[string]any{
		"name":        "proj-async-events",
		"source_type": string(projectSourceTypeLocalPath),
		"repo_path":   filepath.Join(t.TempDir(), "repo-local-path-events"),
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	resp, err := http.Post(ts.URL+"/api/v1/projects/create-requests", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST create request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	var accepted createProjectRequestAcceptedPayload
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode accepted payload: %v", err)
	}

	gotTypes := map[string]bool{}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		msg := readWSMessage(t, conn, 2*time.Second)
		reqID, _ := msg.Data["request_id"].(string)
		if reqID != accepted.RequestID {
			continue
		}
		sourceType, _ := msg.Data["source_type"].(string)
		if sourceType != string(projectSourceTypeLocalPath) {
			t.Fatalf("expected source_type %s, got %s", projectSourceTypeLocalPath, sourceType)
		}
		step, _ := msg.Data["step"].(string)
		if strings.TrimSpace(step) == "" {
			t.Fatalf("expected non-empty step, msg=%+v", msg)
		}
		message, _ := msg.Data["message"].(string)
		if strings.TrimSpace(message) == "" {
			t.Fatalf("expected non-empty message, msg=%+v", msg)
		}
		gotTypes[msg.Type] = true
		if msg.Type == "project_create_succeeded" {
			break
		}
	}

	if !gotTypes["project_create_started"] {
		t.Fatalf("expected project_create_started event, got=%v", gotTypes)
	}
	if !gotTypes["project_create_progress"] {
		t.Fatalf("expected project_create_progress event, got=%v", gotTypes)
	}
	if !gotTypes["project_create_succeeded"] {
		t.Fatalf("expected project_create_succeeded event, got=%v", gotTypes)
	}
}

type createProjectRequestAcceptedPayload struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

type createProjectRequestStatePayload struct {
	RequestID  string `json:"request_id"`
	SourceType string `json:"source_type"`
	Status     string `json:"status"`
	ProjectID  string `json:"project_id,omitempty"`
	RepoPath   string `json:"repo_path,omitempty"`
	Step       string `json:"step,omitempty"`
	Message    string `json:"message,omitempty"`
	Progress   int    `json:"progress"`
	Error      string `json:"error,omitempty"`
}

func waitForCreateRequestTerminalState(
	t *testing.T,
	baseURL string,
	requestID string,
	timeout time.Duration,
) (createProjectRequestStatePayload, map[string]struct{}) {
	t.Helper()

	seenStatuses := map[string]struct{}{}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/v1/projects/create-requests/" + requestID)
		if err != nil {
			t.Fatalf("GET create request state: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			t.Fatalf("expected 200 while polling create request, got %d", resp.StatusCode)
		}

		var state createProjectRequestStatePayload
		if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
			_ = resp.Body.Close()
			t.Fatalf("decode create request state: %v", err)
		}
		_ = resp.Body.Close()
		seenStatuses[state.Status] = struct{}{}
		if state.Status == string(projectCreateRequestStatusSucceeded) || state.Status == string(projectCreateRequestStatusFailed) {
			return state, seenStatuses
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for create request %s to finish", requestID)
	return createProjectRequestStatePayload{}, seenStatuses
}

type stubProjectRepoProvisioner struct {
	mu     sync.Mutex
	delay  time.Duration
	result ProjectRepoProvisionResult
	err    error
	calls  []ProjectRepoProvisionInput
}

func (s *stubProjectRepoProvisioner) Provision(ctx context.Context, input ProjectRepoProvisionInput) (ProjectRepoProvisionResult, error) {
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return ProjectRepoProvisionResult{}, ctx.Err()
		case <-time.After(s.delay):
		}
	}

	s.mu.Lock()
	s.calls = append(s.calls, input)
	result := s.result
	err := s.err
	s.mu.Unlock()

	if err != nil {
		return ProjectRepoProvisionResult{}, err
	}

	if strings.TrimSpace(result.RepoPath) == "" {
		result.RepoPath = strings.TrimSpace(input.RepoPath)
	}
	return result, nil
}

func (s *stubProjectRepoProvisioner) Calls() []ProjectRepoProvisionInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ProjectRepoProvisionInput, len(s.calls))
	copy(out, s.calls)
	return out
}
