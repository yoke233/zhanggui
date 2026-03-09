package teamleader

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
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

	runner := newControlledBlockingRunner()
	t.Cleanup(runner.Release)

	s := NewDepScheduler(store, nil, runner.Run, nil, 1)
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
	updatedRun, err := store.GetRun(issue.RunID)
	if err != nil {
		t.Fatalf("GetRun(%s) after watchdog error = %v", issue.RunID, err)
	}
	if updatedRun.Status != core.StatusCompleted {
		t.Fatalf("run status after watchdog = %q, want %q", updatedRun.Status, core.StatusCompleted)
	}
	if updatedRun.Conclusion != core.ConclusionTimedOut {
		t.Fatalf("run conclusion after watchdog = %q, want %q", updatedRun.Conclusion, core.ConclusionTimedOut)
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

func TestWatchdogOnce_QueueStaleOnlyLogs(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-watchdog-queue-stale")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-watchdog-queue-stale", core.FailSkip, []core.Issue{
		newIssueWithProfile("issue-watchdog-queue-stale", "queue stale", core.WorkflowProfileStrict, nil),
	})

	s := NewDepScheduler(store, nil, nil, nil, 0)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issue := waitIssueStatus(t, store, "issue-watchdog-queue-stale", core.IssueStatusExecuting, 3*time.Second)
	issue.Status = core.IssueStatusQueued
	issue.RunID = ""
	if err := store.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue(%s) error = %v", issue.ID, err)
	}

	s.mu.Lock()
	sessionID := makeSessionID(issue.ProjectID, issue.SessionID)
	rs := s.sessions[sessionID]
	if rs == nil {
		s.mu.Unlock()
		t.Fatalf("expected running session %q", sessionID)
	}
	delete(rs.Running, issue.ID)
	if current := rs.IssueByID[issue.ID]; current != nil {
		current.Status = core.IssueStatusQueued
		current.RunID = ""
		current.UpdatedAt = time.Now().Add(-2 * time.Hour)
	}
	s.mu.Unlock()
	s.releaseSlot()

	var logBuf bytes.Buffer
	prevLogger := slog.Default()
	testLogger := slog.New(slog.NewTextHandler(&logBuf, nil))
	slog.SetDefault(testLogger)
	defer slog.SetDefault(prevLogger)

	s.watchdogOnce(context.Background(), config.WatchdogConfig{
		Enabled:       true,
		Interval:      config.Duration{Duration: time.Minute},
		StuckRunTTL:   config.Duration{Duration: time.Hour},
		StuckMergeTTL: config.Duration{Duration: time.Hour},
		QueueStaleTTL: config.Duration{Duration: 5 * time.Millisecond},
	})

	after := waitIssueStatus(t, store, "issue-watchdog-queue-stale", core.IssueStatusQueued, 500*time.Millisecond)
	if after.RunID != "" {
		t.Fatalf("expected queue stale issue to keep empty run id, got %q", after.RunID)
	}
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "watchdog: stale queue item") {
		t.Fatalf("expected queue stale warning log, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "issue-watchdog-queue-stale") {
		t.Fatalf("expected queue stale log to include issue id, got %q", logOutput)
	}
}

func TestWatchdogOnce_ReconcilesCompletedRunToIssueDone(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-watchdog-skip-completed")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-watchdog-skip-completed", core.FailSkip, []core.Issue{
		newIssueWithProfile("issue-watchdog-skip-completed", "skip completed", core.WorkflowProfileStrict, nil),
	})

	runner := newControlledBlockingRunner()
	t.Cleanup(runner.Release)

	s := NewDepScheduler(store, nil, runner.Run, nil, 1)
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

	doneIssue := waitIssueStatus(t, store, "issue-watchdog-skip-completed", core.IssueStatusDone, 3*time.Second)
	if doneIssue.RunID != issue.RunID {
		t.Fatalf("expected completed issue to keep original run id %q, got %q", issue.RunID, doneIssue.RunID)
	}
}

func TestWatchdogOnce_SkipsActionRequiredRun(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-watchdog-action-required")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-watchdog-action-required", core.FailSkip, []core.Issue{
		newIssueWithProfile("issue-watchdog-action-required", "action required", core.WorkflowProfileStrict, nil),
	})

	runner := newControlledBlockingRunner()
	t.Cleanup(runner.Release)

	s := NewDepScheduler(store, nil, runner.Run, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issue := waitIssueStatus(t, store, "issue-watchdog-action-required", core.IssueStatusExecuting, 3*time.Second)
	run, err := store.GetRun(issue.RunID)
	if err != nil {
		t.Fatalf("GetRun(%s) error = %v", issue.RunID, err)
	}
	run.Status = core.StatusActionRequired
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

	stillExecuting := waitIssueStatus(t, store, "issue-watchdog-action-required", core.IssueStatusExecuting, 500*time.Millisecond)
	if stillExecuting.RunID != issue.RunID {
		t.Fatalf("expected action-required issue to keep run id %q, got %q", issue.RunID, stillExecuting.RunID)
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

	runner := newControlledBlockingRunner()
	t.Cleanup(runner.Release)

	s := NewDepScheduler(store, nil, runner.Run, nil, 1)
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

func TestDepScheduler_StartUsesWatchdogConfig(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	s := NewDepScheduler(store, &recordingSchedulerBus{}, nil, nil, 1)
	s.SetWatchdogConfig(config.WatchdogConfig{
		Enabled:       true,
		Interval:      config.Duration{Duration: 10 * time.Millisecond},
		StuckRunTTL:   config.Duration{Duration: time.Hour},
		StuckMergeTTL: config.Duration{Duration: time.Hour},
		QueueStaleTTL: config.Duration{Duration: time.Hour},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		running := s.watchdogCancel != nil
		s.mu.Unlock()
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected Start() to launch watchdog from config")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watchdogCancel != nil {
		t.Fatal("expected Stop() to clear watchdog cancel after Start() launched it")
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

type controlledBlockingRunner struct {
	release chan struct{}
	once    sync.Once
}

func newControlledBlockingRunner() *controlledBlockingRunner {
	return &controlledBlockingRunner{release: make(chan struct{})}
}

func (r *controlledBlockingRunner) Run(ctx context.Context, _ string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		return nil
	}
}

func (r *controlledBlockingRunner) Release() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		close(r.release)
	})
}
