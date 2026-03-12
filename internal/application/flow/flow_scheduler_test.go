package flow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
)

func TestIssueScheduler_BasicExecution(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	var executed atomic.Int32
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		executed.Add(1)
		return nil
	}
	eng := New(store, bus, executor)

	// Create an issue with one step.
	issueID := createTestIssue(t, store, "test-issue")
	createTestStep(t, store, issueID, "step-1", core.StepExec, 0)

	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	if err := sched.Submit(ctx, issueID); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Wait for execution.
	waitFor(t, func() bool { return executed.Load() >= 1 }, 2*time.Second)

	// Issue should be done.
	issue, _ := store.GetIssue(ctx, issueID)
	if issue.Status != core.IssueDone {
		t.Errorf("issue status = %s, want done", issue.Status)
	}
}

func TestIssueScheduler_ConcurrencyLimit(t *testing.T) {
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

	// Create 4 issues, each with 1 step.
	var issueIDs []int64
	for i := 0; i < 4; i++ {
		id := createTestIssue(t, store, "issue")
		createTestStep(t, store, id, "step", core.StepExec, 0)
		issueIDs = append(issueIDs, id)
	}

	// Scheduler allows max 2 concurrent issues.
	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	for _, id := range issueIDs {
		if err := sched.Submit(ctx, id); err != nil {
			t.Fatalf("Submit issue %d: %v", id, err)
		}
	}

	// Wait for all 4 issues to finish.
	waitFor(t, func() bool {
		for _, id := range issueIDs {
			issue, _ := store.GetIssue(ctx, id)
			if issue.Status != core.IssueDone && issue.Status != core.IssueFailed {
				return false
			}
		}
		return true
	}, 5*time.Second)

	// Max concurrent should not exceed 2.
	if maxSeen.Load() > 2 {
		t.Errorf("max concurrent issues = %d, want <= 2", maxSeen.Load())
	}
}

func TestIssueScheduler_CancelQueued(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	blocker := make(chan struct{})
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		<-blocker // block forever until test closes
		return nil
	}
	eng := New(store, bus, executor)

	// Create 3 issues with 1 step each.
	issue1 := createTestIssue(t, store, "issue-1")
	createTestStep(t, store, issue1, "step", core.StepExec, 0)
	issue2 := createTestIssue(t, store, "issue-2")
	createTestStep(t, store, issue2, "step", core.StepExec, 0)
	issue3 := createTestIssue(t, store, "issue-3")
	createTestStep(t, store, issue3, "step", core.StepExec, 0)

	// Max 1 concurrent — issue2 and issue3 will be queued.
	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(blocker)

	go sched.Start(ctx)

	sched.Submit(ctx, issue1)
	sched.Submit(ctx, issue2)
	sched.Submit(ctx, issue3)

	// Wait for issue1 to be dispatched, issue2+issue3 queued.
	waitFor(t, func() bool { return sched.RunningCount() == 1 }, 2*time.Second)

	if sched.QueueLen() != 2 {
		t.Fatalf("queue len = %d, want 2", sched.QueueLen())
	}

	// Cancel issue2 (which is in queue).
	if err := sched.Cancel(ctx, issue2); err != nil {
		t.Fatalf("Cancel queued issue: %v", err)
	}

	if sched.QueueLen() != 1 {
		t.Errorf("queue len after cancel = %d, want 1", sched.QueueLen())
	}

	i2, _ := store.GetIssue(ctx, issue2)
	if i2.Status != core.IssueCancelled {
		t.Errorf("issue2 status = %s, want cancelled", i2.Status)
	}
}

func TestIssueScheduler_CancelRunning(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	var cancelledByCtx atomic.Bool
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		<-ctx.Done()
		cancelledByCtx.Store(true)
		return ctx.Err()
	}
	eng := New(store, bus, executor)

	issueID := createTestIssue(t, store, "issue")
	createTestStep(t, store, issueID, "step", core.StepExec, 0)

	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)
	sched.Submit(ctx, issueID)

	// Wait until running.
	waitFor(t, func() bool { return sched.RunningCount() == 1 }, 2*time.Second)

	// Cancel the running issue.
	if err := sched.Cancel(ctx, issueID); err != nil {
		t.Fatalf("Cancel running issue: %v", err)
	}

	// Wait for the executor to detect cancellation.
	waitFor(t, func() bool { return cancelledByCtx.Load() }, 2*time.Second)
}

func TestIssueScheduler_Stats(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	blocker := make(chan struct{})
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		<-blocker
		return nil
	}
	eng := New(store, bus, executor)

	issue1 := createTestIssue(t, store, "issue-1")
	createTestStep(t, store, issue1, "step", core.StepExec, 0)
	issue2 := createTestIssue(t, store, "issue-2")
	createTestStep(t, store, issue2, "step", core.StepExec, 0)

	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(blocker)

	go sched.Start(ctx)

	sched.Submit(ctx, issue1)
	sched.Submit(ctx, issue2)

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

func TestIssueScheduler_SubmitRejectNonOpen(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	eng := New(store, bus, func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		return nil
	})

	issueID := createTestIssue(t, store, "issue")
	// Mark issue as done.
	store.UpdateIssueStatus(context.Background(), issueID, core.IssueRunning)
	store.UpdateIssueStatus(context.Background(), issueID, core.IssueDone)

	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{})
	err := sched.Submit(context.Background(), issueID)
	if err == nil {
		t.Fatal("expected error submitting non-open issue")
	}
}

func TestIssueScheduler_QueuedTransition(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()

	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 32})
	defer sub.Cancel()

	eng := New(store, bus, func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		return nil
	})

	issueID := createTestIssue(t, store, "issue")
	createTestStep(t, store, issueID, "step", core.StepExec, 0)

	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	sched.Submit(ctx, issueID)

	// Check that issue.queued event was emitted.
	var foundQueued bool
	timeout := time.After(2 * time.Second)
	for !foundQueued {
		select {
		case ev := <-sub.C:
			if ev.Type == core.EventIssueQueued && ev.IssueID == issueID {
				foundQueued = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for issue.queued event")
		}
	}
}

// --- helpers ---

func createTestIssue(t *testing.T, store *sqlite.Store, title string) int64 {
	t.Helper()
	id, err := store.CreateIssue(context.Background(), &core.Issue{
		Title:  title,
		Status: core.IssueOpen,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	return id
}

func createTestStep(t *testing.T, store *sqlite.Store, issueID int64, name string, stepType core.StepType, position int) int64 {
	t.Helper()
	id, err := store.CreateStep(context.Background(), &core.Step{
		IssueID:  issueID,
		Name:     name,
		Type:     stepType,
		Status:   core.StepPending,
		Position: position,
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
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
