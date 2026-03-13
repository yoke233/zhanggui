package flow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestRecoverInterruptedWorkItems_RunningWorkItem(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// Simulate a work item that was running when the process crashed.
	workItemID := createTestWorkItem(t, store, "interrupted-work-item")
	action1ID := createTestAction(t, store, workItemID, "action-1", core.ActionExec, 0)
	action2ID := createTestAction(t, store, workItemID, "action-2", core.ActionExec, 1)

	// action-1 was done, action-2 was running.
	store.UpdateWorkItemStatus(ctx, workItemID, core.WorkItemRunning)
	store.UpdateActionStatus(ctx, action1ID, core.ActionReady)
	store.UpdateActionStatus(ctx, action1ID, core.ActionRunning)
	store.UpdateActionStatus(ctx, action1ID, core.ActionDone)
	store.UpdateActionStatus(ctx, action2ID, core.ActionReady)
	store.UpdateActionStatus(ctx, action2ID, core.ActionRunning)

	// Create a stale run for action-2.
	runID, _ := store.CreateRun(ctx, &core.Run{
		ActionID:   action2ID,
		WorkItemID: workItemID,
		Status:     core.RunRunning,
	})

	// Set up scheduler.
	var executed atomic.Int32
	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
		executed.Add(1)
		return nil
	}
	eng := New(store, bus, executor)
	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 2})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	// Recover.
	n, err := RecoverInterruptedWorkItems(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedWorkItems: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}

	// Wait for execution to finish.
	waitFor(t, func() bool {
		wi, _ := store.GetWorkItem(ctx, workItemID)
		return wi.Status == core.WorkItemDone
	}, 3*time.Second)

	// action-2 should have been re-executed (action-1 stays done).
	if executed.Load() < 1 {
		t.Error("expected at least 1 action execution after recovery")
	}

	// The stale run should be marked failed.
	run, _ := store.GetRun(ctx, runID)
	if run.Status != core.RunFailed {
		t.Errorf("stale run status = %s, want failed", run.Status)
	}
	if run.ErrorKind != core.ErrKindTransient {
		t.Errorf("stale run error_kind = %s, want transient", run.ErrorKind)
	}
}

func TestRecoverInterruptedWorkItems_QueuedWorkItem(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// Simulate a work item that was queued when the process crashed.
	workItemID := createTestWorkItem(t, store, "queued-work-item")
	createTestAction(t, store, workItemID, "action-1", core.ActionExec, 0)
	store.UpdateWorkItemStatus(ctx, workItemID, core.WorkItemQueued)

	var executed atomic.Int32
	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
		executed.Add(1)
		return nil
	}
	eng := New(store, bus, executor)
	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 2})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverInterruptedWorkItems(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedWorkItems: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}

	waitFor(t, func() bool {
		wi, _ := store.GetWorkItem(ctx, workItemID)
		return wi.Status == core.WorkItemDone
	}, 3*time.Second)
}

func TestRecoverInterruptedWorkItems_NoInterrupted(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// Create only done/open work items — nothing to recover.
	wi1 := createTestWorkItem(t, store, "done-work-item")
	store.UpdateWorkItemStatus(ctx, wi1, core.WorkItemRunning)
	store.UpdateWorkItemStatus(ctx, wi1, core.WorkItemDone)

	createTestWorkItem(t, store, "open-work-item") // stays open

	eng := New(store, bus, func(ctx context.Context, action *core.Action, run *core.Run) error {
		return nil
	})
	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverInterruptedWorkItems(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedWorkItems: %v", err)
	}
	if n != 0 {
		t.Errorf("recovered = %d, want 0", n)
	}
}

func TestRecoverQueuedWorkItems_SkipsRunningWorkItems(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	queuedWorkItemID := createTestWorkItem(t, store, "queued-work-item")
	createTestAction(t, store, queuedWorkItemID, "queued-action", core.ActionExec, 0)
	store.UpdateWorkItemStatus(ctx, queuedWorkItemID, core.WorkItemQueued)

	runningWorkItemID := createTestWorkItem(t, store, "running-work-item")
	runningActionID := createTestAction(t, store, runningWorkItemID, "running-action", core.ActionExec, 0)
	store.UpdateWorkItemStatus(ctx, runningWorkItemID, core.WorkItemRunning)
	store.UpdateActionStatus(ctx, runningActionID, core.ActionReady)
	store.UpdateActionStatus(ctx, runningActionID, core.ActionRunning)

	var executed atomic.Int32
	eng := New(store, bus, func(ctx context.Context, action *core.Action, run *core.Run) error {
		executed.Add(1)
		return nil
	})
	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 2})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverQueuedWorkItems(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverQueuedWorkItems: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}

	waitFor(t, func() bool {
		wi, _ := store.GetWorkItem(ctx, queuedWorkItemID)
		return wi.Status == core.WorkItemDone
	}, 3*time.Second)

	runningWI, _ := store.GetWorkItem(ctx, runningWorkItemID)
	if runningWI.Status != core.WorkItemRunning {
		t.Fatalf("running work item status = %s, want running", runningWI.Status)
	}
	if executed.Load() != 1 {
		t.Fatalf("executed = %d, want 1 queued work item execution", executed.Load())
	}
}

func TestRecoverInterruptedWorkItems_MultipleWorkItems(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// 2 running work items + 1 queued.
	wi1 := createTestWorkItem(t, store, "running-1")
	createTestAction(t, store, wi1, "action", core.ActionExec, 0)
	store.UpdateWorkItemStatus(ctx, wi1, core.WorkItemRunning)

	wi2 := createTestWorkItem(t, store, "running-2")
	createTestAction(t, store, wi2, "action", core.ActionExec, 0)
	store.UpdateWorkItemStatus(ctx, wi2, core.WorkItemRunning)

	wi3 := createTestWorkItem(t, store, "queued-1")
	createTestAction(t, store, wi3, "action", core.ActionExec, 0)
	store.UpdateWorkItemStatus(ctx, wi3, core.WorkItemQueued)

	var executed atomic.Int32
	eng := New(store, bus, func(ctx context.Context, action *core.Action, run *core.Run) error {
		executed.Add(1)
		return nil
	})
	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 3})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverInterruptedWorkItems(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedWorkItems: %v", err)
	}
	if n != 3 {
		t.Fatalf("recovered = %d, want 3", n)
	}

	waitFor(t, func() bool { return executed.Load() >= 3 }, 5*time.Second)
}
