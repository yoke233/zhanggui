package flow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestWorkItemScheduler_BasicExecution(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	var executed atomic.Int32
	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
		executed.Add(1)
		return nil
	}
	eng := New(store, bus, executor)

	// Create a work item with one action.
	workItemID := createTestWorkItem(t, store, "test-work-item")
	createTestAction(t, store, workItemID, "action-1", core.ActionExec, 0)

	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	if err := sched.Submit(ctx, workItemID); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Wait for execution.
	waitFor(t, func() bool { return executed.Load() >= 1 }, 2*time.Second)

	// WorkItem should be done.
	workItem, _ := store.GetWorkItem(ctx, workItemID)
	if workItem.Status != core.WorkItemDone {
		t.Errorf("work item status = %s, want done", workItem.Status)
	}
}

func TestWorkItemScheduler_ConcurrencyLimit(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	var running atomic.Int32
	var maxSeen atomic.Int32
	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
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

	// Create 4 work items, each with 1 action.
	var workItemIDs []int64
	for i := 0; i < 4; i++ {
		id := createTestWorkItem(t, store, "work-item")
		createTestAction(t, store, id, "action", core.ActionExec, 0)
		workItemIDs = append(workItemIDs, id)
	}

	// Scheduler allows max 2 concurrent work items.
	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	for _, id := range workItemIDs {
		if err := sched.Submit(ctx, id); err != nil {
			t.Fatalf("Submit work item %d: %v", id, err)
		}
	}

	// Wait for all 4 work items to finish.
	waitFor(t, func() bool {
		for _, id := range workItemIDs {
			wi, _ := store.GetWorkItem(ctx, id)
			if wi.Status != core.WorkItemDone && wi.Status != core.WorkItemFailed {
				return false
			}
		}
		return true
	}, 5*time.Second)

	// Max concurrent should not exceed 2.
	if maxSeen.Load() > 2 {
		t.Errorf("max concurrent work items = %d, want <= 2", maxSeen.Load())
	}
}

func TestWorkItemScheduler_CancelQueued(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	blocker := make(chan struct{})
	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
		<-blocker // block forever until test closes
		return nil
	}
	eng := New(store, bus, executor)

	// Create 3 work items with 1 action each.
	wi1 := createTestWorkItem(t, store, "work-item-1")
	createTestAction(t, store, wi1, "action", core.ActionExec, 0)
	wi2 := createTestWorkItem(t, store, "work-item-2")
	createTestAction(t, store, wi2, "action", core.ActionExec, 0)
	wi3 := createTestWorkItem(t, store, "work-item-3")
	createTestAction(t, store, wi3, "action", core.ActionExec, 0)

	// Max 1 concurrent — wi2 and wi3 will be queued.
	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(blocker)

	go sched.Start(ctx)

	sched.Submit(ctx, wi1)
	sched.Submit(ctx, wi2)
	sched.Submit(ctx, wi3)

	// Wait for wi1 to be dispatched, wi2+wi3 queued.
	waitFor(t, func() bool { return sched.RunningCount() == 1 }, 2*time.Second)

	if sched.QueueLen() != 2 {
		t.Fatalf("queue len = %d, want 2", sched.QueueLen())
	}

	// Cancel wi2 (which is in queue).
	if err := sched.Cancel(ctx, wi2); err != nil {
		t.Fatalf("Cancel queued work item: %v", err)
	}

	if sched.QueueLen() != 1 {
		t.Errorf("queue len after cancel = %d, want 1", sched.QueueLen())
	}

	w2, _ := store.GetWorkItem(ctx, wi2)
	if w2.Status != core.WorkItemCancelled {
		t.Errorf("wi2 status = %s, want cancelled", w2.Status)
	}
}

func TestWorkItemScheduler_CancelRunning(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	var cancelledByCtx atomic.Bool
	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
		<-ctx.Done()
		cancelledByCtx.Store(true)
		return ctx.Err()
	}
	eng := New(store, bus, executor)

	workItemID := createTestWorkItem(t, store, "work-item")
	createTestAction(t, store, workItemID, "action", core.ActionExec, 0)

	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)
	sched.Submit(ctx, workItemID)

	// Wait until running.
	waitFor(t, func() bool { return sched.RunningCount() == 1 }, 2*time.Second)

	// Cancel the running work item.
	if err := sched.Cancel(ctx, workItemID); err != nil {
		t.Fatalf("Cancel running work item: %v", err)
	}

	// Wait for the executor to detect cancellation.
	waitFor(t, func() bool { return cancelledByCtx.Load() }, 2*time.Second)
}

func TestWorkItemScheduler_Stats(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	blocker := make(chan struct{})
	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
		<-blocker
		return nil
	}
	eng := New(store, bus, executor)

	wi1 := createTestWorkItem(t, store, "work-item-1")
	createTestAction(t, store, wi1, "action", core.ActionExec, 0)
	wi2 := createTestWorkItem(t, store, "work-item-2")
	createTestAction(t, store, wi2, "action", core.ActionExec, 0)

	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(blocker)

	go sched.Start(ctx)

	sched.Submit(ctx, wi1)
	sched.Submit(ctx, wi2)

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

func TestWorkItemScheduler_SubmitRejectNonOpen(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	eng := New(store, bus, func(ctx context.Context, action *core.Action, run *core.Run) error {
		return nil
	})

	workItemID := createTestWorkItem(t, store, "work-item")
	// Mark work item as done.
	store.UpdateWorkItemStatus(context.Background(), workItemID, core.WorkItemRunning)
	store.UpdateWorkItemStatus(context.Background(), workItemID, core.WorkItemDone)

	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{})
	err := sched.Submit(context.Background(), workItemID)
	if err == nil {
		t.Fatal("expected error submitting non-open work item")
	}
}

func TestWorkItemScheduler_QueuedTransition(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 32})
	defer sub.Cancel()

	eng := New(store, bus, func(ctx context.Context, action *core.Action, run *core.Run) error {
		return nil
	})

	workItemID := createTestWorkItem(t, store, "work-item")
	createTestAction(t, store, workItemID, "action", core.ActionExec, 0)

	sched := NewWorkItemScheduler(eng, store, bus, WorkItemSchedulerConfig{MaxConcurrentWorkItems: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	sched.Submit(ctx, workItemID)

	// Check that work_item.queued event was emitted.
	var foundQueued bool
	timeout := time.After(2 * time.Second)
	for !foundQueued {
		select {
		case ev := <-sub.C:
			if ev.Type == core.EventWorkItemQueued && ev.WorkItemID == workItemID {
				foundQueued = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for work_item.queued event")
		}
	}
}

// --- helpers ---

func createTestWorkItem(t *testing.T, store *sqlite.Store, title string) int64 {
	t.Helper()
	id, err := store.CreateWorkItem(context.Background(), &core.WorkItem{
		Title:  title,
		Status: core.WorkItemOpen,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	return id
}

func createTestAction(t *testing.T, store *sqlite.Store, workItemID int64, name string, actionType core.ActionType, position int) int64 {
	t.Helper()
	id, err := store.CreateAction(context.Background(), &core.Action{
		WorkItemID: workItemID,
		Name:       name,
		Type:       actionType,
		Status:     core.ActionPending,
		Position:   position,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
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
