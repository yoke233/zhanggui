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
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	"github.com/yoke233/ai-workflow/internal/adapters/llmconfig"
	v2sandbox "github.com/yoke233/ai-workflow/internal/adapters/sandbox"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	chatapp "github.com/yoke233/ai-workflow/internal/application/chat"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	inspectionapp "github.com/yoke233/ai-workflow/internal/application/inspection"
	"github.com/yoke233/ai-workflow/internal/audit"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

type failingCreateThreadMessageStore struct {
	Store
	err error
}

func (s *failingCreateThreadMessageStore) CreateThreadMessage(context.Context, *core.ThreadMessage) (int64, error) {
	return 0, s.err
}

type failNthCreateActionStore struct {
	Store
	failAt int
	calls  int
	err    error
}

func (s *failNthCreateActionStore) CreateAction(ctx context.Context, step *core.Action) (int64, error) {
	s.calls++
	if s.calls == s.failAt {
		return 0, s.err
	}
	return s.Store.CreateAction(ctx, step)
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

	executor := func(_ context.Context, step *core.Action, exec *core.Run) error {
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

func setupAPIWithDataDir(t *testing.T, dataDir string) (*Handler, *httptest.Server) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	bus := membus.NewBus()
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error { return nil }
	eng := flowapp.New(store, bus, executor, flowapp.WithConcurrency(2))

	h := NewHandler(store, bus, eng, WithSandboxInspector(v2sandbox.NewDefaultSupportInspector(false, "")), WithDataDir(dataDir))
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

func patch(ts *httptest.Server, path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPatch, ts.URL+path, bytes.NewReader(b))
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

func mustMarshalJSONRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}
	return data
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

type stubLLMConfigController struct {
	report    llmconfig.Report
	updateErr error
	lastReq   llmconfig.UpdateRequest
}

func (s *stubLLMConfigController) Inspect(context.Context) llmconfig.Report {
	return s.report
}

func (s *stubLLMConfigController) Update(_ context.Context, req llmconfig.UpdateRequest) (llmconfig.Report, error) {
	s.lastReq = req
	if s.updateErr != nil {
		return s.report, s.updateErr
	}
	if req.DefaultConfigID != nil {
		s.report.DefaultConfigID = *req.DefaultConfigID
	}
	if req.Configs != nil {
		s.report.Configs = append([]config.RuntimeLLMEntryConfig(nil), (*req.Configs)...)
	}
	return s.report, nil
}

// ---------------------------------------------------------------------------
// Issue CRUD Tests
// ---------------------------------------------------------------------------

func TestAPI_CreateWorkItem(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := post(ts, "/work-items", map[string]any{
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

	var issue core.WorkItem
	if err := decodeJSON(resp, &issue); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if issue.Title != "test-issue" {
		t.Fatalf("expected title test-issue, got %s", issue.Title)
	}
	if issue.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if issue.Status != core.WorkItemOpen {
		t.Fatalf("expected open, got %s", issue.Status)
	}
}

func TestAPI_WorkItemRoutesCRUDAndLifecycle(t *testing.T) {
	h, ts := setupAPI(t)

	resp, err := post(ts, "/work-items", map[string]any{
		"title":    "alias-create",
		"priority": "medium",
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var issue core.WorkItem
	if err := decodeJSON(resp, &issue); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 reading created work item, got %d", resp.StatusCode)
	}
	var fetched core.WorkItem
	decodeJSON(resp, &fetched)
	if fetched.Title != "alias-create" {
		t.Fatalf("expected created work item to be readable, got %q", fetched.Title)
	}

	resp, _ = put(ts, fmt.Sprintf("/work-items/%d", issue.ID), map[string]any{
		"title": "alias-updated",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 updating via /work-items, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 reading via /work-items, got %d", resp.StatusCode)
	}
	var updated core.WorkItem
	decodeJSON(resp, &updated)
	if updated.Title != "alias-updated" {
		t.Fatalf("expected alias-updated, got %q", updated.Title)
	}

	resp, _ = get(ts, "/work-items")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing work-items, got %d", resp.StatusCode)
	}
	var listed []*core.WorkItem
	decodeJSON(resp, &listed)
	if len(listed) != 1 || listed[0].ID != issue.ID {
		t.Fatalf("unexpected work-item list response: %+v", listed)
	}

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "A",
		"type": "exec",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating step via alias, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing steps via legacy route, got %d", resp.StatusCode)
	}
	var steps []core.Action
	decodeJSON(resp, &steps)
	if len(steps) != 1 || steps[0].WorkItemID != issue.ID {
		t.Fatalf("unexpected steps from legacy route: %+v", steps)
	}

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/resources", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing attachments via alias, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/cron", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 getting cron status via alias, got %d", resp.StatusCode)
	}

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/generate-steps", issue.ID), map[string]any{
		"description": "build a REST API",
	})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from generate-steps alias without generator, got %d", resp.StatusCode)
	}

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 running via alias, got %d", resp.StatusCode)
	}
	time.Sleep(500 * time.Millisecond)

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/events", issue.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing events via alias, got %d", resp.StatusCode)
	}
	var events []core.Event
	if err := decodeJSON(resp, &events); err != nil {
		t.Fatalf("decode events: %v", err)
	}

	resp, _ = post(ts, "/work-items", map[string]any{"title": "archive-alias", "priority": "medium"})
	var archiveWorkItem core.WorkItem
	decodeJSON(resp, &archiveWorkItem)

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/archive", archiveWorkItem.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 archiving via alias, got %d", resp.StatusCode)
	}
	var archived core.WorkItem
	decodeJSON(resp, &archived)
	if archived.ArchivedAt == nil {
		t.Fatal("expected archived_at to be set via work-item route")
	}

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/unarchive", archiveWorkItem.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 unarchiving via alias, got %d", resp.StatusCode)
	}
	var unarchived core.WorkItem
	decodeJSON(resp, &unarchived)
	if unarchived.ArchivedAt != nil {
		t.Fatal("expected archived_at to be cleared via work-item route")
	}

	resp, _ = post(ts, "/work-items", map[string]any{"title": "cancel-alias", "priority": "medium"})
	var cancelWorkItem core.WorkItem
	decodeJSON(resp, &cancelWorkItem)
	if err := h.store.UpdateWorkItemStatus(context.Background(), cancelWorkItem.ID, core.WorkItemRunning); err != nil {
		t.Fatalf("set issue running: %v", err)
	}

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/cancel", cancelWorkItem.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 cancelling via alias, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d", cancelWorkItem.ID))
	var cancelled core.WorkItem
	decodeJSON(resp, &cancelled)
	if cancelled.Status != core.WorkItemCancelled {
		t.Fatalf("expected cancelled status via work-item read, got %s", cancelled.Status)
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

	resourceResp, err := post(ts, fmt.Sprintf("/projects/%d/spaces", project.ID), map[string]any{
		"kind":     "git",
		"root_uri": repoDir,
		"label":    "repo",
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

	issueResp, err := post(ts, "/work-items", map[string]any{
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
	var issue core.WorkItem
	if err := decodeJSON(issueResp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}

	stepsResp, err := get(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if stepsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing steps, got %d", stepsResp.StatusCode)
	}
	var steps []*core.Action
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

	createResource := func(label, uri string) core.ResourceSpace {
		resp, err := post(ts, fmt.Sprintf("/projects/%d/spaces", project.ID), map[string]any{
			"kind":     "git",
			"root_uri": uri,
			"label":    label,
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
		var space core.ResourceSpace
		if err := decodeJSON(resp, &space); err != nil {
			t.Fatalf("decode resource %s: %v", label, err)
		}
		return space
	}

	_ = createResource("repo-a", repoA)
	selected := createResource("repo-b", repoB)

	issueResp, err := post(ts, "/work-items", map[string]any{
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
	var issue core.WorkItem
	if err := decodeJSON(issueResp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}

	stepsResp, err := get(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if stepsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing steps, got %d", stepsResp.StatusCode)
	}
	var steps []*core.Action
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
		resp, err := post(ts, fmt.Sprintf("/projects/%d/spaces", project.ID), map[string]any{
			"kind":     "git",
			"root_uri": repoDir,
			"label":    filepath.Base(repoDir),
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

	issueResp, err := post(ts, "/work-items", map[string]any{
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
	var issue core.WorkItem
	if err := decodeJSON(issueResp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}

	stepsResp, err := get(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if stepsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing steps, got %d", stepsResp.StatusCode)
	}
	var steps []*core.Action
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

	resourceResp, err := post(ts, fmt.Sprintf("/projects/%d/spaces", project.ID), map[string]any{
		"kind":     "git",
		"root_uri": repoDir,
		"label":    "repo",
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

	issueResp, err := post(ts, "/work-items", map[string]any{
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
	var issue core.WorkItem
	if err := decodeJSON(issueResp, &issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}

	stepsResp, err := get(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if stepsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing steps, got %d", stepsResp.StatusCode)
	}
	var steps []*core.Action
	if err := decodeJSON(stepsResp, &steps); err != nil {
		t.Fatalf("decode steps: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("expected 0 auto-bootstrapped steps, got %d", len(steps))
	}
}

func TestAPI_AttachmentRoutesRejectNonAttachmentBindings(t *testing.T) {
	_, ts := setupAPI(t)

	projectResp, err := post(ts, "/projects", map[string]any{
		"name": "attachment-safety-project",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	var project core.Project
	if err := decodeJSON(projectResp, &project); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	resourceResp, err := post(ts, fmt.Sprintf("/projects/%d/spaces", project.ID), map[string]any{
		"kind":     "local_fs",
		"root_uri": t.TempDir(),
		"label":    "workspace",
	})
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if resourceResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating resource, got %d", resourceResp.StatusCode)
	}
	var binding core.ResourceSpace
	if err := decodeJSON(resourceResp, &binding); err != nil {
		t.Fatalf("decode resource: %v", err)
	}

	resp, err := get(ts, fmt.Sprintf("/resources/%d", binding.ID))
	if err != nil {
		t.Fatalf("get attachment: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-attachment binding, got %d", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodDelete, ts.URL+fmt.Sprintf("/resources/%d", binding.ID), nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete attachment: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 deleting non-attachment binding, got %d", resp.StatusCode)
	}
}

func TestAPI_TriggerInspectionRejectsInvalidJSON(t *testing.T) {
	h, ts := setupAPI(t)
	h.inspectionEngine = inspectionapp.New(h.store, h.bus)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/inspections/trigger", bytes.NewBufferString("{"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post inspection trigger: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestAPI_CreateIssue_Validation(t *testing.T) {
	_, ts := setupAPI(t)

	// Missing title.
	resp, _ := post(ts, "/work-items", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAPI_GetWorkItem(t *testing.T) {
	_, ts := setupAPI(t)

	// Create issue.
	resp, _ := post(ts, "/work-items", map[string]any{"title": "get-test", "priority": "medium"})
	var created core.WorkItem
	decodeJSON(resp, &created)

	// Get issue.
	resp, _ = get(ts, fmt.Sprintf("/work-items/%d", created.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got core.WorkItem
	decodeJSON(resp, &got)
	if got.Title != "get-test" {
		t.Fatalf("expected title get-test, got %s", got.Title)
	}
}

func TestAPI_GetIssue_NotFound(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := get(ts, "/work-items/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAPI_ListWorkItems(t *testing.T) {
	_, ts := setupAPI(t)

	post(ts, "/work-items", map[string]any{"title": "issue-1", "priority": "medium"})
	resp, _ := post(ts, "/work-items", map[string]any{"title": "issue-2", "priority": "medium"})
	var archivedIssue core.WorkItem
	decodeJSON(resp, &archivedIssue)
	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/archive", archivedIssue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 when archiving, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, "/work-items")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var issues []*core.WorkItem
	decodeJSON(resp, &issues)
	if len(issues) != 1 {
		t.Fatalf("expected 1 unarchived issue, got %d", len(issues))
	}

	resp, _ = get(ts, "/work-items?archived=true")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 archived list, got %d", resp.StatusCode)
	}
	decodeJSON(resp, &issues)
	if len(issues) != 1 {
		t.Fatalf("expected 1 archived issue, got %d", len(issues))
	}

	resp, _ = get(ts, "/work-items?archived=all")
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

	post(ts, "/work-items", map[string]any{"title": "i1", "priority": "medium"})
	post(ts, "/work-items", map[string]any{"title": "i2", "priority": "medium"})

	resp, _ := get(ts, "/work-items?status=open")
	var issues []*core.WorkItem
	decodeJSON(resp, &issues)
	if len(issues) != 2 {
		t.Fatalf("expected 2 open, got %d", len(issues))
	}

	resp, _ = get(ts, "/work-items?status=running")
	decodeJSON(resp, &issues)
	if len(issues) != 0 {
		t.Fatalf("expected 0 running, got %d", len(issues))
	}
}

func TestAPI_ArchiveIssue(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/work-items", map[string]any{"title": "archive-test", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/archive", issue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var archivedIssue core.WorkItem
	decodeJSON(resp, &archivedIssue)
	if archivedIssue.ArchivedAt == nil {
		t.Fatal("expected archived_at to be set")
	}

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/unarchive", issue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on unarchive, got %d", resp.StatusCode)
	}
	var unarchivedIssue core.WorkItem
	decodeJSON(resp, &unarchivedIssue)
	if unarchivedIssue.ArchivedAt != nil {
		t.Fatal("expected archived_at to be cleared")
	}
}

func TestAPI_ArchiveIssue_RejectsActiveIssue(t *testing.T) {
	h, ts := setupAPI(t)

	resp, _ := post(ts, "/work-items", map[string]any{"title": "running-issue", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	if err := h.store.UpdateWorkItemStatus(context.Background(), issue.ID, core.WorkItemRunning); err != nil {
		t.Fatalf("set issue running: %v", err)
	}

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/archive", issue.ID), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestAPI_RunIssue_Archived(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/work-items", map[string]any{"title": "archived-run", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/archive", issue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 when archiving, got %d", resp.StatusCode)
	}

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 running archived issue, got %d", resp.StatusCode)
	}
}

func TestAPI_GetStats_IncludesArchivedIssues(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/work-items", map[string]any{"title": "done-issue", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/archive", issue.ID), nil)
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

func TestAPI_CreateAction(t *testing.T) {
	_, ts := setupAPI(t)

	// Create issue first.
	resp, _ := post(ts, "/work-items", map[string]any{"title": "issue", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	// Create step.
	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
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

	var step core.Action
	decodeJSON(resp, &step)
	if step.Name != "build" {
		t.Fatalf("expected name build, got %s", step.Name)
	}
	if step.Type != core.ActionExec {
		t.Fatalf("expected type exec, got %s", step.Type)
	}
	if step.MaxRetries != 2 {
		t.Fatalf("expected max_retries=2, got %d", step.MaxRetries)
	}
}

func TestAPI_ListSteps(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/work-items", map[string]any{"title": "issue", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})
	post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{"name": "B", "type": "gate"})

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	var steps []*core.Action
	decodeJSON(resp, &steps)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
}

func TestAPI_GetAction(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/work-items", map[string]any{"title": "issue", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})
	var created core.Action
	decodeJSON(resp, &created)

	resp, _ = get(ts, fmt.Sprintf("/steps/%d", created.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var step core.Action
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
	resp, _ := post(ts, "/work-items", map[string]any{"title": "no-steps", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	// Try to run — should fail with 400.
	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
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
	resp, _ := post(ts, "/work-items", map[string]any{"title": "run-test", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})

	// Run issue.
	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Wait for async completion.
	time.Sleep(500 * time.Millisecond)

	// Verify issue is done.
	resp, _ = get(ts, fmt.Sprintf("/work-items/%d", issue.ID))
	var done core.WorkItem
	decodeJSON(resp, &done)
	if done.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", done.Status)
	}
}

func TestAPI_RunIssue_NotOpen(t *testing.T) {
	_, ts := setupAPI(t)

	// Create and run issue.
	resp, _ := post(ts, "/work-items", map[string]any{"title": "run-twice", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)
	post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})
	post(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)

	// Wait for issue to complete.
	time.Sleep(500 * time.Millisecond)

	// Verify issue is done.
	resp, _ = get(ts, fmt.Sprintf("/work-items/%d", issue.ID))
	decodeJSON(resp, &issue)
	if issue.Status != core.WorkItemDone {
		t.Fatalf("expected done after first run, got %s", issue.Status)
	}

	// Try to run again — should fail since it's not open.
	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestAPI_CancelIssue(t *testing.T) {
	h, ts := setupAPI(t)

	resp, _ := post(ts, "/work-items", map[string]any{"title": "cancel-test", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)

	// Manually set issue to running for cancel test.
	h.store.UpdateWorkItemStatus(context.Background(), issue.ID, core.WorkItemRunning)

	resp, _ = post(ts, fmt.Sprintf("/work-items/%d/cancel", issue.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/work-items/%d", issue.ID))
	var cancelled core.WorkItem
	decodeJSON(resp, &cancelled)
	if cancelled.Status != core.WorkItemCancelled {
		t.Fatalf("expected cancelled, got %s", cancelled.Status)
	}
}

// ---------------------------------------------------------------------------
// Events Tests
// ---------------------------------------------------------------------------

func TestAPI_ListEvents(t *testing.T) {
	_, ts := setupAPI(t)

	// Create issue + step + run to generate events.
	resp, _ := post(ts, "/work-items", map[string]any{"title": "events-test", "priority": "medium"})
	var issue core.WorkItem
	decodeJSON(resp, &issue)
	post(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{"name": "A", "type": "exec"})
	post(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	time.Sleep(500 * time.Millisecond)

	// List all events.
	resp, _ = get(ts, "/events")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// List events filtered by issue.
	resp, _ = get(ts, fmt.Sprintf("/work-items/%d/events", issue.ID))
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

func TestAPI_ListEvents_FilterThreadID(t *testing.T) {
	h, ts := setupAPI(t)

	now := time.Now().UTC()
	if _, err := h.store.CreateEvent(context.Background(), &core.Event{
		Type:      core.EventThreadMessage,
		Data:      map[string]any{"thread_id": int64(7), "message": "hello"},
		Timestamp: now,
	}); err != nil {
		t.Fatalf("create thread event a: %v", err)
	}
	if _, err := h.store.CreateEvent(context.Background(), &core.Event{
		Type:      core.EventThreadMessage,
		Data:      map[string]any{"thread_id": int64(8), "message": "world"},
		Timestamp: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create thread event b: %v", err)
	}

	resp, err := get(ts, "/events?types=thread.message&thread_id=7")
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
	if got, _ := events[0].Data["thread_id"].(float64); int64(got) != 7 {
		t.Fatalf("expected thread_id=7, got %#v", events[0].Data["thread_id"])
	}
}

func TestAPI_ListThreadEventsAlias(t *testing.T) {
	h, ts := setupAPI(t)

	resp, _ := post(ts, "/threads", map[string]any{"title": "thread-events"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	if _, err := h.store.CreateEvent(context.Background(), &core.Event{
		Type: core.EventThreadMessage,
		Data: map[string]any{
			"thread_id": thread.ID,
			"message":   "from alias route",
		},
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create thread event: %v", err)
	}

	resp, err := get(ts, fmt.Sprintf("/threads/%d/events?types=thread.message", thread.ID))
	if err != nil {
		t.Fatalf("get thread events alias: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var events []core.Event
	if err := decodeJSON(resp, &events); err != nil {
		t.Fatalf("decode thread events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestAPI_ToolCallAuditRoutes(t *testing.T) {
	h, ts := setupAPI(t)

	ctx := context.Background()
	workItemID, err := h.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "tool-call-audit",
		Status:   core.WorkItemOpen,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := h.store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "tool-call-action",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	runID, err := h.store.CreateRun(ctx, &core.Run{
		ActionID:   actionID,
		WorkItemID: workItemID,
		Status:     core.RunCreated,
		Attempt:    1,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	logger := audit.NewLogger(h.store, audit.Config{
		Enabled:        true,
		RootDir:        t.TempDir(),
		RedactionLevel: "basic",
	})
	sink := logger.NewRunSink(audit.Scope{
		WorkItemID: workItemID,
		ActionID:   actionID,
		RunID:      runID,
	})

	if err := sink.HandleSessionUpdate(ctx, acpclient.SessionUpdate{
		SessionID: "session-1",
		Type:      "tool_call",
		Status:    "started",
		RawJSON: mustMarshalJSONRaw(t, map[string]any{
			"title":      "functions.shell_command",
			"toolCallId": "call-1",
			"status":     "started",
			"rawInput": map[string]any{
				"token": "sk-http-123",
			},
		}),
	}); err != nil {
		t.Fatalf("handle tool start: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := sink.HandleSessionUpdate(ctx, acpclient.SessionUpdate{
		SessionID: "session-1",
		Type:      "tool_call_update",
		Status:    "completed",
		RawJSON: mustMarshalJSONRaw(t, map[string]any{
			"title":      "functions.shell_command",
			"toolCallId": "call-1",
			"status":     "completed",
			"rawOutput": map[string]any{
				"exit_code": 0,
				"stdout":    "token=secret-value",
				"stderr":    "authorization: Bearer qwe.rty",
			},
		}),
	}); err != nil {
		t.Fatalf("handle tool finish: %v", err)
	}

	listResp, err := get(ts, fmt.Sprintf("/runs/%d/tool-calls", runID))
	if err != nil {
		t.Fatalf("list tool call audits: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResp.StatusCode)
	}
	var list []*core.ToolCallAudit
	if err := decodeJSON(listResp, &list); err != nil {
		t.Fatalf("decode audit list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 audit item, got %d", len(list))
	}
	if list[0].ToolCallID != "call-1" || list[0].Status != "completed" {
		t.Fatalf("unexpected audit list item: %+v", list[0])
	}

	detailResp, err := get(ts, fmt.Sprintf("/tool-calls/%d", list[0].ID))
	if err != nil {
		t.Fatalf("get tool call audit: %v", err)
	}
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", detailResp.StatusCode)
	}
	var detail core.ToolCallAudit
	if err := decodeJSON(detailResp, &detail); err != nil {
		t.Fatalf("decode audit detail: %v", err)
	}
	if detail.ID != list[0].ID || detail.ToolCallID != "call-1" {
		t.Fatalf("unexpected audit detail: %+v", detail)
	}
}

func TestAPI_RunAuditTimelineRoute(t *testing.T) {
	h, ts := setupAPI(t)
	ctx := context.Background()

	workItemID, err := h.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "audit-timeline",
		Status:   core.WorkItemRunning,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := h.store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "audit-timeline-action",
		Type:       core.ActionExec,
		Status:     core.ActionRunning,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	runStartedAt := time.Now().UTC().Add(-10 * time.Minute)
	runID, err := h.store.CreateRun(ctx, &core.Run{
		ActionID:   actionID,
		WorkItemID: workItemID,
		Status:     core.RunRunning,
		Attempt:    1,
		StartedAt:  &runStartedAt,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	eventAt := time.Now().UTC().Add(-5 * time.Minute)
	if _, err := h.store.CreateEvent(ctx, &core.Event{
		Type:       core.EventRunAudit,
		WorkItemID: workItemID,
		ActionID:   actionID,
		RunID:      runID,
		Timestamp:  eventAt,
		Data: map[string]any{
			"kind":   "run.dispatch",
			"status": "succeeded",
		},
	}); err != nil {
		t.Fatalf("create execution audit event: %v", err)
	}

	probeSentAt := time.Now().UTC().Add(-4 * time.Minute).Add(20 * time.Second)
	probeAnsweredAt := probeSentAt.Add(30 * time.Second)
	probeSig := core.NewProbeResponseSignal(&core.RunProbe{
		RunID:         runID,
		WorkItemID:    workItemID,
		ActionID:      actionID,
		TriggerSource: core.RunProbeTriggerManual,
		Question:      "still alive?",
		Status:        core.RunProbeAnswered,
		Verdict:       core.RunProbeAlive,
		ReplyText:     "alive and progressing",
		SentAt:        &probeSentAt,
		AnsweredAt:    &probeAnsweredAt,
	})
	if _, err := h.store.CreateActionSignal(ctx, probeSig); err != nil {
		t.Fatalf("create probe signal: %v", err)
	}

	toolStartedAt := time.Now().UTC().Add(-3 * time.Minute)
	toolFinishedAt := toolStartedAt.Add(45 * time.Second)
	if _, err := h.store.CreateToolCallAudit(ctx, &core.ToolCallAudit{
		WorkItemID:     workItemID,
		ActionID:       actionID,
		RunID:          runID,
		SessionID:      "session-1",
		ToolCallID:     "call-1",
		ToolName:       "functions.shell_command",
		Status:         "completed",
		StartedAt:      &toolStartedAt,
		FinishedAt:     &toolFinishedAt,
		DurationMs:     toolFinishedAt.Sub(toolStartedAt).Milliseconds(),
		InputPreview:   "{\"command\":\"dir\"}",
		OutputPreview:  "{\"stdout\":\"ok\"}",
		RedactionLevel: "basic",
		CreatedAt:      toolStartedAt,
	}); err != nil {
		t.Fatalf("create tool call audit: %v", err)
	}
	signalCreatedAt := time.Now().UTC().Add(-2 * time.Minute)
	if _, err := h.store.CreateActionSignal(ctx, &core.ActionSignal{
		ActionID:   actionID,
		WorkItemID: workItemID,
		RunID:      runID,
		Type:       core.SignalComplete,
		Source:     core.SignalSourceAgent,
		Payload: map[string]any{
			"summary": "implemented auth module with tests",
		},
		Actor:     "agent",
		CreatedAt: signalCreatedAt,
	}); err != nil {
		t.Fatalf("create action signal: %v", err)
	}

	resp, err := get(ts, fmt.Sprintf("/runs/%d/audit-timeline", runID))
	if err != nil {
		t.Fatalf("get audit timeline: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got struct {
		RunID int64 `json:"run_id"`
		Items []struct {
			Source    string    `json:"source"`
			Kind      string    `json:"kind"`
			Timestamp time.Time `json:"timestamp"`
			Status    string    `json:"status"`
			Summary   string    `json:"summary"`
			Event     *struct {
				Type string `json:"type"`
			} `json:"event"`
			Probe *struct {
				Status string `json:"status"`
			} `json:"probe"`
			Signal *struct {
				Type string `json:"type"`
			} `json:"signal"`
			ToolCall *struct {
				ToolCallID string `json:"tool_call_id"`
			} `json:"tool_call"`
		} `json:"items"`
	}
	if err := decodeJSON(resp, &got); err != nil {
		t.Fatalf("decode audit timeline: %v", err)
	}
	if got.RunID != runID {
		t.Fatalf("run_id = %d, want %d", got.RunID, runID)
	}
	if len(got.Items) != 4 {
		t.Fatalf("expected 4 timeline items, got %d", len(got.Items))
	}
	if !got.Items[0].Timestamp.Before(got.Items[1].Timestamp) || !got.Items[1].Timestamp.Before(got.Items[2].Timestamp) || !got.Items[2].Timestamp.Before(got.Items[3].Timestamp) {
		t.Fatalf("timeline items not sorted by timestamp: %+v", got.Items)
	}
	if got.Items[0].Source != "event" || got.Items[0].Kind != string(core.EventRunAudit) || got.Items[0].Status != "succeeded" {
		t.Fatalf("unexpected event item: %+v", got.Items[0])
	}
	if got.Items[0].Summary != "run.dispatch succeeded" {
		t.Fatalf("event summary = %q, want run.dispatch succeeded", got.Items[0].Summary)
	}
	if got.Items[1].Source != "probe" || got.Items[1].Probe == nil || got.Items[1].Probe.Status != string(core.RunProbeAnswered) {
		t.Fatalf("unexpected probe item: %+v", got.Items[1])
	}
	if got.Items[2].Source != "tool_call" || got.Items[2].ToolCall == nil || got.Items[2].ToolCall.ToolCallID != "call-1" {
		t.Fatalf("unexpected tool call item: %+v", got.Items[2])
	}
	if got.Items[3].Source != "signal" || got.Items[3].Signal == nil || got.Items[3].Signal.Type != string(core.SignalComplete) {
		t.Fatalf("unexpected signal item: %+v", got.Items[3])
	}
	if got.Items[3].Summary != "implemented auth module with tests" {
		t.Fatalf("signal summary = %q, want implemented auth module with tests", got.Items[3].Summary)
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
		Type:       core.EventWorkItemStarted,
		WorkItemID: 42,
		Timestamp:  time.Now().UTC(),
	})

	// Read event from WebSocket.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ev core.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev.Type != core.EventWorkItemStarted {
		t.Fatalf("expected issue.started, got %s", ev.Type)
	}
	if ev.WorkItemID != 42 {
		t.Fatalf("expected issue_id=42, got %d", ev.WorkItemID)
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

func TestAPI_GetLLMConfig(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := get(ts, "/admin/system/llm-config")
	if err != nil {
		t.Fatalf("get llm config: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got struct {
		DefaultConfigID string `json:"default_config_id"`
		Configs         []struct {
			ID                   string  `json:"id"`
			Type                 string  `json:"type"`
			BaseURL              string  `json:"base_url"`
			APIKey               string  `json:"api_key"`
			Model                string  `json:"model"`
			Temperature          float64 `json:"temperature"`
			MaxOutputTokens      int64   `json:"max_output_tokens"`
			ReasoningEffort      string  `json:"reasoning_effort"`
			ThinkingBudgetTokens int64   `json:"thinking_budget_tokens"`
		} `json:"configs"`
	}
	if err := decodeJSON(resp, &got); err != nil {
		t.Fatalf("decode llm config: %v", err)
	}
	if got.DefaultConfigID != "openai-response-default" {
		t.Fatalf("default_config_id = %q, want openai-response-default", got.DefaultConfigID)
	}
	if len(got.Configs) < 3 {
		t.Fatalf("expected at least 3 llm configs, got %#v", got.Configs)
	}
	if got.Configs[0].ID == "" || got.Configs[0].Type == "" {
		t.Fatalf("expected llm config id/type, got %#v", got.Configs[0])
	}
	if got.Configs[2].MaxOutputTokens != 4096 {
		t.Fatalf("expected anthropic max_output_tokens in response, got %#v", got.Configs[2])
	}
}

func TestAPI_UpdateLLMConfig(t *testing.T) {
	h, ts := setupAPI(t)
	ctrl := &stubLLMConfigController{
		report: llmconfig.Report{
			DefaultConfigID: "openai-response-default",
			Configs: []config.RuntimeLLMEntryConfig{
				{
					ID:      "openai-chat-default",
					Type:    "openai_chat_completion",
					BaseURL: "https://api.openai.com/v1",
					APIKey:  "",
					Model:   "gpt-4.1",
				},
				{
					ID:      "openai-response-default",
					Type:    "openai_response",
					BaseURL: "https://api.openai.com/v1",
					APIKey:  "",
					Model:   "gpt-4.1-mini",
				},
				{
					ID:                   "anthropic-default",
					Type:                 "anthropic",
					BaseURL:              "https://api.anthropic.com",
					APIKey:               "",
					Model:                "claude-3-7-sonnet-latest",
					Temperature:          0,
					MaxOutputTokens:      4096,
					ThinkingBudgetTokens: 1024,
				},
			},
		},
	}
	h.llmConfig = ctrl

	resp, err := put(ts, "/admin/system/llm-config", map[string]any{
		"default_config_id": "anthropic-default",
		"configs": []map[string]any{
			{
				"id":                     "anthropic-default",
				"type":                   "anthropic",
				"base_url":               "https://api.anthropic.com",
				"api_key":                "sk-ant",
				"model":                  "claude-sonnet-4-5",
				"temperature":            0.15,
				"max_output_tokens":      4096,
				"reasoning_effort":       "high",
				"thinking_budget_tokens": 2048,
			},
		},
	})
	if err != nil {
		t.Fatalf("update llm config: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got llmconfig.Report
	if err := decodeJSON(resp, &got); err != nil {
		t.Fatalf("decode llm update: %v", err)
	}
	if got.DefaultConfigID != "anthropic-default" {
		t.Fatalf("default_config_id = %q, want anthropic-default", got.DefaultConfigID)
	}
	if len(got.Configs) != 1 || got.Configs[0].APIKey != "sk-ant" {
		t.Fatalf("anthropic api key not updated: %#v", got.Configs)
	}
	if ctrl.lastReq.DefaultConfigID == nil || *ctrl.lastReq.DefaultConfigID != "anthropic-default" {
		t.Fatalf("default_config_id request not passed through: %#v", ctrl.lastReq)
	}
	if ctrl.lastReq.Configs == nil || len(*ctrl.lastReq.Configs) != 1 || (*ctrl.lastReq.Configs)[0].Model != "claude-sonnet-4-5" {
		t.Fatalf("configs request not passed through: %#v", ctrl.lastReq)
	}
	if (*ctrl.lastReq.Configs)[0].ThinkingBudgetTokens != 2048 || (*ctrl.lastReq.Configs)[0].ReasoningEffort != "high" {
		t.Fatalf("llm tuning fields not passed through: %#v", ctrl.lastReq)
	}
}

func TestAPI_UpdateLLMConfig_ConfigUnavailable(t *testing.T) {
	h, ts := setupAPI(t)
	h.llmConfig = &stubLLMConfigController{
		report: llmconfig.Report{
			DefaultConfigID: "openai-response-default",
			Configs:         []config.RuntimeLLMEntryConfig{},
		},
		updateErr: llmconfig.ErrLLMConfigUnavailable,
	}

	resp, err := put(ts, "/admin/system/llm-config", map[string]any{
		"default_config_id": "openai-response-default",
	})
	if err != nil {
		t.Fatalf("update llm config unavailable: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestAPI_UpdateLLMConfig_BadRequest(t *testing.T) {
	h, ts := setupAPI(t)
	h.llmConfig = &stubLLMConfigController{
		report: llmconfig.Report{
			DefaultConfigID: "openai-response-default",
			Configs:         []config.RuntimeLLMEntryConfig{},
		},
		updateErr: errors.New("unknown llm provider"),
	}

	resp, err := put(ts, "/admin/system/llm-config", map[string]any{
		"default_config_id": "bad-provider",
	})
	if err != nil {
		t.Fatalf("update llm config bad request: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
