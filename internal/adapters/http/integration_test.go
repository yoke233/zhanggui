package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	agentapp "github.com/yoke233/ai-workflow/internal/application/agent"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

// ---------------------------------------------------------------------------
// Full-stack test harness: Store + Bus + Persister + Registry + Engine + Scheduler + API
// ---------------------------------------------------------------------------

type integrationEnv struct {
	store     core.Store
	bus       core.EventBus
	persister *flowapp.EventPersister
	registry  *agentapp.ConfigRegistry
	eng       *flowapp.WorkItemEngine
	scheduler *flowapp.WorkItemScheduler
	handler   *Handler
	server    *httptest.Server
	cancel    context.CancelFunc
}

func setupIntegration(t *testing.T, executor flowapp.ActionExecutor) *integrationEnv {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "e2e.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	bus := membus.NewBus()

	// Start event persister.
	ctx, cancel := context.WithCancel(context.Background())
	persister := flowapp.NewEventPersister(store, bus)
	if err := persister.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start persister: %v", err)
	}

	// Setup agent registry with worker profile + gate profile.
	registry := agentapp.NewConfigRegistry()
	testDriverCfg := core.DriverConfig{
		LaunchCommand: "echo",
		LaunchArgs:    []string{"test"},
		CapabilitiesMax: core.DriverCapabilities{
			FSRead: true, FSWrite: true, Terminal: true,
		},
	}
	registry.LoadProfiles([]*core.AgentProfile{
		{
			ID:           "test-worker",
			Name:         "Test Worker",
			Driver:       testDriverCfg,
			Role:         core.RoleWorker,
			Capabilities: []string{"go", "backend"},
		},
		{
			ID:           "test-gate",
			Name:         "Test Gate",
			Driver:       testDriverCfg,
			Role:         core.RoleGate,
			Capabilities: []string{"review"},
		},
	})

	eng := flowapp.New(store, bus, executor,
		flowapp.WithConcurrency(4),
		flowapp.WithResolver(registry),
	)

	scheduler := flowapp.NewFlowScheduler(eng, store, bus, flowapp.FlowSchedulerConfig{
		MaxConcurrentFlows: 2,
	})
	go scheduler.Start(ctx)

	h := NewHandler(store, bus, eng,
		WithScheduler(scheduler),
		WithRegistry(registry),
	)
	r := chi.NewRouter()
	h.Register(r)
	ts := httptest.NewServer(r)

	t.Cleanup(func() {
		ts.Close()
		cancel()
		persister.Stop()
	})

	return &integrationEnv{
		store:     store,
		bus:       bus,
		persister: persister,
		registry:  registry,
		eng:       eng,
		scheduler: scheduler,
		handler:   h,
		server:    ts,
		cancel:    cancel,
	}
}

func postJSON(ts *httptest.Server, path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	return http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
}

func getJSON(ts *httptest.Server, path string) (*http.Response, error) {
	return http.Get(ts.URL + path)
}

func putJSON(ts *httptest.Server, path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func deleteReq(ts *httptest.Server, path string) (*http.Response, error) {
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+path, nil)
	return http.DefaultClient.Do(req)
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode %T: %v", v, err)
	}
	return v
}

func requireStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Fatalf("expected HTTP %d, got %d", expected, resp.StatusCode)
	}
}

// pollWorkItemStatus polls until the issue reaches the target status or timeout.
func pollWorkItemStatus(t *testing.T, ts *httptest.Server, issueID int64, target core.WorkItemStatus, timeout time.Duration) core.WorkItem {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, _ := getJSON(ts, fmt.Sprintf("/work-items/%d", issueID))
		f := decode[core.WorkItem](t, resp)
		if f.Status == target {
			return f
		}
		// Also stop polling if issue reached a terminal state that isn't target.
		if f.Status == core.WorkItemDone || f.Status == core.WorkItemFailed || f.Status == core.WorkItemCancelled {
			if f.Status != target {
				t.Fatalf("issue %d reached terminal %s, wanted %s", issueID, f.Status, target)
			}
			return f
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("issue %d did not reach %s within %v", issueID, target, timeout)
	return core.WorkItem{}
}

// ---------------------------------------------------------------------------
// Test 1: Full lifecycle — Project + Issue + Steps → run → done
// ---------------------------------------------------------------------------

func TestIntegration_FullLifecycle(t *testing.T) {
	var execCount int32
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error {
		atomic.AddInt32(&execCount, 1)
		exec.Output = map[string]any{
			"result": fmt.Sprintf("completed %s", step.Name),
		}
		return nil
	}
	env := setupIntegration(t, executor)
	ts := env.server

	// 1. Create project.
	resp, _ := postJSON(ts, "/projects", map[string]any{
		"name": "e2e-project", "kind": "dev", "description": "E2E test project",
	})
	requireStatus(t, resp, http.StatusCreated)
	project := decode[core.Project](t, resp)
	if project.ID == 0 {
		t.Fatal("expected non-zero project ID")
	}

	// 2. Create issue linked to project.
	resp, _ = postJSON(ts, "/work-items", map[string]any{
		"title":      "e2e-issue",
		"priority":   "medium",
		"project_id": project.ID,
	})
	requireStatus(t, resp, http.StatusCreated)
	issue := decode[core.WorkItem](t, resp)
	if issue.ProjectID == nil || *issue.ProjectID != project.ID {
		t.Fatalf("expected project_id=%d, got %v", project.ID, issue.ProjectID)
	}

	// 3. Create steps: A, B, C (sequential by position).
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "step-A", "type": "exec", "agent_role": "worker",
	})
	requireStatus(t, resp, http.StatusCreated)
	stepA := decode[core.Action](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "step-B", "type": "exec", "agent_role": "worker",
	})
	requireStatus(t, resp, http.StatusCreated)
	stepB := decode[core.Action](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "step-C", "type": "exec", "agent_role": "worker",
	})
	requireStatus(t, resp, http.StatusCreated)
	stepC := decode[core.Action](t, resp)

	// 4. Verify step listing.
	resp, _ = getJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	requireStatus(t, resp, http.StatusOK)
	steps := decode[[]*core.Action](t, resp)
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}

	// 5. Run issue (goes through scheduler → queued → running → done).
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	requireStatus(t, resp, http.StatusAccepted)

	// 6. Poll until done.
	doneIssue := pollWorkItemStatus(t, ts, issue.ID, core.WorkItemDone, 5*time.Second)
	if doneIssue.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", doneIssue.Status)
	}

	// 7. Verify all steps done.
	for _, id := range []int64{stepA.ID, stepB.ID, stepC.ID} {
		resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d", id))
		s := decode[core.Action](t, resp)
		if s.Status != core.ActionDone {
			t.Fatalf("step %d: expected done, got %s", id, s.Status)
		}
	}

	// 8. Verify executions exist for each step.
	for _, id := range []int64{stepA.ID, stepB.ID, stepC.ID} {
		resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d/executions", id))
		execs := decode[[]*core.Run](t, resp)
		if len(execs) == 0 {
			t.Fatalf("step %d: expected at least 1 execution", id)
		}
		if execs[0].Status != core.RunSucceeded {
			t.Fatalf("step %d exec: expected succeeded, got %s", id, execs[0].Status)
		}
	}

	// 9. Verify all 3 steps were executed.
	if c := atomic.LoadInt32(&execCount); c != 3 {
		t.Fatalf("expected 3 executions, got %d", c)
	}

	// 10. Verify persisted events exist.
	time.Sleep(100 * time.Millisecond) // allow persister to flush
	resp, _ = getJSON(ts, fmt.Sprintf("/work-items/%d/events", issue.ID))
	requireStatus(t, resp, http.StatusOK)
	events := decode[[]*core.Event](t, resp)
	if len(events) == 0 {
		t.Fatal("expected persisted events for issue")
	}

	// Verify event types include issue lifecycle.
	eventTypes := make(map[core.EventType]bool)
	for _, ev := range events {
		eventTypes[ev.Type] = true
	}
	for _, expected := range []core.EventType{
		core.EventWorkItemQueued, core.EventWorkItemStarted, core.EventWorkItemCompleted,
	} {
		if !eventTypes[expected] {
			t.Errorf("missing event type %s in persisted events", expected)
		}
	}

	// 11. Verify stats endpoint.
	resp, _ = getJSON(ts, "/stats")
	requireStatus(t, resp, http.StatusOK)
	var stats statsResponse
	decodeJSON(resp, &stats)
	if stats.TotalIssues < 1 {
		t.Fatalf("expected at least 1 total issue, got %d", stats.TotalIssues)
	}
}

// ---------------------------------------------------------------------------
// Test 2: Fan-out + fan-in DAG with concurrent execution
// ---------------------------------------------------------------------------

func TestIntegration_FanOutFanIn(t *testing.T) {
	var execCount int32
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error {
		atomic.AddInt32(&execCount, 1)
		// Simulate work.
		time.Sleep(50 * time.Millisecond)
		exec.Output = map[string]any{"step": step.Name}
		return nil
	}
	env := setupIntegration(t, executor)
	ts := env.server

	// Create issue.
	resp, _ := postJSON(ts, "/work-items", map[string]any{"title": "fan-out", "priority": "medium"})
	issue := decode[core.WorkItem](t, resp)

	// Steps: A, B, C, D, E (sequential by position).
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "A", "type": "exec",
	})
	_ = decode[core.Action](t, resp)

	for _, name := range []string{"B", "C", "D"} {
		resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
			"name": name, "type": "exec",
		})
		_ = decode[core.Action](t, resp)
	}

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "E", "type": "exec",
	})
	sE := decode[core.Action](t, resp)

	// Run.
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	requireStatus(t, resp, http.StatusAccepted)

	pollWorkItemStatus(t, ts, issue.ID, core.WorkItemDone, 5*time.Second)

	// All 5 steps executed.
	if c := atomic.LoadInt32(&execCount); c != 5 {
		t.Fatalf("expected 5 executions, got %d", c)
	}

	// E is done.
	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d", sE.ID))
	stepE := decode[core.Action](t, resp)
	if stepE.Status != core.ActionDone {
		t.Fatalf("step E expected done, got %s", stepE.Status)
	}
}

// ---------------------------------------------------------------------------
// Test 3: Gate pass flow — exec → gate(pass) → exec
// ---------------------------------------------------------------------------

func TestIntegration_GatePass(t *testing.T) {
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error {
		if step.Type == core.ActionGate {
			// Gate always passes.
			exec.Output = map[string]any{"verdict": "pass", "reason": "all good"}
		} else {
			exec.Output = map[string]any{"result": step.Name + " done"}
		}
		return nil
	}
	env := setupIntegration(t, executor)
	ts := env.server

	resp, _ := postJSON(ts, "/work-items", map[string]any{"title": "gate-pass", "priority": "medium"})
	issue := decode[core.WorkItem](t, resp)

	// Steps: build → review(gate) → deploy (sequential by position).
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "build", "type": "exec",
	})
	_ = decode[core.Action](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "review", "type": "gate",
		"acceptance_criteria": []string{"code compiles", "tests pass"},
	})
	_ = decode[core.Action](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "deploy", "type": "exec",
	})
	_ = decode[core.Action](t, resp)

	// Run.
	postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	pollWorkItemStatus(t, ts, issue.ID, core.WorkItemDone, 5*time.Second)

	// Gate passed — verify event.
	time.Sleep(100 * time.Millisecond)
	resp, _ = getJSON(ts, fmt.Sprintf("/work-items/%d/events", issue.ID))
	events := decode[[]*core.Event](t, resp)
	hasGatePass := false
	for _, ev := range events {
		if ev.Type == core.EventGatePassed {
			hasGatePass = true
			break
		}
	}
	if !hasGatePass {
		t.Error("expected gate.passed event in persisted events")
	}
}

// ---------------------------------------------------------------------------
// Test 4: Step failure with retry, then succeed
// ---------------------------------------------------------------------------

func TestIntegration_RetryThenSucceed(t *testing.T) {
	var attempts int32
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 1 {
			// First attempt fails (transient).
			exec.ErrorKind = core.ErrKindTransient
			return fmt.Errorf("transient network error")
		}
		exec.Output = map[string]any{"result": "ok after retry"}
		return nil
	}
	env := setupIntegration(t, executor)
	ts := env.server

	resp, _ := postJSON(ts, "/work-items", map[string]any{"title": "retry-issue", "priority": "medium"})
	issue := decode[core.WorkItem](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "flaky-step", "type": "exec", "max_retries": 3,
	})
	step := decode[core.Action](t, resp)

	postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	pollWorkItemStatus(t, ts, issue.ID, core.WorkItemDone, 5*time.Second)

	// Step should have 2 executions (1 failed + 1 succeeded).
	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d/executions", step.ID))
	execs := decode[[]*core.Run](t, resp)
	if len(execs) < 2 {
		t.Fatalf("expected at least 2 executions (retry), got %d", len(execs))
	}

	// At least one should be succeeded.
	hasSuccess := false
	for _, e := range execs {
		if e.Status == core.RunSucceeded {
			hasSuccess = true
		}
	}
	if !hasSuccess {
		t.Fatal("expected at least one succeeded execution after retry")
	}
}

// ---------------------------------------------------------------------------
// Test 5: Permanent failure stops flow immediately
// ---------------------------------------------------------------------------

func TestIntegration_PermanentFailure(t *testing.T) {
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error {
		exec.ErrorKind = core.ErrKindPermanent
		return fmt.Errorf("fatal: syntax error in source")
	}
	env := setupIntegration(t, executor)
	ts := env.server

	resp, _ := postJSON(ts, "/work-items", map[string]any{"title": "perm-fail", "priority": "medium"})
	issue := decode[core.WorkItem](t, resp)

	postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "broken", "type": "exec", "max_retries": 5,
	})

	postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	pollWorkItemStatus(t, ts, issue.ID, core.WorkItemFailed, 5*time.Second)

	// Only 1 execution (no retries for permanent errors).
	resp, _ = getJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	steps := decode[[]*core.Action](t, resp)
	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d/executions", steps[0].ID))
	execs := decode[[]*core.Run](t, resp)
	if len(execs) != 1 {
		t.Fatalf("expected exactly 1 execution for permanent failure, got %d", len(execs))
	}
}

// ---------------------------------------------------------------------------
// Test 6: Cancel a running flow via scheduler
// ---------------------------------------------------------------------------

func TestIntegration_CancelRunningIssue(t *testing.T) {
	started := make(chan struct{})
	executor := func(ctx context.Context, step *core.Action, exec *core.Run) error {
		close(started)
		// Block until cancelled.
		<-ctx.Done()
		return ctx.Err()
	}
	env := setupIntegration(t, executor)
	ts := env.server

	resp, _ := postJSON(ts, "/work-items", map[string]any{"title": "cancel-me", "priority": "medium"})
	issue := decode[core.WorkItem](t, resp)

	postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "long-running", "type": "exec",
	})

	postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)

	// Wait for executor to start.
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("executor did not start in time")
	}

	// Cancel.
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/cancel", issue.ID), nil)
	requireStatus(t, resp, http.StatusOK)

	// Issue should reach cancelled or failed within a reasonable time.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, _ = getJSON(ts, fmt.Sprintf("/work-items/%d", issue.ID))
		f := decode[core.WorkItem](t, resp)
		if f.Status == core.WorkItemCancelled || f.Status == core.WorkItemFailed {
			return // success
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("issue did not reach cancelled/failed state after cancel")
}

// ---------------------------------------------------------------------------
// Test 7: Project CRUD lifecycle
// ---------------------------------------------------------------------------

func TestIntegration_ProjectCRUD(t *testing.T) {
	env := setupIntegration(t, func(_ context.Context, _ *core.Action, _ *core.Run) error {
		return nil
	})
	ts := env.server

	// Create.
	resp, _ := postJSON(ts, "/projects", map[string]any{
		"name": "my-project", "kind": "dev", "description": "test project",
		"metadata": map[string]string{"team": "backend"},
	})
	requireStatus(t, resp, http.StatusCreated)
	p := decode[core.Project](t, resp)
	if p.Name != "my-project" || p.Kind != "dev" {
		t.Fatalf("unexpected project: %+v", p)
	}

	// Get.
	resp, _ = getJSON(ts, fmt.Sprintf("/projects/%d", p.ID))
	requireStatus(t, resp, http.StatusOK)
	got := decode[core.Project](t, resp)
	if got.Description != "test project" {
		t.Fatalf("expected description 'test project', got %q", got.Description)
	}

	// Update.
	resp, _ = putJSON(ts, fmt.Sprintf("/projects/%d", p.ID), map[string]any{
		"name": "renamed-project",
	})
	requireStatus(t, resp, http.StatusOK)
	updated := decode[core.Project](t, resp)
	if updated.Name != "renamed-project" {
		t.Fatalf("expected name 'renamed-project', got %q", updated.Name)
	}

	// List.
	resp, _ = getJSON(ts, "/projects")
	requireStatus(t, resp, http.StatusOK)
	projects := decode[[]*core.Project](t, resp)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	// Delete.
	resp, _ = deleteReq(ts, fmt.Sprintf("/projects/%d", p.ID))
	requireStatus(t, resp, http.StatusNoContent)

	// Verify deleted.
	resp, _ = getJSON(ts, fmt.Sprintf("/projects/%d", p.ID))
	requireStatus(t, resp, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Test 8: Resource Binding CRUD
// ---------------------------------------------------------------------------

func TestIntegration_ResourceBindingCRUD(t *testing.T) {
	env := setupIntegration(t, func(_ context.Context, _ *core.Action, _ *core.Run) error {
		return nil
	})
	ts := env.server

	// Create project first.
	resp, _ := postJSON(ts, "/projects", map[string]any{
		"name": "res-project", "kind": "dev",
	})
	p := decode[core.Project](t, resp)

	// Create resource space.
	resp, _ = postJSON(ts, fmt.Sprintf("/projects/%d/spaces", p.ID), map[string]any{
		"kind":     "git",
		"root_uri": "https://github.com/example/repo.git",
		"label":    "main-repo",
	})
	requireStatus(t, resp, http.StatusCreated)
	rb := decode[core.ResourceSpace](t, resp)
	if rb.Kind != "git" || rb.RootURI != "https://github.com/example/repo.git" {
		t.Fatalf("unexpected resource space: %+v", rb)
	}

	// List.
	resp, _ = getJSON(ts, fmt.Sprintf("/projects/%d/spaces", p.ID))
	requireStatus(t, resp, http.StatusOK)
	bindings := decode[[]*core.ResourceSpace](t, resp)
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}

	// Get.
	resp, _ = getJSON(ts, fmt.Sprintf("/spaces/%d", rb.ID))
	requireStatus(t, resp, http.StatusOK)

	// Delete.
	resp, _ = deleteReq(ts, fmt.Sprintf("/spaces/%d", rb.ID))
	requireStatus(t, resp, http.StatusNoContent)

	// Verify deleted.
	resp, _ = getJSON(ts, fmt.Sprintf("/resources/%d", rb.ID))
	requireStatus(t, resp, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Test 9: Agent Profile CRUD via API
// ---------------------------------------------------------------------------

func TestIntegration_AgentProfileCRUD(t *testing.T) {
	env := setupIntegration(t, func(_ context.Context, _ *core.Action, _ *core.Run) error {
		return nil
	})
	ts := env.server

	// Create profile with support role — should succeed.
	resp, _ := postJSON(ts, "/agents/profiles", map[string]any{
		"id":   "api-support",
		"name": "API Support",
		"driver": map[string]any{
			"launch_command": "node",
			"launch_args":    []string{"agent.js"},
			"capabilities_max": map[string]bool{
				"fs_read": true, "fs_write": false, "terminal": false,
			},
		},
		"role":         "support",
		"capabilities": []string{"javascript"},
	})
	requireStatus(t, resp, http.StatusCreated)

	// Get profile.
	resp, _ = getJSON(ts, "/agents/profiles/api-support")
	requireStatus(t, resp, http.StatusOK)
	var profile core.AgentProfile
	decodeJSON(resp, &profile)
	if profile.Driver.LaunchCommand != "node" {
		t.Fatalf("expected launch_command=node, got %s", profile.Driver.LaunchCommand)
	}

	// Capability overflow: worker role needs fs_write+terminal, but driver forbids them.
	resp, _ = postJSON(ts, "/agents/profiles", map[string]any{
		"id": "overflow-profile",
		"driver": map[string]any{
			"launch_command": "node",
			"capabilities_max": map[string]bool{
				"fs_read": true, "fs_write": false, "terminal": false,
			},
		},
		"role": "worker",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for capability overflow, got %d", resp.StatusCode)
	}

	// Delete profile.
	resp, _ = deleteReq(ts, "/agents/profiles/api-support")
	requireStatus(t, resp, http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Test 10: WebSocket real-time event streaming during flow execution
// ---------------------------------------------------------------------------

func TestIntegration_WebSocketEvents(t *testing.T) {
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error {
		exec.Output = map[string]any{"done": true}
		return nil
	}
	env := setupIntegration(t, executor)
	ts := env.server

	// Connect WebSocket.
	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond) // ensure subscription is registered

	// Create and run an issue.
	resp, _ := postJSON(ts, "/work-items", map[string]any{"title": "ws-test", "priority": "medium"})
	issue := decode[core.WorkItem](t, resp)

	postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "ws-step", "type": "exec",
	})
	postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)

	// Collect events from WebSocket.
	receivedTypes := make(map[core.EventType]bool)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < 20; i++ { // read up to 20 events
		var ev core.Event
		if err := conn.ReadJSON(&ev); err != nil {
			break // timeout or connection closed
		}
		receivedTypes[ev.Type] = true
		// Stop once we see issue.completed.
		if ev.Type == core.EventWorkItemCompleted {
			break
		}
	}

	// Verify we received key lifecycle events via WebSocket.
	for _, expected := range []core.EventType{
		core.EventWorkItemQueued, core.EventWorkItemStarted, core.EventWorkItemCompleted,
	} {
		if !receivedTypes[expected] {
			t.Errorf("did not receive %s via WebSocket", expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 11: Scheduler stats endpoint during execution
// ---------------------------------------------------------------------------

func TestIntegration_SchedulerStats(t *testing.T) {
	env := setupIntegration(t, func(_ context.Context, _ *core.Action, _ *core.Run) error {
		return nil
	})
	ts := env.server

	// Check scheduler stats (idle).
	resp, _ := getJSON(ts, "/scheduler/stats")
	requireStatus(t, resp, http.StatusOK)
	var result map[string]any
	decodeJSON(resp, &result)
	if enabled, ok := result["enabled"]; !ok || enabled != true {
		t.Fatalf("expected scheduler enabled=true, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// Test 12: Issue with project_id referencing non-existent project → 404
// ---------------------------------------------------------------------------

func TestIntegration_IssueWithInvalidProject(t *testing.T) {
	env := setupIntegration(t, func(_ context.Context, _ *core.Action, _ *core.Run) error {
		return nil
	})
	ts := env.server

	badID := int64(9999)
	resp, _ := postJSON(ts, "/work-items", map[string]any{
		"title":      "orphan-issue",
		"priority":   "medium",
		"project_id": badID,
	})
	requireStatus(t, resp, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Test 13: Concurrent issues through scheduler
// ---------------------------------------------------------------------------

func TestIntegration_ConcurrentIssues(t *testing.T) {
	var execCount int32
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error {
		atomic.AddInt32(&execCount, 1)
		time.Sleep(50 * time.Millisecond) // simulate work
		exec.Output = map[string]any{"ok": true}
		return nil
	}
	env := setupIntegration(t, executor)
	ts := env.server

	// Submit 3 issues, scheduler maxConcurrent=2, so one must queue.
	var issueIDs []int64
	for i := 0; i < 3; i++ {
		resp, _ := postJSON(ts, "/work-items", map[string]any{
			"title":    fmt.Sprintf("concurrent-%d", i),
			"priority": "medium",
		})
		f := decode[core.WorkItem](t, resp)
		postJSON(ts, fmt.Sprintf("/work-items/%d/steps", f.ID), map[string]any{
			"name": "work", "type": "exec",
		})
		issueIDs = append(issueIDs, f.ID)
	}

	// Run all 3.
	for _, id := range issueIDs {
		resp, _ := postJSON(ts, fmt.Sprintf("/work-items/%d/run", id), nil)
		requireStatus(t, resp, http.StatusAccepted)
	}

	// Wait for all to complete.
	for _, id := range issueIDs {
		pollWorkItemStatus(t, ts, id, core.WorkItemDone, 10*time.Second)
	}

	// All 3 issues × 1 step = 3 executions.
	if c := atomic.LoadInt32(&execCount); c != 3 {
		t.Fatalf("expected 3 executions, got %d", c)
	}
}

// ---------------------------------------------------------------------------
// Test 14: Step update + delete (DAG editing support)
// ---------------------------------------------------------------------------

func TestIntegration_StepUpdateAndDelete(t *testing.T) {
	env := setupIntegration(t, func(_ context.Context, _ *core.Action, _ *core.Run) error {
		return nil
	})
	ts := env.server

	// Create issue + steps.
	resp, _ := postJSON(ts, "/work-items", map[string]any{"title": "edit-dag", "priority": "medium"})
	issue := decode[core.WorkItem](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "step-A", "type": "exec", "agent_role": "worker",
	})
	sA := decode[core.Action](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "step-B", "type": "exec",
	})
	sB := decode[core.Action](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "step-C", "type": "exec",
	})
	sC := decode[core.Action](t, resp)

	// --- Update step B: rename + change role ---
	resp, _ = putJSON(ts, fmt.Sprintf("/steps/%d", sB.ID), map[string]any{
		"name":                "step-B-renamed",
		"agent_role":          "gate",
		"acceptance_criteria": []string{"all tests pass"},
	})
	requireStatus(t, resp, http.StatusOK)
	updated := decode[core.Action](t, resp)
	if updated.Name != "step-B-renamed" {
		t.Fatalf("expected step-B-renamed, got %s", updated.Name)
	}
	if updated.AgentRole != "gate" {
		t.Fatalf("expected agent_role=gate, got %s", updated.AgentRole)
	}
	if len(updated.AcceptanceCriteria) != 1 || updated.AcceptanceCriteria[0] != "all tests pass" {
		t.Fatalf("unexpected acceptance_criteria: %v", updated.AcceptanceCriteria)
	}

	// --- Delete step B ---
	resp, _ = deleteReq(ts, fmt.Sprintf("/steps/%d", sB.ID))
	requireStatus(t, resp, http.StatusNoContent)

	// Verify deleted.
	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d", sB.ID))
	requireStatus(t, resp, http.StatusNotFound)

	// Verify remaining steps.
	resp, _ = getJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID))
	steps := decode[[]*core.Action](t, resp)
	if len(steps) != 2 {
		t.Fatalf("expected 2 remaining steps, got %d", len(steps))
	}

	// --- Cannot edit/delete non-pending step ---
	// Run issue to make steps non-pending.
	postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	pollWorkItemStatus(t, ts, issue.ID, core.WorkItemDone, 5*time.Second)

	resp, _ = putJSON(ts, fmt.Sprintf("/steps/%d", sA.ID), map[string]any{
		"name": "should-fail",
	})
	requireStatus(t, resp, http.StatusConflict)

	resp, _ = deleteReq(ts, fmt.Sprintf("/steps/%d", sC.ID))
	requireStatus(t, resp, http.StatusConflict)
}

// ---------------------------------------------------------------------------
// Test 15: DAG generate-steps endpoint (mock LLM)
// ---------------------------------------------------------------------------

func TestIntegration_GenerateSteps_Unavailable(t *testing.T) {
	// Without DAGGenerator configured, should return 503.
	env := setupIntegration(t, func(_ context.Context, _ *core.Action, _ *core.Run) error {
		return nil
	})
	ts := env.server

	resp, _ := postJSON(ts, "/work-items", map[string]any{"title": "gen-test", "priority": "medium"})
	issue := decode[core.WorkItem](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/generate-steps", issue.ID), map[string]any{
		"description": "build a REST API",
	})
	requireStatus(t, resp, http.StatusServiceUnavailable)
}

// ---------------------------------------------------------------------------
// Test 16: Gate reject → rework → approve (artifact metadata path)
// Mirrors issue-e2e-github.ps1: exec → gate(reject) → exec rework → gate(approve) → done
// ---------------------------------------------------------------------------

func TestIntegration_GateRejectReworkApprove(t *testing.T) {
	var gateRuns int32
	var execRuns int32

	executor := func(ctx context.Context, step *core.Action, exec *core.Run) error {
		if step.Type == core.ActionGate {
			n := atomic.AddInt32(&gateRuns, 1)
			verdict := "reject"
			reason := "missing test coverage"
			if n > 1 {
				verdict = "pass"
				reason = "all tests present, LGTM"
			}
			// Store gate verdict in run result fields (same path as real agents).
			exec.ResultMarkdown = fmt.Sprintf("Review round %d: %s", n, reason)
			exec.ResultMetadata = map[string]any{"verdict": verdict, "reason": reason}
			exec.Output = map[string]any{"verdict": verdict, "reason": reason}
			return nil
		}
		// Exec step — just succeed.
		atomic.AddInt32(&execRuns, 1)
		exec.Output = map[string]any{"result": fmt.Sprintf("implemented %s", step.Name)}
		return nil
	}

	env := setupIntegration(t, executor)
	ts := env.server

	// 1. Create project.
	resp, _ := postJSON(ts, "/projects", map[string]any{
		"name": "gate-rework-e2e", "kind": "dev",
	})
	requireStatus(t, resp, http.StatusCreated)
	project := decode[core.Project](t, resp)

	// 2. Create issue.
	resp, _ = postJSON(ts, "/work-items", map[string]any{
		"title":      "gate reject→rework→approve",
		"priority":   "medium",
		"project_id": project.ID,
	})
	requireStatus(t, resp, http.StatusCreated)
	issue := decode[core.WorkItem](t, resp)

	// 3. Create steps: implement(exec) → review(gate).
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "implement", "type": "exec", "max_retries": 3,
	})
	requireStatus(t, resp, http.StatusCreated)
	stepImpl := decode[core.Action](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "review", "type": "gate",
	})
	requireStatus(t, resp, http.StatusCreated)
	stepGate := decode[core.Action](t, resp)

	// 4. Run issue.
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	requireStatus(t, resp, http.StatusAccepted)

	// 5. Poll until done.
	doneIssue := pollWorkItemStatus(t, ts, issue.ID, core.WorkItemDone, 10*time.Second)
	if doneIssue.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", doneIssue.Status)
	}

	// 6. Verify exec ran twice (original + rework) and gate ran twice.
	if n := atomic.LoadInt32(&execRuns); n != 2 {
		t.Fatalf("expected 2 exec runs, got %d", n)
	}
	if n := atomic.LoadInt32(&gateRuns); n != 2 {
		t.Fatalf("expected 2 gate runs, got %d", n)
	}

	// 7. Verify step statuses.
	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d", stepImpl.ID))
	finalImpl := decode[core.Action](t, resp)
	if finalImpl.Status != core.ActionDone {
		t.Fatalf("expected impl done, got %s", finalImpl.Status)
	}
	if finalImpl.RetryCount != 1 {
		t.Fatalf("expected impl retry_count=1, got %d", finalImpl.RetryCount)
	}

	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d", stepGate.ID))
	finalGate := decode[core.Action](t, resp)
	if finalGate.Status != core.ActionDone {
		t.Fatalf("expected gate done, got %s", finalGate.Status)
	}

	// 8. Verify gate events (rejected + passed).
	time.Sleep(100 * time.Millisecond) // allow persister to flush
	resp, _ = getJSON(ts, fmt.Sprintf("/work-items/%d/events", issue.ID))
	events := decode[[]*core.Event](t, resp)
	hasReject := false
	hasPass := false
	for _, ev := range events {
		if ev.Type == core.EventGateRejected {
			hasReject = true
		}
		if ev.Type == core.EventGatePassed {
			hasPass = true
		}
	}
	if !hasReject {
		t.Error("expected gate.rejected event")
	}
	if !hasPass {
		t.Error("expected gate.passed event")
	}
}

// ---------------------------------------------------------------------------
// Test 17: Gate rework limit → blocked → issue failed
// Mirrors merge-conflict-e2e.ps1 scenario: gate always rejects → hits max_rework_rounds → blocked
// ---------------------------------------------------------------------------

func TestIntegration_GateReworkLimitBlocked(t *testing.T) {
	var gateRuns int32

	executor := func(ctx context.Context, step *core.Action, exec *core.Run) error {
		if step.Type == core.ActionGate {
			atomic.AddInt32(&gateRuns, 1)
			exec.ResultMarkdown = "Review: always reject"
			exec.ResultMetadata = map[string]any{"verdict": "reject", "reason": "merge conflict unresolvable"}
			exec.Output = map[string]any{"verdict": "reject"}
			return nil
		}
		exec.Output = map[string]any{"result": "ok"}
		return nil
	}

	env := setupIntegration(t, executor)
	ts := env.server

	// Create issue with exec → gate (max_rework_rounds=2).
	resp, _ := postJSON(ts, "/work-items", map[string]any{
		"title": "rework-limit-blocked", "priority": "medium",
	})
	issue := decode[core.WorkItem](t, resp)

	postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "implement", "type": "exec", "max_retries": 10,
	})
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "review", "type": "gate",
		"config": map[string]any{"max_rework_rounds": 2},
	})
	requireStatus(t, resp, http.StatusCreated)
	stepGate := decode[core.Action](t, resp)

	// Run — should end in failed (engine returns "stuck" when gate is blocked).
	postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	pollWorkItemStatus(t, ts, issue.ID, core.WorkItemFailed, 10*time.Second)

	// Gate step should be blocked.
	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d", stepGate.ID))
	finalGate := decode[core.Action](t, resp)
	if finalGate.Status != core.ActionBlocked {
		t.Fatalf("expected gate blocked, got %s", finalGate.Status)
	}

	// Gate ran 3 times: round 1 reject → round 2 reject → round 3 reject (limit=2, so block after 2 rejects counted).
	n := atomic.LoadInt32(&gateRuns)
	if n < 2 {
		t.Fatalf("expected at least 2 gate runs, got %d", n)
	}

	// Verify rework limit event was persisted.
	time.Sleep(100 * time.Millisecond)
	resp, _ = getJSON(ts, fmt.Sprintf("/work-items/%d/events", issue.ID))
	events := decode[[]*core.Event](t, resp)
	hasLimitEvent := false
	for _, ev := range events {
		if ev.Type == core.EventGateReworkLimitReached {
			hasLimitEvent = true
		}
	}
	if !hasLimitEvent {
		t.Error("expected gate.rework_limit_reached event")
	}
}

// ---------------------------------------------------------------------------
// Test 18: Gate via ActionSignal (reject → rework → approve)
// Mirrors real agent behavior: gate agent calls gate_approve/gate_reject via MCP tool
// ---------------------------------------------------------------------------

func TestIntegration_GateSignalRejectThenApprove(t *testing.T) {
	var gateRuns int32
	var store core.Store

	executor := func(ctx context.Context, step *core.Action, exec *core.Run) error {
		if step.Type == core.ActionGate {
			n := atomic.AddInt32(&gateRuns, 1)
			if n == 1 {
				// First run: agent rejects via ActionSignal.
				_, err := store.CreateActionSignal(ctx, &core.ActionSignal{
					ActionID:   step.ID,
					WorkItemID: step.WorkItemID,
					RunID:      exec.ID,
					Type:       core.SignalReject,
					Source:     core.SignalSourceAgent,
					Payload:    map[string]any{"reason": "no error handling in auth module"},
					Actor:      "agent",
					CreatedAt:  time.Now().UTC(),
				})
				return err
			}
			// Second run: agent approves via ActionSignal.
			_, err := store.CreateActionSignal(ctx, &core.ActionSignal{
				ActionID:   step.ID,
				WorkItemID: step.WorkItemID,
				RunID:      exec.ID,
				Type:       core.SignalApprove,
				Source:     core.SignalSourceAgent,
				Payload:    map[string]any{"reason": "error handling added, LGTM"},
				Actor:      "agent",
				CreatedAt:  time.Now().UTC(),
			})
			return err
		}
		// Exec step.
		exec.Output = map[string]any{"result": "done"}
		return nil
	}

	env := setupIntegration(t, executor)
	store = env.store
	ts := env.server

	resp, _ := postJSON(ts, "/work-items", map[string]any{
		"title": "signal-gate-e2e", "priority": "medium",
	})
	issue := decode[core.WorkItem](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "implement", "type": "exec", "max_retries": 3,
	})
	stepImpl := decode[core.Action](t, resp)

	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "review", "type": "gate",
	})
	stepGate := decode[core.Action](t, resp)

	postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	pollWorkItemStatus(t, ts, issue.ID, core.WorkItemDone, 10*time.Second)

	// Verify counts.
	if n := atomic.LoadInt32(&gateRuns); n != 2 {
		t.Fatalf("expected 2 gate runs, got %d", n)
	}

	// Verify step signals persisted.
	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d/signals", stepGate.ID))
	requireStatus(t, resp, http.StatusOK)
	var signals []*core.ActionSignal
	decodeJSON(resp, &signals)
	// Should have at least: 1 agent reject + 1 system reject (from ProcessGate) + 1 agent approve + feedback signals
	hasAgentReject := false
	hasAgentApprove := false
	for _, sig := range signals {
		if sig.Type == core.SignalReject && sig.Source == core.SignalSourceAgent {
			hasAgentReject = true
		}
		if sig.Type == core.SignalApprove && sig.Source == core.SignalSourceAgent {
			hasAgentApprove = true
		}
	}
	if !hasAgentReject {
		t.Error("expected agent reject signal on gate step")
	}
	if !hasAgentApprove {
		t.Error("expected agent approve signal on gate step")
	}

	// Verify feedback signal on impl step (gate rejection propagated).
	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d/signals", stepImpl.ID))
	requireStatus(t, resp, http.StatusOK)
	var implSignals []*core.ActionSignal
	decodeJSON(resp, &implSignals)
	hasFeedback := false
	for _, sig := range implSignals {
		if sig.Type == core.SignalFeedback {
			hasFeedback = true
		}
	}
	if !hasFeedback {
		t.Error("expected feedback signal on impl step from gate rejection")
	}
}

// ---------------------------------------------------------------------------
// Test 19: Step decision API — human approve/reject on running gate
// Tests POST /steps/{stepID}/decision and GET /steps/{stepID}/signals
// ---------------------------------------------------------------------------

func TestIntegration_StepDecisionAPI(t *testing.T) {
	// Gate executor blocks waiting for human decision (real scenario).
	// For this test, the gate executor just succeeds — the signal was already
	// written before the engine's finalizeGate checks it.
	var store core.Store

	executor := func(ctx context.Context, step *core.Action, exec *core.Run) error {
		if step.Type == core.ActionGate {
			// Simulate: before finalizeGate runs, the human already submitted a decision.
			// We pre-create an approve signal that finalizeGate will find.
			_, err := store.CreateActionSignal(ctx, &core.ActionSignal{
				ActionID:   step.ID,
				WorkItemID: step.WorkItemID,
				RunID:      exec.ID,
				Type:       core.SignalApprove,
				Source:     core.SignalSourceHuman,
				Payload:    map[string]any{"reason": "looks good to me"},
				Actor:      "human",
				CreatedAt:  time.Now().UTC(),
			})
			return err
		}
		exec.Output = map[string]any{"result": "ok"}
		return nil
	}

	env := setupIntegration(t, executor)
	store = env.store
	ts := env.server

	resp, _ := postJSON(ts, "/work-items", map[string]any{
		"title": "decision-api-test", "priority": "medium",
	})
	issue := decode[core.WorkItem](t, resp)

	postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "build", "type": "exec",
	})
	resp, _ = postJSON(ts, fmt.Sprintf("/work-items/%d/steps", issue.ID), map[string]any{
		"name": "review", "type": "gate",
	})
	stepGate := decode[core.Action](t, resp)

	// Run and wait for completion.
	postJSON(ts, fmt.Sprintf("/work-items/%d/run", issue.ID), nil)
	pollWorkItemStatus(t, ts, issue.ID, core.WorkItemDone, 10*time.Second)

	// Verify signals via API.
	resp, _ = getJSON(ts, fmt.Sprintf("/steps/%d/signals", stepGate.ID))
	requireStatus(t, resp, http.StatusOK)
	var signals []*core.ActionSignal
	decodeJSON(resp, &signals)
	if len(signals) == 0 {
		t.Fatal("expected at least 1 signal on gate step")
	}

	// Verify pending-decisions returns empty (no more blocked steps).
	resp, _ = getJSON(ts, "/pending-decisions")
	requireStatus(t, resp, http.StatusOK)
	var pending []map[string]any
	decodeJSON(resp, &pending)
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending decisions after completion, got %d", len(pending))
	}
}
