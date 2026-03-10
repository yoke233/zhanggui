package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

func TestRecoverInterruptedFlows_RunningFlow(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// Simulate a flow that was running when the process crashed.
	flowID := createTestFlow(t, store, "interrupted-flow")
	step1ID := createTestStep(t, store, flowID, "step-1", core.StepExec)
	step2ID := createTestStepWithDeps(t, store, flowID, "step-2", core.StepExec, []int64{step1ID})

	// step-1 was done, step-2 was running.
	store.UpdateFlowStatus(ctx, flowID, core.FlowRunning)
	store.UpdateStepStatus(ctx, step1ID, core.StepReady)
	store.UpdateStepStatus(ctx, step1ID, core.StepRunning)
	store.UpdateStepStatus(ctx, step1ID, core.StepDone)
	store.UpdateStepStatus(ctx, step2ID, core.StepReady)
	store.UpdateStepStatus(ctx, step2ID, core.StepRunning)

	// Create a stale execution for step-2.
	execID, _ := store.CreateExecution(ctx, &core.Execution{
		StepID: step2ID,
		FlowID: flowID,
		Status: core.ExecRunning,
	})

	// Set up scheduler.
	var executed atomic.Int32
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		executed.Add(1)
		return nil
	}
	eng := New(store, bus, executor)
	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{MaxConcurrentFlows: 2})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	// Recover.
	n, err := RecoverInterruptedFlows(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedFlows: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}

	// Wait for execution to finish.
	waitFor(t, func() bool {
		f, _ := store.GetFlow(ctx, flowID)
		return f.Status == core.FlowDone
	}, 3*time.Second)

	// step-2 should have been re-executed (step-1 stays done).
	if executed.Load() < 1 {
		t.Error("expected at least 1 step execution after recovery")
	}

	// The stale execution should be marked failed.
	exec, _ := store.GetExecution(ctx, execID)
	if exec.Status != core.ExecFailed {
		t.Errorf("stale exec status = %s, want failed", exec.Status)
	}
	if exec.ErrorKind != core.ErrKindTransient {
		t.Errorf("stale exec error_kind = %s, want transient", exec.ErrorKind)
	}
}

func TestRecoverInterruptedFlows_QueuedFlow(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// Simulate a flow that was queued when the process crashed.
	flowID := createTestFlow(t, store, "queued-flow")
	createTestStep(t, store, flowID, "step-1", core.StepExec)
	store.UpdateFlowStatus(ctx, flowID, core.FlowQueued)

	var executed atomic.Int32
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		executed.Add(1)
		return nil
	}
	eng := New(store, bus, executor)
	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{MaxConcurrentFlows: 2})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverInterruptedFlows(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedFlows: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}

	waitFor(t, func() bool {
		f, _ := store.GetFlow(ctx, flowID)
		return f.Status == core.FlowDone
	}, 3*time.Second)
}

func TestRecoverInterruptedFlows_NoInterrupted(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// Create only done/pending flows — nothing to recover.
	f1 := createTestFlow(t, store, "done-flow")
	store.UpdateFlowStatus(ctx, f1, core.FlowRunning)
	store.UpdateFlowStatus(ctx, f1, core.FlowDone)

	createTestFlow(t, store, "pending-flow") // stays pending

	eng := New(store, bus, func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		return nil
	})
	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverInterruptedFlows(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedFlows: %v", err)
	}
	if n != 0 {
		t.Errorf("recovered = %d, want 0", n)
	}
}

func TestRecoverInterruptedFlows_MultipleFlows(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// 2 running flows + 1 queued.
	f1 := createTestFlow(t, store, "running-1")
	createTestStep(t, store, f1, "step", core.StepExec)
	store.UpdateFlowStatus(ctx, f1, core.FlowRunning)

	f2 := createTestFlow(t, store, "running-2")
	createTestStep(t, store, f2, "step", core.StepExec)
	store.UpdateFlowStatus(ctx, f2, core.FlowRunning)

	f3 := createTestFlow(t, store, "queued-1")
	createTestStep(t, store, f3, "step", core.StepExec)
	store.UpdateFlowStatus(ctx, f3, core.FlowQueued)

	var executed atomic.Int32
	eng := New(store, bus, func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		executed.Add(1)
		return nil
	})
	sched := NewFlowScheduler(eng, store, bus, FlowSchedulerConfig{MaxConcurrentFlows: 3})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverInterruptedFlows(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedFlows: %v", err)
	}
	if n != 3 {
		t.Fatalf("recovered = %d, want 3", n)
	}

	waitFor(t, func() bool { return executed.Load() >= 3 }, 5*time.Second)
}
