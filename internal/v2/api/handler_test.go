package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/yoke233/ai-workflow/internal/v2/core"
	"github.com/yoke233/ai-workflow/internal/v2/engine"
	v2sandbox "github.com/yoke233/ai-workflow/internal/v2/sandbox"
	"github.com/yoke233/ai-workflow/internal/v2/store/sqlite"
)

func setupAPI(t *testing.T) (*Handler, *httptest.Server) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	bus := engine.NewMemBus()

	executor := func(_ context.Context, step *core.Step, exec *core.Execution) error {
		return nil // noop executor for API tests
	}
	eng := engine.New(store, bus, executor, engine.WithConcurrency(2))

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

func decodeJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// ---------------------------------------------------------------------------
// Flow CRUD Tests
// ---------------------------------------------------------------------------

func TestAPI_CreateFlow(t *testing.T) {
	_, ts := setupAPI(t)

	resp, err := post(ts, "/flows", map[string]any{
		"name":     "test-flow",
		"metadata": map[string]string{"env": "test"},
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var flow core.Flow
	if err := decodeJSON(resp, &flow); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if flow.Name != "test-flow" {
		t.Fatalf("expected name test-flow, got %s", flow.Name)
	}
	if flow.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if flow.Status != core.FlowPending {
		t.Fatalf("expected pending, got %s", flow.Status)
	}
}

func TestAPI_CreateFlow_Validation(t *testing.T) {
	_, ts := setupAPI(t)

	// Missing name.
	resp, _ := post(ts, "/flows", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAPI_GetFlow(t *testing.T) {
	_, ts := setupAPI(t)

	// Create flow.
	resp, _ := post(ts, "/flows", map[string]any{"name": "get-test"})
	var created core.Flow
	decodeJSON(resp, &created)

	// Get flow.
	resp, _ = get(ts, fmt.Sprintf("/flows/%d", created.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got core.Flow
	decodeJSON(resp, &got)
	if got.Name != "get-test" {
		t.Fatalf("expected name get-test, got %s", got.Name)
	}
}

func TestAPI_GetFlow_NotFound(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := get(ts, "/flows/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAPI_ListFlows(t *testing.T) {
	_, ts := setupAPI(t)

	post(ts, "/flows", map[string]any{"name": "flow-1"})
	post(ts, "/flows", map[string]any{"name": "flow-2"})

	resp, _ := get(ts, "/flows")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var flows []*core.Flow
	decodeJSON(resp, &flows)
	if len(flows) != 2 {
		t.Fatalf("expected 2 flows, got %d", len(flows))
	}
}

func TestAPI_ListFlows_FilterStatus(t *testing.T) {
	_, ts := setupAPI(t)

	post(ts, "/flows", map[string]any{"name": "f1"})
	post(ts, "/flows", map[string]any{"name": "f2"})

	resp, _ := get(ts, "/flows?status=pending")
	var flows []*core.Flow
	decodeJSON(resp, &flows)
	if len(flows) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(flows))
	}

	resp, _ = get(ts, "/flows?status=running")
	decodeJSON(resp, &flows)
	if len(flows) != 0 {
		t.Fatalf("expected 0 running, got %d", len(flows))
	}
}

// ---------------------------------------------------------------------------
// Step CRUD Tests
// ---------------------------------------------------------------------------

func TestAPI_CreateStep(t *testing.T) {
	_, ts := setupAPI(t)

	// Create flow first.
	resp, _ := post(ts, "/flows", map[string]any{"name": "flow"})
	var flow core.Flow
	decodeJSON(resp, &flow)

	// Create step.
	resp, _ = post(ts, fmt.Sprintf("/flows/%d/steps", flow.ID), map[string]any{
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

	resp, _ := post(ts, "/flows", map[string]any{"name": "flow"})
	var flow core.Flow
	decodeJSON(resp, &flow)

	post(ts, fmt.Sprintf("/flows/%d/steps", flow.ID), map[string]any{"name": "A", "type": "exec"})
	post(ts, fmt.Sprintf("/flows/%d/steps", flow.ID), map[string]any{"name": "B", "type": "gate"})

	resp, _ = get(ts, fmt.Sprintf("/flows/%d/steps", flow.ID))
	var steps []*core.Step
	decodeJSON(resp, &steps)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
}

func TestAPI_GetStep(t *testing.T) {
	_, ts := setupAPI(t)

	resp, _ := post(ts, "/flows", map[string]any{"name": "flow"})
	var flow core.Flow
	decodeJSON(resp, &flow)

	resp, _ = post(ts, fmt.Sprintf("/flows/%d/steps", flow.ID), map[string]any{"name": "A", "type": "exec"})
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
// Run & Cancel Flow Tests
// ---------------------------------------------------------------------------

func TestAPI_RunFlow(t *testing.T) {
	_, ts := setupAPI(t)

	// Create flow + step.
	resp, _ := post(ts, "/flows", map[string]any{"name": "run-test"})
	var flow core.Flow
	decodeJSON(resp, &flow)

	post(ts, fmt.Sprintf("/flows/%d/steps", flow.ID), map[string]any{"name": "A", "type": "exec"})

	// Run flow.
	resp, _ = post(ts, fmt.Sprintf("/flows/%d/run", flow.ID), nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Wait for async completion.
	time.Sleep(500 * time.Millisecond)

	// Verify flow is done.
	resp, _ = get(ts, fmt.Sprintf("/flows/%d", flow.ID))
	var done core.Flow
	decodeJSON(resp, &done)
	if done.Status != core.FlowDone {
		t.Fatalf("expected done, got %s", done.Status)
	}
}

func TestAPI_RunFlow_NotPending(t *testing.T) {
	_, ts := setupAPI(t)

	// Create and run flow.
	resp, _ := post(ts, "/flows", map[string]any{"name": "run-twice"})
	var flow core.Flow
	decodeJSON(resp, &flow)
	post(ts, fmt.Sprintf("/flows/%d/steps", flow.ID), map[string]any{"name": "A", "type": "exec"})
	post(ts, fmt.Sprintf("/flows/%d/run", flow.ID), nil)

	// Wait for flow to complete.
	time.Sleep(500 * time.Millisecond)

	// Verify flow is done.
	resp, _ = get(ts, fmt.Sprintf("/flows/%d", flow.ID))
	decodeJSON(resp, &flow)
	if flow.Status != core.FlowDone {
		t.Fatalf("expected done after first run, got %s", flow.Status)
	}

	// Try to run again — should fail since it's not pending.
	resp, _ = post(ts, fmt.Sprintf("/flows/%d/run", flow.ID), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestAPI_CancelFlow(t *testing.T) {
	h, ts := setupAPI(t)

	resp, _ := post(ts, "/flows", map[string]any{"name": "cancel-test"})
	var flow core.Flow
	decodeJSON(resp, &flow)

	// Manually set flow to running for cancel test.
	h.store.UpdateFlowStatus(context.Background(), flow.ID, core.FlowRunning)

	resp, _ = post(ts, fmt.Sprintf("/flows/%d/cancel", flow.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp, _ = get(ts, fmt.Sprintf("/flows/%d", flow.ID))
	var cancelled core.Flow
	decodeJSON(resp, &cancelled)
	if cancelled.Status != core.FlowCancelled {
		t.Fatalf("expected cancelled, got %s", cancelled.Status)
	}
}

// ---------------------------------------------------------------------------
// Events Tests
// ---------------------------------------------------------------------------

func TestAPI_ListEvents(t *testing.T) {
	_, ts := setupAPI(t)

	// Create flow + step + run to generate events.
	resp, _ := post(ts, "/flows", map[string]any{"name": "events-test"})
	var flow core.Flow
	decodeJSON(resp, &flow)
	post(ts, fmt.Sprintf("/flows/%d/steps", flow.ID), map[string]any{"name": "A", "type": "exec"})
	post(ts, fmt.Sprintf("/flows/%d/run", flow.ID), nil)
	time.Sleep(500 * time.Millisecond)

	// List all events.
	resp, _ = get(ts, "/events")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// List events filtered by flow.
	resp, _ = get(ts, fmt.Sprintf("/flows/%d/events", flow.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for flow events, got %d", resp.StatusCode)
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
		Type:      core.EventFlowStarted,
		FlowID:    42,
		Timestamp: time.Now().UTC(),
	})

	// Read event from WebSocket.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ev core.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev.Type != core.EventFlowStarted {
		t.Fatalf("expected flow.started, got %s", ev.Type)
	}
	if ev.FlowID != 42 {
		t.Fatalf("expected flow_id=42, got %d", ev.FlowID)
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
		OS               string `json:"os"`
		Arch             string `json:"arch"`
		Enabled          bool   `json:"enabled"`
		CurrentProvider  string `json:"current_provider"`
		CurrentSupported bool   `json:"current_supported"`
		Providers        map[string]struct {
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
	if got.CurrentSupported {
		t.Fatal("current_supported = true, want false for disabled sandbox")
	}
	if !got.Providers["home_dir"].Supported {
		t.Fatal("home_dir should be reported as supported")
	}
}

// ---------------------------------------------------------------------------
// E2E API Test: Create flow + steps → run → verify all entities
// ---------------------------------------------------------------------------

func TestAPI_E2E_FlowLifecycle(t *testing.T) {
	_, ts := setupAPI(t)

	// 1. Create flow.
	resp, _ := post(ts, "/flows", map[string]any{"name": "e2e-api"})
	var flow core.Flow
	decodeJSON(resp, &flow)

	// 2. Create steps: A → B.
	resp, _ = post(ts, fmt.Sprintf("/flows/%d/steps", flow.ID), map[string]any{
		"name": "A", "type": "exec",
	})
	var stepA core.Step
	decodeJSON(resp, &stepA)

	resp, _ = post(ts, fmt.Sprintf("/flows/%d/steps", flow.ID), map[string]any{
		"name": "B", "type": "exec", "depends_on": []int64{stepA.ID},
	})
	var stepB core.Step
	decodeJSON(resp, &stepB)

	// 3. List steps.
	resp, _ = get(ts, fmt.Sprintf("/flows/%d/steps", flow.ID))
	var steps []*core.Step
	decodeJSON(resp, &steps)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	// 4. Run flow.
	resp, _ = post(ts, fmt.Sprintf("/flows/%d/run", flow.ID), nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	time.Sleep(500 * time.Millisecond)

	// 5. Verify flow done.
	resp, _ = get(ts, fmt.Sprintf("/flows/%d", flow.ID))
	decodeJSON(resp, &flow)
	if flow.Status != core.FlowDone {
		t.Fatalf("expected done, got %s", flow.Status)
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
	resp, _ = get(ts, fmt.Sprintf("/flows/%d/events", flow.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for events, got %d", resp.StatusCode)
	}
}
