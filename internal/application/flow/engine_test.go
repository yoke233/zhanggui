package flow

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

func setup(t *testing.T) (core.Store, core.EventBus) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, NewMemBus()
}

// TestLinearFlow: A → B → C, all succeed (sequential by Position).
func TestLinearFlow(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var callOrder []string
	var counter int32
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		atomic.AddInt32(&counter, 1)
		callOrder = append(callOrder, action.Name)
		run.Output = map[string]any{"ok": true}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	// Create work item + actions.
	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "linear", Status: core.WorkItemOpen})

	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "B", Type: core.ActionExec, Status: core.ActionPending, Position: 1})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "C", Type: core.ActionExec, Status: core.ActionPending, Position: 2})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	if counter != 3 {
		t.Fatalf("expected 3 executions, got %d", counter)
	}
	if callOrder[0] != "A" || callOrder[1] != "B" || callOrder[2] != "C" {
		t.Fatalf("unexpected order: %v", callOrder)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}
}

// TestSequentialPositions: actions with unique positions all execute successfully.
func TestParallelFanOut(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var counter int32
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		atomic.AddInt32(&counter, 1)
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(4))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "fanout", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "B", Type: core.ActionExec, Status: core.ActionPending, Position: 1})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "C", Type: core.ActionExec, Status: core.ActionPending, Position: 2})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}
	if counter != 3 {
		t.Fatalf("expected 3 executions, got %d", counter)
	}
}

// TestActionFailure: A fails, work item fails.
func TestActionFailure(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		return fmt.Errorf("boom")
	}

	eng := New(store, bus, executor)

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "fail", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0})

	err := eng.Run(ctx, workItemID)
	if err == nil {
		t.Fatal("expected error")
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemFailed {
		t.Fatalf("expected failed, got %s", workItem.Status)
	}
}

// TestRetry: action fails once, retries, succeeds.
func TestRetry(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var attempts int32
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			return fmt.Errorf("transient")
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "retry", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0, MaxRetries: 1})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

// TestCancelWorkItem: cancel a running work item.
func TestCancelWorkItem(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	eng := New(store, bus, nil)

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "cancel-test", Status: core.WorkItemOpen})
	_ = store.UpdateWorkItemStatus(ctx, workItemID, core.WorkItemRunning) // simulate running

	if err := eng.Cancel(ctx, workItemID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemCancelled {
		t.Fatalf("expected cancelled, got %s", workItem.Status)
	}
}

// TestRetryPersistence: verify retry_count is persisted, preventing infinite retries.
func TestRetryPersistence(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		return fmt.Errorf("always fail")
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "retry-persist", Status: core.WorkItemOpen})
	aID, _ := store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0, MaxRetries: 2})

	// Should fail after 3 attempts (1 original + 2 retries).
	err := eng.Run(ctx, workItemID)
	if err == nil {
		t.Fatal("expected error")
	}

	// Verify retry_count was persisted.
	action, _ := store.GetAction(ctx, aID)
	if action.RetryCount != 2 {
		t.Fatalf("expected retry_count=2, got %d", action.RetryCount)
	}
}

// TestGateAutoPass: exec → gate(pass) → work item done.
func TestGateAutoPass(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		if action.Type == core.ActionGate {
			run.ResultMarkdown = "LGTM, all tests pass."
			run.ResultMetadata = map[string]any{"verdict": "pass"}
			return nil
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "gate-pass", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "impl", Type: core.ActionExec, Status: core.ActionPending, Position: 0})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "review", Type: core.ActionGate, Status: core.ActionPending, Position: 1})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}
	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}
}

// TestGateAutoReject: exec → gate(reject) → exec retries → gate(pass) → work item done.
func TestGateAutoReject(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var gateCount int32
	var execCount int32
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		if action.Type == core.ActionGate {
			n := atomic.AddInt32(&gateCount, 1)
			verdict := "reject"
			if n > 1 {
				verdict = "pass"
			}
			run.ResultMarkdown = "Review result"
			run.ResultMetadata = map[string]any{"verdict": verdict, "reason": "needs improvement"}
			return nil
		}
		atomic.AddInt32(&execCount, 1)
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "gate-reject", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "impl", Type: core.ActionExec, Status: core.ActionPending, Position: 0, MaxRetries: 1})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "review", Type: core.ActionGate, Status: core.ActionPending, Position: 1})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}
	if gateCount != 2 {
		t.Fatalf("expected 2 gate evaluations, got %d", gateCount)
	}
	if execCount != 2 {
		t.Fatalf("expected 2 exec runs, got %d", execCount)
	}
}

// TestActionTimeout: action times out on first attempt, retries, succeeds.
func TestActionTimeout(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var attempts int32
	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			select {
			case <-time.After(500 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "timeout", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "slow",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
		Timeout:    50 * time.Millisecond,
		MaxRetries: 1,
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

// TestErrorKindPermanent: permanent error skips retry despite MaxRetries > 0.
func TestErrorKindPermanent(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		run.ErrorKind = core.ErrKindPermanent
		return fmt.Errorf("fatal: invalid configuration")
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "permanent", Status: core.WorkItemOpen})
	aID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "A",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
		MaxRetries: 5,
	})

	err := eng.Run(ctx, workItemID)
	if err == nil {
		t.Fatal("expected error")
	}

	action, _ := store.GetAction(ctx, aID)
	if action.RetryCount != 0 {
		t.Fatalf("expected retry_count=0 (permanent skips retry), got %d", action.RetryCount)
	}
	if action.Status != core.ActionFailed {
		t.Fatalf("expected failed, got %s", action.Status)
	}
}

// TestProfileRegistry: resolve by role + capabilities.
func TestProfileRegistry(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "claude-worker", Role: core.RoleWorker, Capabilities: []string{"backend", "frontend"}},
		{ID: "claude-gate", Role: core.RoleGate, Capabilities: []string{"review"}},
		{ID: "codex-worker", Role: core.RoleWorker, Capabilities: []string{"backend", "qa"}},
	}
	reg := NewProfileRegistry(profiles)
	ctx := context.Background()

	// Match role + capability.
	id, err := reg.Resolve(ctx, &core.Action{AgentRole: "worker", RequiredCapabilities: []string{"qa"}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if id != "codex-worker" {
		t.Fatalf("expected codex-worker, got %s", id)
	}

	// Match role only (no capability filter).
	id, err = reg.Resolve(ctx, &core.Action{AgentRole: "gate"})
	if err != nil {
		t.Fatalf("resolve gate: %v", err)
	}
	if id != "claude-gate" {
		t.Fatalf("expected claude-gate, got %s", id)
	}

	// No role filter — first match.
	id, err = reg.Resolve(ctx, &core.Action{})
	if err != nil {
		t.Fatalf("resolve any: %v", err)
	}
	if id != "claude-worker" {
		t.Fatalf("expected claude-worker, got %s", id)
	}

	// No match.
	_, err = reg.Resolve(ctx, &core.Action{AgentRole: "worker", RequiredCapabilities: []string{"k8s"}})
	if err != core.ErrNoMatchingAgent {
		t.Fatalf("expected ErrNoMatchingAgent, got %v", err)
	}
}

// TestInputBuilder: assembles input from upstream deliverables.
func TestInputBuilder(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	// Create a work item with A → B (by Position).
	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "input-test", Status: core.WorkItemOpen})
	aID, _ := store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionDone, Position: 0})
	bID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID:         workItemID,
		Name:               "B",
		Type:               core.ActionExec,
		Status:             core.ActionPending,
		Position:           1,
		AcceptanceCriteria: []string{"must pass lint", "must have tests"},
		Config:             map[string]any{"objective": "Implement login endpoint"},
	})

	// A has a result.
	rID, _ := store.CreateRun(ctx, &core.Run{ActionID: aID, WorkItemID: workItemID, Status: core.RunSucceeded, Attempt: 1})
	aRun, _ := store.GetRun(ctx, rID)
	aRun.ResultMarkdown = "## Design\nAPI design for login."
	store.UpdateRun(ctx, aRun)

	builder := NewInputBuilder(store)
	actionB, _ := store.GetAction(ctx, bID)
	input, err := builder.Build(ctx, actionB)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if !strings.Contains(input, "Implement login endpoint") {
		t.Fatalf("expected objective in input, got %q", input)
	}
	if !strings.Contains(input, "Acceptance Criteria") {
		t.Fatalf("expected acceptance criteria in input, got %q", input)
	}
	if !strings.Contains(input, "API design for login.") {
		t.Fatalf("expected upstream deliverable content in input, got %q", input)
	}

	_ = bus // satisfy usage
}

// TestCollectorWiring: collector extracts metadata into deliverable after success.
func TestCollectorWiring(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	// Collector that extracts a "summary" field.
	collector := CollectorFunc(func(_ context.Context, actionType core.ActionType, markdown string) (map[string]any, error) {
		return map[string]any{"summary": "extracted from: " + string(actionType)}, nil
	})

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		// Simulate agent producing a result.
		run.ResultMarkdown = "## Implementation\nDid the thing."
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1), WithCollector(collector))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "collector-test", Status: core.WorkItemOpen})
	aID, _ := store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	del, err := store.GetLatestRunWithResult(ctx, aID)
	if err != nil {
		t.Fatalf("get run with result: %v", err)
	}
	if del.ResultMetadata["summary"] != "extracted from: exec" {
		t.Fatalf("expected extracted metadata, got %v", del.ResultMetadata)
	}
}

// TestResolverIntegration: engine uses resolver to set agent_id on run.
func TestResolverIntegration(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	profiles := []*core.AgentProfile{
		{ID: "my-worker", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}

	var capturedAgentID string
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		capturedAgentID = run.AgentID
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1), WithResolver(NewProfileRegistry(profiles)))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "resolver-test", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{
		WorkItemID:           workItemID,
		Name:                 "build",
		Type:                 core.ActionExec,
		Status:               core.ActionPending,
		Position:             0,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"go"},
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedAgentID != "my-worker" {
		t.Fatalf("expected agent_id=my-worker, got %q", capturedAgentID)
	}
}

// TestEventBus: subscribe and receive events.
func TestEventBus(t *testing.T) {
	bus := NewMemBus()
	ctx := context.Background()

	sub := bus.Subscribe(core.SubscribeOpts{
		Types:      []core.EventType{core.EventWorkItemStarted},
		BufferSize: 8,
	})
	defer sub.Cancel()

	bus.Publish(ctx, core.Event{Type: core.EventWorkItemStarted, WorkItemID: 1})
	bus.Publish(ctx, core.Event{Type: core.EventActionReady, WorkItemID: 1})      // should be filtered out
	bus.Publish(ctx, core.Event{Type: core.EventWorkItemStarted, WorkItemID: 2}) // should be received

	ev := <-sub.C
	if ev.WorkItemID != 1 {
		t.Fatalf("expected work item 1, got %d", ev.WorkItemID)
	}
	ev = <-sub.C
	if ev.WorkItemID != 2 {
		t.Fatalf("expected work item 2, got %d", ev.WorkItemID)
	}
}

// ---------------------------------------------------------------------------
// Composite Action Tests
// ---------------------------------------------------------------------------

// TestCompositeSimple: A(exec) → B(plan[B1,B2]) → C(exec), all succeed.
func TestCompositeSimple(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var callOrder []string
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		callOrder = append(callOrder, action.Name)
		return nil
	}

	// Expander returns two child actions: B1 and B2.
	expander := ExpanderFunc(func(_ context.Context, action *core.Action) ([]*core.Action, error) {
		b1 := &core.Action{Name: "B1", Type: core.ActionExec}
		b2 := &core.Action{Name: "B2", Type: core.ActionExec}
		return []*core.Action{b1, b2}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "composite-simple", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0})
	bID, _ := store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "B", Type: core.ActionPlan, Status: core.ActionPending, Position: 1})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "C", Type: core.ActionExec, Status: core.ActionPending, Position: 2})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// A runs first, then B expands (B1, B2 run in child work item), then C.
	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}

	// Verify A ran, B1/B2 ran inside child work item, then C ran.
	if len(callOrder) != 4 {
		t.Fatalf("expected 4 executor calls (A, B1, B2, C), got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "A" {
		t.Fatalf("expected A first, got %s", callOrder[0])
	}
	if callOrder[3] != "C" {
		t.Fatalf("expected C last, got %s", callOrder[3])
	}

	// B should have child_work_item_id in Config.
	actionB, _ := store.GetAction(ctx, bID)
	childID := childWorkItemID(actionB)
	if childID == nil {
		t.Fatal("expected B to have child_work_item_id in Config")
	}
	if actionB.Status != core.ActionDone {
		t.Fatalf("expected B done, got %s", actionB.Status)
	}

	// Child work item should also be done.
	childWI, _ := store.GetWorkItem(ctx, *childID)
	if childWI.Status != core.WorkItemDone {
		t.Fatalf("expected child work item done, got %s", childWI.Status)
	}
}

// TestCompositeChainedChildren: plan with sequential children B1 → B2.
func TestCompositeChainedChildren(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var callOrder []string
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		callOrder = append(callOrder, action.Name)
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, action *core.Action) ([]*core.Action, error) {
		return []*core.Action{
			{Name: "B1", Type: core.ActionExec},
			{Name: "B2", Type: core.ActionExec},
		}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "composite-chain", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "B", Type: core.ActionPlan, Status: core.ActionPending, Position: 0})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}
	if len(callOrder) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(callOrder), callOrder)
	}
}

// TestCompositeSubFlowFail: plan child fails → plan fails → parent work item fails.
func TestCompositeSubFlowFail(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		if action.Name == "child-bad" {
			return fmt.Errorf("child failure")
		}
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, action *core.Action) ([]*core.Action, error) {
		return []*core.Action{
			{Name: "child-bad", Type: core.ActionExec},
		}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "composite-fail", Status: core.WorkItemOpen})
	compID, _ := store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "comp", Type: core.ActionPlan, Status: core.ActionPending, Position: 0})

	err := eng.Run(ctx, workItemID)
	if err == nil {
		t.Fatal("expected error from child work item failure")
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemFailed {
		t.Fatalf("expected failed, got %s", workItem.Status)
	}

	compAction, _ := store.GetAction(ctx, compID)
	if compAction.Status != core.ActionFailed {
		t.Fatalf("expected plan action failed, got %s", compAction.Status)
	}
}

// TestCompositeRetry: plan child work item fails once, plan retries with fresh child work item, succeeds.
func TestCompositeRetry(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var expandCount int32
	var execCount int32

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		n := atomic.AddInt32(&execCount, 1)
		// First child execution fails, second succeeds.
		if n == 1 {
			return fmt.Errorf("transient child failure")
		}
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, action *core.Action) ([]*core.Action, error) {
		atomic.AddInt32(&expandCount, 1)
		return []*core.Action{
			{Name: "child", Type: core.ActionExec},
		}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "composite-retry", Status: core.WorkItemOpen})
	compID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "comp",
		Type:       core.ActionPlan,
		Status:     core.ActionPending,
		Position:   0,
		MaxRetries: 1,
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}

	compAction, _ := store.GetAction(ctx, compID)
	if compAction.Status != core.ActionDone {
		t.Fatalf("expected plan done, got %s", compAction.Status)
	}
	if compAction.RetryCount != 1 {
		t.Fatalf("expected retry_count=1, got %d", compAction.RetryCount)
	}

	// Expander should have been called twice (original + retry).
	if expandCount != 2 {
		t.Fatalf("expected 2 expansions, got %d", expandCount)
	}
}

// ---------------------------------------------------------------------------
// WorkItem Integration Tests — cross-cutting scenarios
// ---------------------------------------------------------------------------

// TestWorkItemE2E_ResolverInputCollector: full pipeline with all 3 injectable interfaces.
func TestWorkItemE2E_ResolverInputCollector(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	profiles := []*core.AgentProfile{
		{ID: "designer", Role: core.RoleWorker, Capabilities: []string{"design"}},
		{ID: "coder", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}

	collector := CollectorFunc(func(_ context.Context, actionType core.ActionType, md string) (map[string]any, error) {
		return map[string]any{"collected": true, "type": string(actionType)}, nil
	})

	var capturedInput string
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		if action.Name == "implement" {
			capturedInput = run.BriefingSnapshot
		}
		// Every action produces a result.
		run.ResultMarkdown = fmt.Sprintf("## %s output\nDone.", action.Name)
		return nil
	}

	eng := New(store, bus, executor,
		WithConcurrency(1),
		WithResolver(NewProfileRegistry(profiles)),
		WithInputBuilder(NewInputBuilder(store)),
		WithCollector(collector),
	)

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "e2e-pipeline", Status: core.WorkItemOpen})
	designID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID:           workItemID,
		Name:                 "design",
		Type:                 core.ActionExec,
		Status:               core.ActionPending,
		Position:             0,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"design"},
	})
	implID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID:           workItemID,
		Name:                 "implement",
		Type:                 core.ActionExec,
		Status:               core.ActionPending,
		Position:             1,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"go"},
		Config:               map[string]any{"objective": "Build login API"},
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}

	// Verify input was assembled with upstream deliverable content.
	if !strings.Contains(capturedInput, "Build login API") {
		t.Fatalf("expected input snapshot to contain objective, got %q", capturedInput)
	}
	if !strings.Contains(capturedInput, "design output") {
		t.Fatalf("expected input snapshot to contain upstream deliverable content, got %q", capturedInput)
	}

	// Verify collector extracted metadata into both runs.
	designRun, _ := store.GetLatestRunWithResult(ctx, designID)
	if designRun.ResultMetadata["collected"] != true {
		t.Fatalf("design run metadata not collected: %v", designRun.ResultMetadata)
	}

	implRun, _ := store.GetLatestRunWithResult(ctx, implID)
	if implRun.ResultMetadata["collected"] != true {
		t.Fatalf("implement run metadata not collected: %v", implRun.ResultMetadata)
	}
}

// TestWorkItemE2E_GateRejectRetryWithCollector: full gate reject → retry → pass cycle.
func TestWorkItemE2E_GateRejectRetryWithCollector(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var gateCount int32
	var implCount int32
	var deployCount int32

	collector := CollectorFunc(func(_ context.Context, actionType core.ActionType, md string) (map[string]any, error) {
		return map[string]any{"action_type": string(actionType)}, nil
	})

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		if action.Type == core.ActionGate {
			n := atomic.AddInt32(&gateCount, 1)
			verdict := "reject"
			if n > 1 {
				verdict = "pass"
			}
			run.ResultMarkdown = "Review feedback"
			run.ResultMetadata = map[string]any{"verdict": verdict, "reason": "iteration " + fmt.Sprint(n)}
			return nil
		}
		if action.Name == "impl" {
			atomic.AddInt32(&implCount, 1)
		} else if action.Name == "deploy" {
			atomic.AddInt32(&deployCount, 1)
		}
		run.ResultMarkdown = fmt.Sprintf("## %s output", action.Name)
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1), WithCollector(collector))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "e2e-gate-retry", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "impl",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
		MaxRetries: 1,
	})
	store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "review",
		Type:       core.ActionGate,
		Status:     core.ActionPending,
		Position:   1,
	})
	deployID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "deploy",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   2,
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}

	if implCount != 2 {
		t.Fatalf("expected 2 impl runs, got %d", implCount)
	}
	if gateCount != 2 {
		t.Fatalf("expected 2 gate evaluations, got %d", gateCount)
	}
	if deployCount != 1 {
		t.Fatalf("expected 1 deploy run, got %d", deployCount)
	}

	deployAction, _ := store.GetAction(ctx, deployID)
	if deployAction.Status != core.ActionDone {
		t.Fatalf("expected deploy done, got %s", deployAction.Status)
	}

	deployRun, _ := store.GetLatestRunWithResult(ctx, deployID)
	if deployRun.ResultMetadata["action_type"] != "exec" {
		t.Fatalf("deploy run missing collected metadata: %v", deployRun.ResultMetadata)
	}
}

// TestWorkItemE2E_CompositeWithGate: plan containing a gate inside its child work item.
func TestWorkItemE2E_CompositeWithGate(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var callOrder []string
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		callOrder = append(callOrder, action.Name)
		if action.Type == core.ActionGate {
			run.ResultMarkdown = "Gate pass"
			run.ResultMetadata = map[string]any{"verdict": "pass"}
			return nil
		}
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, action *core.Action) ([]*core.Action, error) {
		return []*core.Action{
			{Name: "B1", Type: core.ActionExec},
			{Name: "B2", Type: core.ActionGate},
		}, nil
	})

	eng := New(store, bus, executor, WithConcurrency(1), WithExpander(expander))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "e2e-composite-gate", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "B", Type: core.ActionPlan, Status: core.ActionPending, Position: 1})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "C", Type: core.ActionExec, Status: core.ActionPending, Position: 2})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}

	if len(callOrder) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "A" || callOrder[3] != "C" {
		t.Fatalf("expected A..C ordering, got %v", callOrder)
	}
}

// TestWorkItemE2E_FanOutMerge: unique positions execute sequentially until completion.
func TestWorkItemE2E_FanOutMerge(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var counter int32
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		atomic.AddInt32(&counter, 1)
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(4))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "e2e-fan-merge", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "B", Type: core.ActionExec, Status: core.ActionPending, Position: 1})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "C", Type: core.ActionExec, Status: core.ActionPending, Position: 2})
	dID, _ := store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "D", Type: core.ActionExec, Status: core.ActionPending, Position: 3})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}
	if counter != 4 {
		t.Fatalf("expected 4 executions, got %d", counter)
	}

	actionD, _ := store.GetAction(ctx, dID)
	if actionD.Status != core.ActionDone {
		t.Fatalf("expected D done, got %s", actionD.Status)
	}
}

// TestWorkItemE2E_TimeoutRetryGatePass: slow action times out → retries → gate passes → done.
func TestWorkItemE2E_TimeoutRetryGatePass(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	var implAttempts int32
	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
		if action.Name == "impl" {
			n := atomic.AddInt32(&implAttempts, 1)
			if n == 1 {
				select {
				case <-time.After(500 * time.Millisecond):
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}
		if action.Type == core.ActionGate {
			run.ResultMarkdown = "Approved"
			run.ResultMetadata = map[string]any{"verdict": "pass"}
			return nil
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(1))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "e2e-timeout-gate", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "impl",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
		Timeout:    50 * time.Millisecond,
		MaxRetries: 1,
	})
	store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "review",
		Type:       core.ActionGate,
		Status:     core.ActionPending,
		Position:   1,
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}
	if implAttempts != 2 {
		t.Fatalf("expected 2 impl attempts, got %d", implAttempts)
	}
}

// TestWorkItemE2E_PermanentErrorStopsWorkItem: action hits permanent error.
func TestWorkItemE2E_PermanentErrorStopsWorkItem(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		if action.Name == "B" {
			run.ErrorKind = core.ErrKindPermanent
			return fmt.Errorf("bad config")
		}
		return nil
	}

	eng := New(store, bus, executor, WithConcurrency(4))

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "e2e-permanent", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "A", Type: core.ActionExec, Status: core.ActionPending, Position: 0})
	bID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "B",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   1,
		MaxRetries: 3,
	})
	store.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "C", Type: core.ActionExec, Status: core.ActionPending, Position: 2})

	err := eng.Run(ctx, workItemID)
	if err == nil {
		t.Fatal("expected error")
	}

	actionB, _ := store.GetAction(ctx, bID)
	if actionB.RetryCount != 0 {
		t.Fatalf("permanent error should skip retry, got retry_count=%d", actionB.RetryCount)
	}
	if actionB.Status != core.ActionFailed {
		t.Fatalf("expected B failed, got %s", actionB.Status)
	}
}

// TestWorkItemE2E_FullOrchestration: all features in a single work item.
// WorkItem: design(exec) → impl(plan[code,test]) → review(gate,reject→pass) → deploy(exec)
func TestWorkItemE2E_FullOrchestration(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	profiles := []*core.AgentProfile{
		{ID: "architect", Role: core.RoleWorker, Capabilities: []string{"design"}},
		{ID: "coder", Role: core.RoleWorker, Capabilities: []string{"go"}},
		{ID: "reviewer", Role: core.RoleGate, Capabilities: []string{"review"}},
		{ID: "deployer", Role: core.RoleWorker, Capabilities: []string{"deploy"}},
	}

	collector := CollectorFunc(func(_ context.Context, actionType core.ActionType, md string) (map[string]any, error) {
		return map[string]any{"collected": true}, nil
	})

	var gateCount int32
	var designCount int32
	executor := func(_ context.Context, action *core.Action, run *core.Run) error {
		switch action.Name {
		case "design":
			atomic.AddInt32(&designCount, 1)
			run.ResultMarkdown = "## Architecture\nLogin API with JWT."
			return nil
		case "code", "test":
			run.ResultMarkdown = fmt.Sprintf("## %s\nDone.", action.Name)
			return nil
		case "review":
			n := atomic.AddInt32(&gateCount, 1)
			verdict := "reject"
			if n > 1 {
				verdict = "pass"
			}
			run.ResultMarkdown = "Review feedback"
			run.ResultMetadata = map[string]any{"verdict": verdict, "reason": "round " + fmt.Sprint(n)}
			return nil
		case "deploy":
			run.ResultMarkdown = "## Deploy\nDeployed to staging."
			return nil
		}
		return nil
	}

	expander := ExpanderFunc(func(_ context.Context, action *core.Action) ([]*core.Action, error) {
		return []*core.Action{
			{Name: "code", Type: core.ActionExec, AgentRole: "worker", RequiredCapabilities: []string{"go"}},
			{Name: "test", Type: core.ActionExec, AgentRole: "worker", RequiredCapabilities: []string{"go"}},
		}, nil
	})

	eng := New(store, bus, executor,
		WithConcurrency(2),
		WithResolver(NewProfileRegistry(profiles)),
		WithInputBuilder(NewInputBuilder(store)),
		WithCollector(collector),
		WithExpander(expander),
	)

	workItemID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "e2e-full", Status: core.WorkItemOpen})
	store.CreateAction(ctx, &core.Action{
		WorkItemID:           workItemID,
		Name:                 "design",
		Type:                 core.ActionExec,
		Status:               core.ActionPending,
		Position:             0,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"design"},
	})
	implID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "impl",
		Type:       core.ActionPlan,
		Status:     core.ActionPending,
		Position:   1,
		MaxRetries: 1,
	})
	store.CreateAction(ctx, &core.Action{
		WorkItemID:           workItemID,
		Name:                 "review",
		Type:                 core.ActionGate,
		Status:               core.ActionPending,
		Position:             2,
		AgentRole:            "gate",
		RequiredCapabilities: []string{"review"},
	})
	deployID, _ := store.CreateAction(ctx, &core.Action{
		WorkItemID:           workItemID,
		Name:                 "deploy",
		Type:                 core.ActionExec,
		Status:               core.ActionPending,
		Position:             3,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"deploy"},
	})

	if err := eng.Run(ctx, workItemID); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify work item completed.
	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}

	// Verify all actions done.
	for _, id := range []int64{implID, deployID} {
		a, _ := store.GetAction(ctx, id)
		if a.Status != core.ActionDone {
			t.Fatalf("action %s (id=%d) expected done, got %s", a.Name, id, a.Status)
		}
	}

	// Gate rejected once then passed.
	if gateCount != 2 {
		t.Fatalf("expected 2 gate evaluations, got %d", gateCount)
	}

	// Plan should have been expanded (impl has child_work_item_id).
	implAction, _ := store.GetAction(ctx, implID)
	if childWorkItemID(implAction) == nil {
		t.Fatal("expected impl to have child_work_item_id")
	}

	// Collector should have enriched deploy run result.
	deployRun, _ := store.GetLatestRunWithResult(ctx, deployID)
	if deployRun == nil {
		t.Fatal("expected deploy run with result")
	}
	if deployRun.ResultMetadata["collected"] != true {
		t.Fatalf("expected collected metadata on deploy, got %v", deployRun.ResultMetadata)
	}

	// Design should have run only once.
	if designCount != 1 {
		t.Fatalf("expected 1 design run, got %d", designCount)
	}
}
