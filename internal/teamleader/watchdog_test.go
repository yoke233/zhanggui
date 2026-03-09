package teamleader

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestWatchdogOnce_StuckRunFailsIssue(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-watchdog-stuck-run")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-watchdog-stuck-run", core.FailSkip, []core.Issue{
		newIssueWithProfile("issue-watchdog-stuck-run", "stuck run", core.WorkflowProfileStrict, nil),
	})

	blockingRunner := func(_ context.Context, _ string) error {
		select {}
	}

	s := NewDepScheduler(store, nil, blockingRunner, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issue := waitIssueStatus(t, store, "issue-watchdog-stuck-run", core.IssueStatusExecuting, 3*time.Second)
	run, err := store.GetRun(issue.RunID)
	if err != nil {
		t.Fatalf("GetRun(%s) error = %v", issue.RunID, err)
	}
	run.Status = core.StatusInProgress
	if err := store.SaveRun(run); err != nil {
		t.Fatalf("SaveRun(%s) error = %v", run.ID, err)
	}
	time.Sleep(25 * time.Millisecond)

	s.watchdogOnce(context.Background(), config.WatchdogConfig{
		Enabled:       true,
		Interval:      config.Duration{Duration: time.Minute},
		StuckRunTTL:   config.Duration{Duration: 5 * time.Millisecond},
		StuckMergeTTL: config.Duration{Duration: time.Hour},
		QueueStaleTTL: config.Duration{Duration: time.Hour},
	})

	failed := waitIssueStatus(t, store, "issue-watchdog-stuck-run", core.IssueStatusFailed, 3*time.Second)
	steps, err := store.ListTaskSteps(failed.ID)
	if err != nil {
		t.Fatalf("ListTaskSteps(%s) error = %v", failed.ID, err)
	}
	if len(steps) == 0 {
		t.Fatal("expected watchdog failure task step to be recorded")
	}
	last := steps[len(steps)-1]
	if last.Action != core.StepFailed {
		t.Fatalf("expected last task step action %q, got %q", core.StepFailed, last.Action)
	}
	if last.AgentID != "system" {
		t.Fatalf("expected watchdog to reuse scheduler failure path agent_id system, got %q", last.AgentID)
	}
}

func TestWatchdogOnce_SemLeakReleasesExcessSlots(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	s := NewDepScheduler(store, nil, nil, nil, 3)
	s.sem <- struct{}{}
	s.sem <- struct{}{}

	if got := len(s.sem); got != 2 {
		t.Fatalf("expected 2 leaked slots before watchdog, got %d", got)
	}

	s.watchdogOnce(context.Background(), config.WatchdogConfig{
		Enabled:       true,
		Interval:      config.Duration{Duration: time.Minute},
		StuckRunTTL:   config.Duration{Duration: time.Hour},
		StuckMergeTTL: config.Duration{Duration: time.Hour},
		QueueStaleTTL: config.Duration{Duration: time.Hour},
	})

	if got := len(s.sem); got != 0 {
		t.Fatalf("expected watchdog to release leaked slots, got %d", got)
	}
}

func TestWatchdogOnce_SkipsCompletedRunEvenIfPreviouslyStale(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-watchdog-skip-completed")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-watchdog-skip-completed", core.FailSkip, []core.Issue{
		newIssueWithProfile("issue-watchdog-skip-completed", "skip completed", core.WorkflowProfileStrict, nil),
	})

	blockingRunner := func(_ context.Context, _ string) error {
		select {}
	}

	s := NewDepScheduler(store, nil, blockingRunner, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issue := waitIssueStatus(t, store, "issue-watchdog-skip-completed", core.IssueStatusExecuting, 3*time.Second)
	run, err := store.GetRun(issue.RunID)
	if err != nil {
		t.Fatalf("GetRun(%s) error = %v", issue.RunID, err)
	}
	run.Status = core.StatusCompleted
	run.Conclusion = core.ConclusionSuccess
	if err := store.SaveRun(run); err != nil {
		t.Fatalf("SaveRun(%s) error = %v", run.ID, err)
	}
	time.Sleep(25 * time.Millisecond)

	s.watchdogOnce(context.Background(), config.WatchdogConfig{
		Enabled:       true,
		Interval:      config.Duration{Duration: time.Minute},
		StuckRunTTL:   config.Duration{Duration: 5 * time.Millisecond},
		StuckMergeTTL: config.Duration{Duration: time.Hour},
		QueueStaleTTL: config.Duration{Duration: time.Hour},
	})

	stillExecuting := waitIssueStatus(t, store, "issue-watchdog-skip-completed", core.IssueStatusExecuting, 500*time.Millisecond)
	if stillExecuting.RunID != issue.RunID {
		t.Fatalf("expected issue to keep original run id %q, got %q", issue.RunID, stillExecuting.RunID)
	}
}

func TestDepScheduler_StopStopsWatchdog(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	s := NewDepScheduler(store, nil, nil, nil, 1)
	s.StartWatchdog(context.Background(), config.WatchdogConfig{
		Enabled:       true,
		Interval:      config.Duration{Duration: 10 * time.Millisecond},
		StuckRunTTL:   config.Duration{Duration: time.Hour},
		StuckMergeTTL: config.Duration{Duration: time.Hour},
		QueueStaleTTL: config.Duration{Duration: time.Hour},
	})

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watchdogCancel != nil {
		t.Fatal("expected Stop() to clear watchdog cancel function")
	}
}

func TestScheduler_StaleRunFailedDoesNotOverrideMerging(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-watchdog-stale-run-failed")
	autoMerge := newIssueWithProfile("issue-watchdog-stale-run-failed", "stale run failed", core.WorkflowProfileStrict, nil)
	autoMerge.AutoMerge = true

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	if err := s.ScheduleIssues(context.Background(), mustCreateIssueSessionWithItems(t, store, project.ID, "session-watchdog-stale-run-failed", core.FailSkip, []core.Issue{autoMerge})); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issue := waitIssueStatus(t, store, "issue-watchdog-stale-run-failed", core.IssueStatusExecuting, 3*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunDone,
		RunID:     issue.RunID,
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(run_done) error = %v", err)
	}
	merging := waitIssueStatus(t, store, "issue-watchdog-stale-run-failed", core.IssueStatusMerging, 3*time.Second)

	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunFailed,
		RunID:     issue.RunID,
		Error:     "late failure",
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(stale run_failed) error = %v", err)
	}

	after := waitIssueStatus(t, store, "issue-watchdog-stale-run-failed", core.IssueStatusMerging, 500*time.Millisecond)
	if after.RunID != merging.RunID {
		t.Fatalf("expected merging issue to keep run id %q, got %q", merging.RunID, after.RunID)
	}
}

func TestDepScheduler_StopHonorsContextWhenWatchdogBusy(t *testing.T) {
	baseStore := newSchedulerTestStore(t)
	defer baseStore.Close()

	store := &blockingGetRunStore{
		Store:   baseStore,
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}

	project := mustCreateSchedulerProject(t, store, "proj-watchdog-stop-timeout")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-watchdog-stop-timeout", core.FailSkip, []core.Issue{
		newIssueWithProfile("issue-watchdog-stop-timeout", "stop timeout", core.WorkflowProfileStrict, nil),
	})

	blockingRunner := func(_ context.Context, _ string) error {
		select {}
	}

	s := NewDepScheduler(store, nil, blockingRunner, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issue := waitIssueStatus(t, store, "issue-watchdog-stop-timeout", core.IssueStatusExecuting, 3*time.Second)
	run, err := baseStore.GetRun(issue.RunID)
	if err != nil {
		t.Fatalf("GetRun(%s) error = %v", issue.RunID, err)
	}
	run.Status = core.StatusInProgress
	if err := baseStore.SaveRun(run); err != nil {
		t.Fatalf("SaveRun(%s) error = %v", run.ID, err)
	}

	s.StartWatchdog(context.Background(), config.WatchdogConfig{
		Enabled:       true,
		Interval:      config.Duration{Duration: 5 * time.Millisecond},
		StuckRunTTL:   config.Duration{Duration: time.Hour},
		StuckMergeTTL: config.Duration{Duration: time.Hour},
		QueueStaleTTL: config.Duration{Duration: time.Hour},
	})

	select {
	case <-store.started:
	case <-time.After(3 * time.Second):
		t.Fatal("expected watchdog GetRun to start")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = s.Stop(stopCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop() error = %v, want deadline exceeded", err)
	}

	close(store.release)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.watchdogWG.Wait()
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("expected watchdog goroutine to exit after release")
	}
}

type blockingGetRunStore struct {
	core.Store
	started chan struct{}
	release chan struct{}
}

func (s *blockingGetRunStore) GetRun(id string) (*core.Run, error) {
	select {
	case s.started <- struct{}{}:
	default:
	}
	<-s.release
	return s.Store.GetRun(id)
}
