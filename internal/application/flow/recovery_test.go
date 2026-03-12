package flow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestRecoverInterruptedIssues_RunningIssue(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// Simulate an issue that was running when the process crashed.
	issueID := createTestIssue(t, store, "interrupted-issue")
	step1ID := createTestStep(t, store, issueID, "step-1", core.StepExec, 0)
	step2ID := createTestStep(t, store, issueID, "step-2", core.StepExec, 1)

	// step-1 was done, step-2 was running.
	store.UpdateIssueStatus(ctx, issueID, core.IssueRunning)
	store.UpdateStepStatus(ctx, step1ID, core.StepReady)
	store.UpdateStepStatus(ctx, step1ID, core.StepRunning)
	store.UpdateStepStatus(ctx, step1ID, core.StepDone)
	store.UpdateStepStatus(ctx, step2ID, core.StepReady)
	store.UpdateStepStatus(ctx, step2ID, core.StepRunning)

	// Create a stale execution for step-2.
	execID, _ := store.CreateExecution(ctx, &core.Execution{
		StepID:  step2ID,
		IssueID: issueID,
		Status:  core.ExecRunning,
	})

	// Set up scheduler.
	var executed atomic.Int32
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		executed.Add(1)
		return nil
	}
	eng := New(store, bus, executor)
	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 2})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	// Recover.
	n, err := RecoverInterruptedIssues(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedIssues: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}

	// Wait for execution to finish.
	waitFor(t, func() bool {
		issue, _ := store.GetIssue(ctx, issueID)
		return issue.Status == core.IssueDone
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

func TestRecoverInterruptedIssues_QueuedIssue(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// Simulate an issue that was queued when the process crashed.
	issueID := createTestIssue(t, store, "queued-issue")
	createTestStep(t, store, issueID, "step-1", core.StepExec, 0)
	store.UpdateIssueStatus(ctx, issueID, core.IssueQueued)

	var executed atomic.Int32
	executor := func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		executed.Add(1)
		return nil
	}
	eng := New(store, bus, executor)
	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 2})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverInterruptedIssues(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedIssues: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}

	waitFor(t, func() bool {
		issue, _ := store.GetIssue(ctx, issueID)
		return issue.Status == core.IssueDone
	}, 3*time.Second)
}

func TestRecoverInterruptedIssues_NoInterrupted(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// Create only done/open issues — nothing to recover.
	i1 := createTestIssue(t, store, "done-issue")
	store.UpdateIssueStatus(ctx, i1, core.IssueRunning)
	store.UpdateIssueStatus(ctx, i1, core.IssueDone)

	createTestIssue(t, store, "open-issue") // stays open

	eng := New(store, bus, func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		return nil
	})
	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverInterruptedIssues(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedIssues: %v", err)
	}
	if n != 0 {
		t.Errorf("recovered = %d, want 0", n)
	}
}

func TestRecoverQueuedIssues_SkipsRunningIssues(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	queuedIssueID := createTestIssue(t, store, "queued-issue")
	createTestStep(t, store, queuedIssueID, "queued-step", core.StepExec, 0)
	store.UpdateIssueStatus(ctx, queuedIssueID, core.IssueQueued)

	runningIssueID := createTestIssue(t, store, "running-issue")
	runningStepID := createTestStep(t, store, runningIssueID, "running-step", core.StepExec, 0)
	store.UpdateIssueStatus(ctx, runningIssueID, core.IssueRunning)
	store.UpdateStepStatus(ctx, runningStepID, core.StepReady)
	store.UpdateStepStatus(ctx, runningStepID, core.StepRunning)

	var executed atomic.Int32
	eng := New(store, bus, func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		executed.Add(1)
		return nil
	})
	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 2})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverQueuedIssues(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverQueuedIssues: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}

	waitFor(t, func() bool {
		issue, _ := store.GetIssue(ctx, queuedIssueID)
		return issue.Status == core.IssueDone
	}, 3*time.Second)

	runningIssue, _ := store.GetIssue(ctx, runningIssueID)
	if runningIssue.Status != core.IssueRunning {
		t.Fatalf("running issue status = %s, want running", runningIssue.Status)
	}
	if executed.Load() != 1 {
		t.Fatalf("executed = %d, want 1 queued issue execution", executed.Load())
	}
}

func TestRecoverInterruptedIssues_MultipleIssues(t *testing.T) {
	store := newTestStore(t)
	bus := NewMemBus()
	ctx := context.Background()

	// 2 running issues + 1 queued.
	i1 := createTestIssue(t, store, "running-1")
	createTestStep(t, store, i1, "step", core.StepExec, 0)
	store.UpdateIssueStatus(ctx, i1, core.IssueRunning)

	i2 := createTestIssue(t, store, "running-2")
	createTestStep(t, store, i2, "step", core.StepExec, 0)
	store.UpdateIssueStatus(ctx, i2, core.IssueRunning)

	i3 := createTestIssue(t, store, "queued-1")
	createTestStep(t, store, i3, "step", core.StepExec, 0)
	store.UpdateIssueStatus(ctx, i3, core.IssueQueued)

	var executed atomic.Int32
	eng := New(store, bus, func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		executed.Add(1)
		return nil
	})
	sched := NewIssueScheduler(eng, store, bus, IssueSchedulerConfig{MaxConcurrentIssues: 3})
	schedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Start(schedCtx)

	n, err := RecoverInterruptedIssues(ctx, store, sched)
	if err != nil {
		t.Fatalf("RecoverInterruptedIssues: %v", err)
	}
	if n != 3 {
		t.Fatalf("recovered = %d, want 3", n)
	}

	waitFor(t, func() bool { return executed.Load() >= 3 }, 5*time.Second)
}
