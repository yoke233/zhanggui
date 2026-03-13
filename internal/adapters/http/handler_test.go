package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	v2sandbox "github.com/yoke233/ai-workflow/internal/adapters/sandbox"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	chatapp "github.com/yoke233/ai-workflow/internal/application/chat"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

type failingCreateThreadMessageStore struct {
	Store
	err error
}

func (s *failingCreateThreadMessageStore) CreateThreadMessage(context.Context, *core.ThreadMessage) (int64, error) {
	return 0, s.err
}

type failNthCreateStepStore struct {
	Store
	failAt int
	calls  int
	err    error
}

func (s *failNthCreateStepStore) CreateStep(ctx context.Context, step *core.Step) (int64, error) {
	s.calls++
	if s.calls == s.failAt {
		return 0, s.err
	}
	return s.Store.CreateStep(ctx, step)
}

func setupAPI(t *testing.T) (*Handler, *httptest.Server) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	bus := membus.NewBus()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		return nil // noop executor for API tests
	}
	eng := flowapp.New(store, bus, executor, flowapp.WithConcurrency(2))

	h := NewHandler(store, bus, eng, WithSandboxInspector(v2sandbox.NewDefaultSupportInspector(false, "")))
	r := chi.NewRouter()
	h.Register(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	return h, ts
}

func post(ts *httptest.Server, path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	return http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
}

func get(ts *httptest.Server, path string) (*http.Response, error) {
	return http.Get(ts.URL + path)
}

func put(ts *httptest.Server, path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPut, ts.URL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func decodeJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

type stubSandboxController struct {
	report    v2sandbox.SupportReport
	updateErr error
	lastReq   v2sandbox.UpdateRequest
}

func (s *stubSandboxController) Inspect(context.Context) v2sandbox.SupportReport {
	return s.report
}

func (s *stubSandboxController) Update(_ context.Context, req v2sandbox.UpdateRequest) (v2sandbox.SupportReport, error) {
	s.lastReq = req
	if s.updateErr != nil {
		return s.report, s.updateErr
	}
	if req.Enabled != nil {
		s.report.Enabled = *req.Enabled
		if !s.report.Enabled {
			s.report.CurrentProvider = "noop"
			s.report.CurrentSupported = false
		}
	}
	if req.Provider != nil {
		s.report.ConfiguredProvider = *req.Provider
		if s.report.Enabled {
			s.report.CurrentProvider = *req.Provider
			if support, ok := s.report.Providers[*req.Provider]; ok {
				s.report.CurrentSupported = support.Supported && support.Implemented
			}
		}
	}
	return s.report, nil
}

// ---------------------------------------------------------------------------
// Issue CRUD Tests
// ---------------------------------------------------------------------------

func TestAPI_CreateIssue(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := post(ts, "/issues", map[string]any{
		"title":    "test-issue",
		"priority": "medium",
		"metadata": map[string]any{"env": "test"},
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var issue core.Issue
	if err := decodeJSON(resp, &issue); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if issue.Title != "test-issue" {
		t.Fatalf("expected title test-issue, got %s", issue.Title)
	}
	if issue.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if issue.Status != core.IssueOpen {
		t.Fatalf("expected open, got %s", issue.Status)
	}
}

func TestAPI_CreateIssue_AutoBootstrapsSCMFlow(t *testing.T) {
	_, ts := setupAPI(t)

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "branch", "-M", "main")
	runGit(t, repoDir, "remote", "add", "origin", "https://github.com/acme/demo.git")

	projectResp, err := post(ts, "/projects", map[string]any{
		"name": "auto-pr-project",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if projectResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating project, got %d", projectResp.StatusCode)
	}
	var project core.Project
	if err := decodeJSON(projectResp, &project); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	resourceResp, err := post(ts, fmt.Sprintf("/projects/%d/resources", project.ID), map[string]any{
		"kind":  "git",
		"uri":   repoDir,
		"label": "repo",
		"config": map[string]any{
			"provider":        "github",
			"enable_scm_flow": true,
			"base_branch":     "main",
			"merge_method":    "squash",
		},
	})
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if resourceResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating resource, got %d", resourceResp.StatusCode)
	}
	resourceResp.Body.Close()

	issueResp, err := post(ts, "/issues", map[string]any{
		"title":      "auto-issue",
		"priority":   "medium",
		"project_id": project.ID,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if issueResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating issue, got %d", issueResp.StatusCode)
	}
	var issue core.Issue
	if err := decodeJSON(issueResp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}

	stepsResp, err := get(ts, fmt.Sprintf("/issues/%d/steps", issue.ID))
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if stepsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing steps, got %d", stepsResp.StatusCode)
	}
	var steps []*core.Step
	if err := decodeJSON(stepsResp, &steps); err != nil {
		t.Fatalf("decode steps: %v", err)
	}
	if len(steps) != 4 {
		t.Fatalf("expected 4 auto-bootstrapped steps, got %d", len(steps))
	}
	if steps[2].Config["builtin"] != "scm_open_pr" {
		t.Fatalf("expected open_pr builtin=scm_open_pr, got %#v", steps[2].Config["builtin"])
	}
	if steps[3].Config["merge_method"] != "squash" {
		t.Fatalf("expected merge_method=squash, got %#v", steps[3].Config["merge_method"])
	}
}

func TestAPI_CreateIssue_AutoBootstrapsSelectedBindingWhenMultipleSCMReposExist(t *testing.T) {
	_, ts := setupAPI(t)

	repoA := t.TempDir()
	runGit(t, repoA, "init")
	runGit(t, repoA, "branch", "-M", "main")
	runGit(t, repoA, "remote", "add", "origin", "https://github.com/acme/demo-a.git")

	repoB := t.TempDir()
	runGit(t, repoB, "init")
	runGit(t, repoB, "branch", "-M", "main")
	runGit(t, repoB, "remote", "add", "origin", "https://github.com/acme/demo-b.git")

	projectResp, err := post(ts, "/projects", map[string]any{
		"name": "auto-pr-multi-project",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if projectResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating project, got %d", projectResp.StatusCode)
	}
	var project core.Project
	if err := decodeJSON(projectResp, &project); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	createResource := func(label, uri string) core.ResourceBinding {
		resp, err := post(ts, fmt.Sprintf("/projects/%d/resources", project.ID), map[string]any{
			"kind":  "git",
			"uri":   uri,
			"label": label,
			"config": map[string]any{
				"provider":        "github",
				"enable_scm_flow": true,
				"base_branch":     "main",
				"merge_method":    "squash",
			},
		})
		if err != nil {
			t.Fatalf("create resource %s: %v", label, err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 creating resource %s, got %d", label, resp.StatusCode)
		}
		var binding core.ResourceBinding
		if err := decodeJSON(resp, &binding); err != nil {
			t.Fatalf("decode resource %s: %v", label, err)
		}
		return binding
	}

	_ = createResource("repo-a", repoA)
	selected := createResource("repo-b", repoB)

	issueResp, err := post(ts, "/issues", map[string]any{
		"title":               "auto-issue-selected-binding",
		"priority":            "medium",
		"project_id":          project.ID,
		"resource_binding_id": selected.ID,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if issueResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating issue, got %d", issueResp.StatusCode)
	}
	var issue core.Issue
	if err := decodeJSON(issueResp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}

	stepsResp, err := get(ts, fmt.Sprintf("/issues/%d/steps", issue.ID))
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if stepsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing steps, got %d", stepsResp.StatusCode)
	}
	var steps []*core.Step
	if err := decodeJSON(stepsResp, &steps); err != nil {
		t.Fatalf("decode steps: %v", err)
	}
	if len(steps) != 4 {
		t.Fatalf("expected 4 auto-bootstrapped steps, got %d", len(steps))
	}
}

func TestAPI_CreateIssue_DoesNotAutoBootstrapAmbiguousSCMBindings(t *testing.T) {
	_, ts := setupAPI(t)

	repoA := t.TempDir()
	runGit(t, repoA, "init")
	runGit(t, repoA, "branch", "-M", "main")
	runGit(t, repoA, "remote", "add", "origin", "https://github.com/acme/demo-a.git")

	repoB := t.TempDir()
	runGit(t, repoB, "init")
	runGit(t, repoB, "branch", "-M", "main")
	runGit(t, repoB, "remote", "add", "origin", "https://github.com/acme/demo-b.git")

	projectResp, err := post(ts, "/projects", map[string]any{
		"name": "manual-pr-multi-project",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if projectResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating project, got %d", projectResp.StatusCode)
	}
	var project core.Project
	if err := decodeJSON(projectResp, &project); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	for _, repoDir := range []string{repoA, repoB} {
		resp, err := post(ts, fmt.Sprintf("/projects/%d/resources", project.ID), map[string]any{
			"kind":  "git",
			"uri":   repoDir,
			"label": filepath.Base(repoDir),
			"config": map[string]any{
				"provider":        "github",
				"enable_scm_flow": true,
				"base_branch":     "main",
				"merge_method":    "squash",
			},
		})
		if err != nil {
			t.Fatalf("create resource for %s: %v", repoDir, err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 creating resource for %s, got %d", repoDir, resp.StatusCode)
		}
		resp.Body.Close()
	}

	issueResp, err := post(ts, "/issues", map[string]any{
		"title":      "manual-issue-ambiguous-binding",
		"priority":   "medium",
		"project_id": project.ID,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if issueResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating issue, got %d", issueResp.StatusCode)
	}
	var issue core.Issue
	if err := decodeJSON(issueResp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}

	stepsResp, err := get(ts, fmt.Sprintf("/issues/%d/steps", issue.ID))
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if stepsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing steps, got %d", stepsResp.StatusCode)
	}
	var steps []*core.Step
	if err := decodeJSON(stepsResp, &steps); err != nil {
		t.Fatalf("decode steps: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("expected 0 auto-bootstrapped steps for ambiguous bindings, got %d", len(steps))
	}
}

func TestAPI_CreateIssue_DoesNotAutoBootstrapWithoutEnabledSCMFlow(t *testing.T) {
	_, ts := setupAPI(t)

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "branch", "-M", "main")
	runGit(t, repoDir, "remote", "add", "origin", "https://github.com/acme/demo.git")

	projectResp, err := post(ts, "/projects", map[string]any{
		"name": "manual-pr-project",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if projectResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating project, got %d", projectResp.StatusCode)
	}
	var project core.Project
	if err := decodeJSON(projectResp, &project); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	resourceResp, err := post(ts, fmt.Sprintf("/projects/%d/resources", project.ID), map[string]any{
		"kind":  "git",
		"uri":   repoDir,
		"label": "repo",
		"config": map[string]any{
			"provider":     "github",
			"base_branch":  "main",
			"merge_method": "squash",
		},
	})
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if resourceResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating resource, got %d", resourceResp.StatusCode)
	}
	resourceResp.Body.Close()

	issueResp, err := post(ts, "/issues", map[string]any{
		"title":      "manual-issue",
		"priority":   "medium",
		"project_id": project.ID,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if issueResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating issue, got %d", issueResp.StatusCode)
	}
	var issue core.Issue
	if err := decodeJSON(issueResp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}

	stepsResp, err := get(ts, fmt.Sprintf("/issues/%d/steps", issue.ID))
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if stepsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing steps, got %d", stepsResp.StatusCode)
	}
	var steps []*core.Step
	if err := decodeJSON(stepsResp, &steps); err != nil {
		t.Fatalf("decode steps: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("expected 0 auto-bootstrapped steps, got %d", len(steps))
	}
}

func TestAPI_CreateIssue_Validation(t *testing.T) {
	_, ts := setupAPI(t)

	// Missing title.
	resp, _ := post(ts, "/issues", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAPI_GetIssue(t *testing.T) {
	_, ts := setupAPI(t)

	// Create issue.
	resp, _ := post(ts, "/issues", map[string]any{"title": "get-test", "priority": "medium"})
	var created core.Issue
	decodeJSON(resp, &created)

	// Get issue.
	resp, _ = get(ts, fmt.Sprintf("/issues/%d", created.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got core.Issue
	decodeJSON(resp, &got)
	if got.Title != "get-test" {
		t.Fatalf("expected title get-test, got %s", got.Title)
	}
}

func TestAPI_GetIssue_NotFound(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := get(ts, "/issues/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAPI_ListIssues(t *testing.T) {
	_, ts := setupAPI(t)

	post(ts, "/issues", map[string]any{"title": "issue-1", "priority": "medium"})
	resp, _ := post(ts, "/issues", map[string]any{"title": "issue-2", "priority": "medium"})
	var archivedIssue core.Issue
	decodeJSON(resp, &archivedIssue)
	resp, _ = post(ts, fmt.Sprintf("/issues/%d/archive", archivedIssue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 when archiving, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, "/issues")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var issues []*core.Issue
	decodeJSON(resp, &issues)
	if len(issues) != 1 {
		t.Fatalf("expected 1 unarchived issue, got %d", len(issues))
	}

	resp, _ = get(ts, "/issues?archived=true")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 archived list, got %d", resp.StatusCode)
	}
	decodeJSON(resp, &issues)
	if len(issues) != 1 {
		t.Fatalf("expected 1 archived issue, got %d", len(issues))
	}

	resp, _ = get(ts, "/issues?archived=all")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 all list, got %d", resp.StatusCode)
	}
	decodeJSON(resp, &issues)
	if len(issues) != 2 {
		t.Fatalf("expected 2 total issues, got %d", len(issues))
	}
}

func TestAPI_ListIssues_FilterStatus(t *testing.T) {
	_, ts := setupAPI(t)

	post(ts, "/issues", map[string]any{"title": "i1", "priority": "medium"})
	post(ts, "/issues", map[string]any{"title": "i2", "priority": "medium"})

	resp, _ := get(ts, "/issues?status=open")
	var issues []*core.Issue
	decodeJSON(resp, &issues)
	if len(issues) != 2 {
		t.Fatalf("expected 2 open, got %d", len(issues))
	}

	resp, _ = get(ts, "/issues?status=running")
	decodeJSON(resp, &issues)
	if len(issues) != 0 {
		t.Fatalf("expected 0 running, got %d", len(issues))
	}
}

func TestAPI_ArchiveIssue(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/issues", map[string]any{"title": "archive-test", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	resp, _ = post(ts, fmt.Sprintf("/issues/%d/archive", issue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var archivedIssue core.Issue
	decodeJSON(resp, &archivedIssue)
	if archivedIssue.ArchivedAt == nil {
		t.Fatal("expected archived_at to be set")
	}

	resp, _ = post(ts, fmt.Sprintf("/issues/%d/unarchive", issue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on unarchive, got %d", resp.StatusCode)
	}
	var unarchivedIssue core.Issue
	decodeJSON(resp, &unarchivedIssue)
	if unarchivedIssue.ArchivedAt != nil {
		t.Fatal("expected archived_at to be cleared")
	}
}

func TestAPI_ArchiveIssue_RejectsActiveIssue(t *testing.T) {
	h, ts := setupAPI(t)

	resp, _ := post(ts, "/issues", map[string]any{"title": "running-issue", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	if err := h.store.UpdateIssueStatus(context.Background(), issue.ID, core.IssueRunning); err != nil {
		t.Fatalf("set issue running: %v", err)
	}

	resp, _ = post(ts, fmt.Sprintf("/issues/%d/archive", issue.ID), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestAPI_RunIssue_Archived(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/issues", map[string]any{"title": "archived-run", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})

	resp, _ = post(ts, fmt.Sprintf("/issues/%d/archive", issue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 when archiving, got %d", resp.StatusCode)
	}

	resp, _ = post(ts, fmt.Sprintf("/issues/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 running archived issue, got %d", resp.StatusCode)
	}
}

func TestAPI_GetStats_IncludesArchivedIssues(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/issues", map[string]any{"title": "done-issue", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	resp, _ = post(ts, fmt.Sprintf("/issues/%d/archive", issue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 when archiving, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, "/stats")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 stats, got %d", resp.StatusCode)
	}

	var stats struct {
		TotalIssues int `json:"total_issues"`
	}
	decodeJSON(resp, &stats)
	if stats.TotalIssues != 1 {
		t.Fatalf("expected stats to include archived issue, got %d", stats.TotalIssues)
	}
}

// ---------------------------------------------------------------------------
// Step CRUD Tests
// ---------------------------------------------------------------------------

func TestAPI_CreateStep(t *testing.T) {
	_, ts := setupAPI(t)

	// Create issue first.
	resp, _ := post(ts, "/issues", map[string]any{"title": "issue", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	// Create step.
	resp, _ = post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{
		"name":                  "build",
		"type":                  "exec",
		"agent_role":            "worker",
		"required_capabilities": []string{"go"},
		"max_retries":           2,
		"timeout":               "30s",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var step core.Step
	decodeJSON(resp, &step)
	if step.Name != "build" {
		t.Fatalf("expected name build, got %s", step.Name)
	}
	if step.Type != core.StepExec {
		t.Fatalf("expected type exec, got %s", step.Type)
	}
	if step.MaxRetries != 2 {
		t.Fatalf("expected max_retries=2, got %d", step.MaxRetries)
	}
}

func TestAPI_ListSteps(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/issues", map[string]any{"title": "issue", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})
	post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{"name": "B", "type": "gate"})

	resp, _ = get(ts, fmt.Sprintf("/issues/%d/steps", issue.ID))
	var steps []*core.Step
	decodeJSON(resp, &steps)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
}

func TestAPI_GetStep(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/issues", map[string]any{"title": "issue", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	resp, _ = post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})
	var created core.Step
	decodeJSON(resp, &created)

	resp, _ = get(ts, fmt.Sprintf("/steps/%d", created.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var step core.Step
	decodeJSON(resp, &step)
	if step.Name != "A" {
		t.Fatalf("expected A, got %s", step.Name)
	}
}

// ---------------------------------------------------------------------------
// Run & Cancel Issue Tests
// ---------------------------------------------------------------------------

func TestAPI_RunIssue_NoSteps(t *testing.T) {
	_, ts := setupAPI(t)

	// Create issue without any steps.
	resp, _ := post(ts, "/issues", map[string]any{"title": "no-steps", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	// Try to run — should fail with 400.
	resp, _ = post(ts, fmt.Sprintf("/issues/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var errResp map[string]any
	decodeJSON(resp, &errResp)
	if errResp["code"] != "NO_STEPS" {
		t.Fatalf("expected error code NO_STEPS, got %v", errResp["code"])
	}
}

func TestAPI_RunIssue(t *testing.T) {
	_, ts := setupAPI(t)

	// Create issue + step.
	resp, _ := post(ts, "/issues", map[string]any{"title": "run-test", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})

	// Run issue.
	resp, _ = post(ts, fmt.Sprintf("/issues/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Wait for async completion.
	time.Sleep(500 * time.Millisecond)

	// Verify issue is done.
	resp, _ = get(ts, fmt.Sprintf("/issues/%d", issue.ID))
	var done core.Issue
	decodeJSON(resp, &done)
	if done.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", done.Status)
	}
}

func TestAPI_RunIssue_NotOpen(t *testing.T) {
	_, ts := setupAPI(t)

	// Create and run issue.
	resp, _ := post(ts, "/issues", map[string]any{"title": "run-twice", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)
	post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})
	post(ts, fmt.Sprintf("/issues/%d/run", issue.ID), nil)

	// Wait for issue to complete.
	time.Sleep(500 * time.Millisecond)

	// Verify issue is done.
	resp, _ = get(ts, fmt.Sprintf("/issues/%d", issue.ID))
	decodeJSON(resp, &issue)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done after first run, got %s", issue.Status)
	}

	// Try to run again — should fail since it's not open.
	resp, _ = post(ts, fmt.Sprintf("/issues/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestAPI_CancelIssue(t *testing.T) {
	h, ts := setupAPI(t)

	resp, _ := post(ts, "/issues", map[string]any{"title": "cancel-test", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	// Manually set issue to running for cancel test.
	h.store.UpdateIssueStatus(context.Background(), issue.ID, core.IssueRunning)

	resp, _ = post(ts, fmt.Sprintf("/issues/%d/cancel", issue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/issues/%d", issue.ID))
	var cancelled core.Issue
	decodeJSON(resp, &cancelled)
	if cancelled.Status != core.IssueCancelled {
		t.Fatalf("expected cancelled, got %s", cancelled.Status)
	}
}

// ---------------------------------------------------------------------------
// Events Tests
// ---------------------------------------------------------------------------

func TestAPI_ListEvents(t *testing.T) {
	_, ts := setupAPI(t)

	// Create issue + step + run to generate events.
	resp, _ := post(ts, "/issues", map[string]any{"title": "events-test", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)
	post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})
	post(ts, fmt.Sprintf("/issues/%d/run", issue.ID), nil)
	time.Sleep(500 * time.Millisecond)

	// List all events.
	resp, _ = get(ts, "/events")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// List events filtered by issue.
	resp, _ = get(ts, fmt.Sprintf("/issues/%d/events", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for issue events, got %d", resp.StatusCode)
	}
}

func TestAPI_ListEvents_FilterSessionID(t *testing.T) {
	h, ts := setupAPI(t)

	now := time.Now().UTC()
	if _, err := h.store.CreateEvent(context.Background(), &core.Event{
		Type:      core.EventChatOutput,
		Data:      map[string]any{"session_id": "session-a", "type": "agent_message", "content": "hello"},
		Timestamp: now,
	}); err != nil {
		t.Fatalf("create event a: %v", err)
	}
	if _, err := h.store.CreateEvent(context.Background(), &core.Event{
		Type:      core.EventChatOutput,
		Data:      map[string]any{"session_id": "session-b", "type": "agent_message", "content": "world"},
		Timestamp: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create event b: %v", err)
	}

	resp, err := get(ts, "/events?types=chat.output&session_id=session-a")
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var events []core.Event
	if err := decodeJSON(resp, &events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if got, _ := events[0].Data["session_id"].(string); got != "session-a" {
		t.Fatalf("expected session-a, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// WebSocket Test
// ---------------------------------------------------------------------------

func TestAPI_WebSocket(t *testing.T) {
	h, ts := setupAPI(t)

	// Connect WebSocket.
	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Small delay to ensure the server-side subscription is registered.
	time.Sleep(50 * time.Millisecond)

	// Publish an event.
	h.bus.Publish(context.Background(), core.Event{
		Type:      core.EventIssueStarted,
		IssueID:   42,
		Timestamp: time.Now().UTC(),
	})

	// Read event from WebSocket.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ev core.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev.Type != core.EventIssueStarted {
		t.Fatalf("expected issue.started, got %s", ev.Type)
	}
	if ev.IssueID != 42 {
		t.Fatalf("expected issue_id=42, got %d", ev.IssueID)
	}
}

func TestAPI_WebSocket_FilterSessionID(t *testing.T) {
	h, ts := setupAPI(t)

	wsURL := "ws" + ts.URL[4:] + "/ws?types=chat.output&session_id=s-1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	h.bus.Publish(context.Background(), core.Event{
		Type:      core.EventChatOutput,
		Data:      map[string]any{"session_id": "s-2", "type": "agent_message_chunk", "content": "ignored"},
		Timestamp: time.Now().UTC(),
	})
	h.bus.Publish(context.Background(), core.Event{
		Type:      core.EventChatOutput,
		Data:      map[string]any{"session_id": "s-1", "type": "agent_message_chunk", "content": "wanted"},
		Timestamp: time.Now().UTC(),
	})

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ev core.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev.Type != core.EventChatOutput {
		t.Fatalf("expected chat.output, got %s", ev.Type)
	}
	if got, _ := ev.Data["session_id"].(string); got != "s-1" {
		t.Fatalf("expected session_id=s-1, got %q", got)
	}
}

func TestAPI_WebSocket_ChatSend(t *testing.T) {
	h, ts := setupAPI(t)
	lead := &stubLeadChatService{
		startResp: &chatapp.AcceptedResponse{
			SessionID: "session-ws",
			WSPath:    "/api/ws?session_id=session-ws&types=chat.output",
		},
	}
	h.lead = lead

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "chat.send",
		"data": map[string]any{
			"request_id": "req-1",
			"message":    "你好",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
		Data struct {
			RequestID string `json:"request_id"`
			SessionID string `json:"session_id"`
			WSPath    string `json:"ws_path"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "chat.ack" {
		t.Fatalf("ack type = %q, want chat.ack", ack.Type)
	}
	if ack.Data.RequestID != "req-1" || ack.Data.SessionID != "session-ws" {
		t.Fatalf("unexpected ack data: %+v", ack.Data)
	}
	if lead.lastStartReq.Message != "你好" {
		t.Fatalf("message = %q, want 你好", lead.lastStartReq.Message)
	}
}

func TestAPI_GetSandboxSupport(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := get(ts, "/system/sandbox-support")
	if err != nil {
		t.Fatalf("get sandbox support: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got struct {
		OS                 string `json:"os"`
		Arch               string `json:"arch"`
		Enabled            bool   `json:"enabled"`
		ConfiguredProvider string `json:"configured_provider"`
		CurrentProvider    string `json:"current_provider"`
		CurrentSupported   bool   `json:"current_supported"`
		Providers          map[string]struct {
			Supported bool   `json:"supported"`
			Reason    string `json:"reason"`
		} `json:"providers"`
	}
	if err := decodeJSON(resp, &got); err != nil {
		t.Fatalf("decode sandbox support: %v", err)
	}
	if got.OS == "" || got.Arch == "" {
		t.Fatalf("expected os/arch in response, got %#v", got)
	}
	if got.CurrentProvider != "noop" {
		t.Fatalf("current_provider = %q, want noop", got.CurrentProvider)
	}
	if got.ConfiguredProvider != "home_dir" {
		t.Fatalf("configured_provider = %q, want home_dir", got.ConfiguredProvider)
	}
	if got.CurrentSupported {
		t.Fatal("current_supported = true, want false for disabled sandbox")
	}
	if !got.Providers["home_dir"].Supported {
		t.Fatal("home_dir should be reported as supported")
	}
	if _, ok := got.Providers["docker"]; !ok {
		t.Fatal("docker provider should be present in API response")
	}
	if _, ok := got.Providers["boxlite"]; !ok {
		t.Fatal("boxlite provider should be present in API response")
	}
}

func TestAPI_UpdateSandboxSupport(t *testing.T) {
	h, ts := setupAPI(t)
	ctrl := &stubSandboxController{
		report: v2sandbox.SupportReport{
			OS:                 "darwin",
			Arch:               "arm64",
			Enabled:            false,
			ConfiguredProvider: "home_dir",
			CurrentProvider:    "noop",
			CurrentSupported:   false,
			Providers: map[string]v2sandbox.ProviderSupport{
				"home_dir": {Supported: true, Implemented: true, Reason: "ok"},
				"litebox":  {Supported: false, Implemented: true, Reason: "windows only"},
			},
		},
	}
	h.sandbox = ctrl

	resp, err := put(ts, "/admin/system/sandbox-support", map[string]any{
		"enabled":  true,
		"provider": "home_dir",
	})
	if err != nil {
		t.Fatalf("update sandbox support: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got v2sandbox.SupportReport
	if err := decodeJSON(resp, &got); err != nil {
		t.Fatalf("decode sandbox update: %v", err)
	}
	if !got.Enabled || got.CurrentProvider != "home_dir" || !got.CurrentSupported {
		t.Fatalf("unexpected sandbox update response: %#v", got)
	}
	if ctrl.lastReq.Enabled == nil || !*ctrl.lastReq.Enabled {
		t.Fatalf("enabled request not passed through: %#v", ctrl.lastReq)
	}
	if ctrl.lastReq.Provider == nil || *ctrl.lastReq.Provider != "home_dir" {
		t.Fatalf("provider request not passed through: %#v", ctrl.lastReq)
	}
}

func TestAPI_UpdateSandboxSupport_ConfigUnavailable(t *testing.T) {
	h, ts := setupAPI(t)
	h.sandbox = &stubSandboxController{
		report: v2sandbox.SupportReport{
			OS:                 "windows",
			Arch:               "amd64",
			Enabled:            false,
			ConfiguredProvider: "home_dir",
			CurrentProvider:    "noop",
			CurrentSupported:   false,
			Providers: map[string]v2sandbox.ProviderSupport{
				"home_dir": {Supported: true, Implemented: true},
			},
		},
		updateErr: v2sandbox.ErrSandboxConfigUnavailable,
	}

	resp, err := put(ts, "/admin/system/sandbox-support", map[string]any{
		"enabled": true,
	})
	if err != nil {
		t.Fatalf("update sandbox support unavailable: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestAPI_UpdateSandboxSupport_BadRequest(t *testing.T) {
	h, ts := setupAPI(t)
	h.sandbox = &stubSandboxController{
		report: v2sandbox.SupportReport{
			OS:                 "windows",
			Arch:               "amd64",
			Enabled:            false,
			ConfiguredProvider: "home_dir",
			CurrentProvider:    "noop",
			CurrentSupported:   false,
			Providers: map[string]v2sandbox.ProviderSupport{
				"home_dir": {Supported: true, Implemented: true},
			},
		},
		updateErr: errors.New("bad provider"),
	}

	resp, err := put(ts, "/admin/system/sandbox-support", map[string]any{
		"provider": "bad",
	})
	if err != nil {
		t.Fatalf("update sandbox support bad request: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// E2E API Test: Create issue + steps → run → verify all entities
// ---------------------------------------------------------------------------

func TestAPI_E2E_IssueLifecycle(t *testing.T) {
	_, ts := setupAPI(t)

	// 1. Create issue.
	resp, _ := post(ts, "/issues", map[string]any{"title": "e2e-api", "priority": "medium"})
	var issue core.Issue
	decodeJSON(resp, &issue)

	// 2. Create steps: A, B.
	resp, _ = post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{
		"name": "A", "type": "exec",
	})
	var stepA core.Step
	decodeJSON(resp, &stepA)

	resp, _ = post(ts, fmt.Sprintf("/issues/%d/steps", issue.ID), map[string]any{
		"name": "B", "type": "exec",
	})
	var stepB core.Step
	decodeJSON(resp, &stepB)

	// 3. List steps.
	resp, _ = get(ts, fmt.Sprintf("/issues/%d/steps", issue.ID))
	var steps []*core.Step
	decodeJSON(resp, &steps)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	// 4. Run issue.
	resp, _ = post(ts, fmt.Sprintf("/issues/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	time.Sleep(500 * time.Millisecond)

	// 5. Verify issue done.
	resp, _ = get(ts, fmt.Sprintf("/issues/%d", issue.ID))
	decodeJSON(resp, &issue)
	if issue.Status != core.IssueDone {
		t.Fatalf("expected done, got %s", issue.Status)
	}

	// 6. Verify steps done.
	resp, _ = get(ts, fmt.Sprintf("/steps/%d", stepA.ID))
	decodeJSON(resp, &stepA)
	if stepA.Status != core.StepDone {
		t.Fatalf("expected A done, got %s", stepA.Status)
	}

	resp, _ = get(ts, fmt.Sprintf("/steps/%d", stepB.ID))
	decodeJSON(resp, &stepB)
	if stepB.Status != core.StepDone {
		t.Fatalf("expected B done, got %s", stepB.Status)
	}

	// 7. Verify executions exist.
	resp, _ = get(ts, fmt.Sprintf("/steps/%d/executions", stepA.ID))
	var execs []*core.Execution
	decodeJSON(resp, &execs)
	if len(execs) == 0 {
		t.Fatal("expected at least 1 execution for step A")
	}
	if execs[0].Status != core.ExecSucceeded {
		t.Fatalf("expected succeeded, got %s", execs[0].Status)
	}

	// 8. Verify events endpoint works (events are in-memory bus, not persisted yet).
	resp, _ = get(ts, fmt.Sprintf("/issues/%d/events", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for events, got %d", resp.StatusCode)
	}
}
