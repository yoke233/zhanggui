package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
	"github.com/yoke233/ai-workflow/internal/v2/store/sqlite"
)

func TestFlowScheduler_BasicExecution(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	var executed atomic.Int32
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		executed.Add(1)
		return nil
	}
	eng := New(store, bus, executor)

	// Create a flow with one step.
	flowID := createTestFlow(t, store, "test-flow")
	createTestStep(t, store, flowID, "step-1", core.StepExec)

	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{MaxConcurrentFlows: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	if err := sched.Submit(ctx, flowID); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Wait for execution.
	waitFor(t, func() bool { return executed.Load() >= 1 }, 2*time.Second)

	// Flow should be done.
	f, _ := store.GetFlow(ctx, flowID)
	if f.Status != core.FlowDone {
		t.Errorf("flow status = %s, want done", f.Status)
	}
}

func TestFlowScheduler_ConcurrencyLimit(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	var running atomic.Int32
	var maxSeen atomic.Int32
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		cur := running.Add(1)
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		running.Add(-1)
		return nil
	}
	eng := New(store, bus, executor)

	// Create 4 flows, each with 1 step.
	var flowIDs []int64
	for i := 0; i < 4; i++ {
		fid := createTestFlow(t, store, "flow")
		createTestStep(t, store, fid, "step", core.StepExec)
		flowIDs = append(flowIDs, fid)
	}

	// Scheduler allows max 2 concurrent flows.
	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{MaxConcurrentFlows: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	for _, fid := range flowIDs {
		if err := sched.Submit(ctx, fid); err != nil {
			t.Fatalf("Submit flow %d: %v", fid, err)
		}
	}

	// Wait for all 4 flows to finish.
	waitFor(t, func() bool {
		for _, fid := range flowIDs {
			f, _ := store.GetFlow(ctx, fid)
			if f.Status != core.FlowDone && f.Status != core.FlowFailed {
				return false
			}
		}
		return true
	}, 5*time.Second)

	// Max concurrent should not exceed 2.
	if maxSeen.Load() > 2 {
		t.Errorf("max concurrent flows = %d, want <= 2", maxSeen.Load())
	}
}

func TestFlowScheduler_CancelQueued(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	blocker := make(chan struct{})
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		<-blocker // block forever until test closes
		return nil
	}
	eng := New(store, bus, executor)

	// Create 3 flows with 1 step each.
	flow1 := createTestFlow(t, store, "flow-1")
	createTestStep(t, store, flow1, "step", core.StepExec)
	flow2 := createTestFlow(t, store, "flow-2")
	createTestStep(t, store, flow2, "step", core.StepExec)
	flow3 := createTestFlow(t, store, "flow-3")
	createTestStep(t, store, flow3, "step", core.StepExec)

	// Max 1 concurrent — flow2 and flow3 will be queued.
	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{MaxConcurrentFlows: 1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(blocker)

	go sched.Start(ctx)

	sched.Submit(ctx, flow1)
	sched.Submit(ctx, flow2)
	sched.Submit(ctx, flow3)

	// Wait for flow1 to be dispatched, flow2+flow3 queued.
	waitFor(t, func() bool { return sched.RunningCount() == 1 }, 2*time.Second)

	if sched.QueueLen() != 2 {
		t.Fatalf("queue len = %d, want 2", sched.QueueLen())
	}

	// Cancel flow2 (which is in queue).
	if err := sched.Cancel(ctx, flow2); err != nil {
		t.Fatalf("Cancel queued flow: %v", err)
	}

	if sched.QueueLen() != 1 {
		t.Errorf("queue len after cancel = %d, want 1", sched.QueueLen())
	}

	f2, _ := store.GetFlow(ctx, flow2)
	if f2.Status != core.FlowCancelled {
		t.Errorf("flow2 status = %s, want cancelled", f2.Status)
	}
}

func TestFlowScheduler_CancelRunning(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	var cancelledByCtx atomic.Bool
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		<-ctx.Done()
		cancelledByCtx.Store(true)
		return ctx.Err()
	}
	eng := New(store, bus, executor)

	flowID := createTestFlow(t, store, "flow")
	createTestStep(t, store, flowID, "step", core.StepExec)

	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{MaxConcurrentFlows: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)
	sched.Submit(ctx, flowID)

	// Wait until running.
	waitFor(t, func() bool { return sched.RunningCount() == 1 }, 2*time.Second)

	// Cancel the running flow.
	if err := sched.Cancel(ctx, flowID); err != nil {
		t.Fatalf("Cancel running flow: %v", err)
	}

	// Wait for the executor to detect cancellation.
	waitFor(t, func() bool { return cancelledByCtx.Load() }, 2*time.Second)
}

func TestFlowScheduler_Stats(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	blocker := make(chan struct{})
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		<-blocker
		return nil
	}
	eng := New(store, bus, executor)

	flow1 := createTestFlow(t, store, "flow-1")
	createTestStep(t, store, flow1, "step", core.StepExec)
	flow2 := createTestFlow(t, store, "flow-2")
	createTestStep(t, store, flow2, "step", core.StepExec)

	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{MaxConcurrentFlows: 1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(blocker)

	go sched.Start(ctx)

	sched.Submit(ctx, flow1)
	sched.Submit(ctx, flow2)

	waitFor(t, func() bool { return sched.RunningCount() == 1 }, 2*time.Second)

	stats := sched.Stats()
	if stats.MaxConcurrent != 1 {
		t.Errorf("MaxConcurrent = %d, want 1", stats.MaxConcurrent)
	}
	if stats.RunningCount != 1 {
		t.Errorf("RunningCount = %d, want 1", stats.RunningCount)
	}
	if stats.QueuedCount != 1 {
		t.Errorf("QueuedCount = %d, want 1", stats.QueuedCount)
	}
}

func TestFlowScheduler_SubmitRejectNonPending(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	eng := New(store, bus, func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		return nil
	})

	flowID := createTestFlow(t, store, "flow")
	// Mark flow as done.
	store.UpdateFlowStatus(context.Background(), flowID, core.FlowRunning)
	store.UpdateFlowStatus(context.Background(), flowID, core.FlowDone)

	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{})
	err := sched.Submit(context.Background(), flowID)
	if err == nil {
		t.Fatal("expected error submitting non-pending flow")
	}
}

func TestFlowScheduler_QueuedTransition(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 32})
	defer sub.Cancel()

	eng := New(store, bus, func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		return nil
	})

	flowID := createTestFlow(t, store, "flow")
	createTestStep(t, store, flowID, "step", core.StepExec)

	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{MaxConcurrentFlows: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	sched.Submit(ctx, flowID)

	// Check that flow.queued event was emitted.
	var foundQueued bool
	timeout := time.After(2 * time.Second)
	for !foundQueued {
		select {
		case ev := <-sub.C:
			if ev.Type == core.EventFlowQueued && ev.FlowID == flowID {
				foundQueued = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for flow.queued event")
		}
	}
}

// --- helpers ---

func createTestFlow(t *testing.T, store *sqlite.Store, name string) int64 {
	t.Helper()
	id, err := store.CreateFlow(context.Background(), &core.Flow{
		Name:   name,
		Status: core.FlowPending,
	})
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}
	return id
}

func createTestStep(t *testing.T, store *sqlite.Store, flowID int64, name string, stepType core.StepType) int64 {
	t.Helper()
	id, err := store.CreateStep(context.Background(), &core.Step{
		FlowID: flowID,
		Name:   name,
		Type:   stepType,
		Status: core.StepPending,
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	return id
}

func createTestStepWithDeps(t *testing.T, store *sqlite.Store, flowID int64, name string, stepType core.StepType, deps []int64) int64 {
	t.Helper()
	id, err := store.CreateStep(context.Background(), &core.Step{
		FlowID:    flowID,
		Name:      name,
		Type:      stepType,
		Status:    core.StepPending,
		DependsOn: deps,
	})
	if err != nil {
		t.Fatalf("create step with deps: %v", err)
	}
	return id
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for condition")
}
