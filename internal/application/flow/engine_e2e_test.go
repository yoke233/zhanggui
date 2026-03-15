package flow

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// TestE2E_WorkItemResolverInputCollector covers resolver, input builder, and collector wiring.
func TestE2E_WorkItemResolverInputCollector(t *testing.T) {
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
	if !strings.Contains(capturedInput, "Build login API") {
		t.Fatalf("expected input snapshot to contain objective, got %q", capturedInput)
	}
	if !strings.Contains(capturedInput, "design output") {
		t.Fatalf("expected input snapshot to contain upstream deliverable content, got %q", capturedInput)
	}

	designRun, _ := store.GetLatestRunWithResult(ctx, designID)
	if designRun.ResultMetadata["collected"] != true {
		t.Fatalf("design run metadata not collected: %v", designRun.ResultMetadata)
	}

	implRun, _ := store.GetLatestRunWithResult(ctx, implID)
	if implRun.ResultMetadata["collected"] != true {
		t.Fatalf("implement run metadata not collected: %v", implRun.ResultMetadata)
	}
}

// TestE2E_WorkItemGateRejectRetryWithCollector covers reject -> retry -> pass with collector output.
func TestE2E_WorkItemGateRejectRetryWithCollector(t *testing.T) {
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

// TestE2E_WorkItemCompositeWithGate covers a plan action whose child work item contains a gate.
func TestE2E_WorkItemCompositeWithGate(t *testing.T) {
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

// TestE2E_WorkItemFanOutMerge covers a full multi-step execution chain.
func TestE2E_WorkItemFanOutMerge(t *testing.T) {
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

// TestE2E_WorkItemTimeoutRetryGatePass covers timeout retry followed by gate approval.
func TestE2E_WorkItemTimeoutRetryGatePass(t *testing.T) {
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

// TestE2E_WorkItemPermanentErrorStopsWorkItem covers permanent errors skipping retries.
func TestE2E_WorkItemPermanentErrorStopsWorkItem(t *testing.T) {
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

// TestE2E_WorkItemFullOrchestration covers a design -> plan -> gate -> deploy pipeline.
func TestE2E_WorkItemFullOrchestration(t *testing.T) {
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

	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Fatalf("expected done, got %s", workItem.Status)
	}

	for _, id := range []int64{implID, deployID} {
		a, _ := store.GetAction(ctx, id)
		if a.Status != core.ActionDone {
			t.Fatalf("action %s (id=%d) expected done, got %s", a.Name, id, a.Status)
		}
	}

	if gateCount != 2 {
		t.Fatalf("expected 2 gate evaluations, got %d", gateCount)
	}

	implAction, _ := store.GetAction(ctx, implID)
	if childWorkItemID(implAction) == nil {
		t.Fatal("expected impl to have child_work_item_id")
	}

	deployRun, _ := store.GetLatestRunWithResult(ctx, deployID)
	if deployRun == nil {
		t.Fatal("expected deploy run with result")
	}
	if deployRun.ResultMetadata["collected"] != true {
		t.Fatalf("expected collected metadata on deploy, got %v", deployRun.ResultMetadata)
	}

	if designCount != 1 {
		t.Fatalf("expected 1 design run, got %d", designCount)
	}
}
