package teamleader

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
)

// eventPublisher is the minimal publish-only interface used by handlers.
// core.EventBus satisfies this interface.
type eventPublisher interface {
	Publish(ctx context.Context, evt core.Event) error
}

// DepScheduler schedules issues by profile queue and maps each issue to one Run.
type DepScheduler struct {
	store   core.Store
	bus     core.EventBus
	pub     eventPublisher
	tracker core.Tracker

	runRun     func(context.Context, string) error
	sem        chan struct{}
	stageRoles map[core.StageID]string

	mu            sync.Mutex
	sessions      map[string]*runningSession
	RunIndex      map[string]RunRef
	lastSessionID string

	loopCancel     context.CancelFunc
	watchdogCancel context.CancelFunc
	loopWG         sync.WaitGroup
	reconcileWG    sync.WaitGroup
	watchdogWG     sync.WaitGroup

	reconcileInterval time.Duration
	reconcileRun      func(context.Context) error
	watchdogCfg       config.WatchdogConfig
}

// SetStageRoles configures the role mapping for run stages.
func (s *DepScheduler) SetStageRoles(roles map[string]string) {
	if s == nil || len(roles) == 0 {
		return
	}
	m := make(map[core.StageID]string, len(roles))
	for k, v := range roles {
		stage := core.StageID(strings.TrimSpace(k))
		role := strings.TrimSpace(v)
		if stage != "" && role != "" {
			m[stage] = role
		}
	}
	s.stageRoles = m
}

func NewDepScheduler(
	store core.Store,
	bus core.EventBus,
	runRun func(context.Context, string) error,
	tracker core.Tracker,
	maxConcurrent int,
) *DepScheduler {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if runRun == nil {
		runRun = func(context.Context, string) error { return nil }
	}

	return &DepScheduler{
		store:    store,
		bus:      bus,
		pub:      bus,
		tracker:  tracker,
		runRun:   runRun,
		sem:      make(chan struct{}, maxConcurrent),
		sessions: make(map[string]*runningSession),
		RunIndex: make(map[string]RunRef),
	}
}

// SetReconcileRunner configures periodic reconcile hook for status drift repair.
func (s *DepScheduler) SetReconcileRunner(interval time.Duration, run func(context.Context) error) {
	if s == nil {
		return
	}
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileInterval = interval
	s.reconcileRun = run
}

// SetWatchdogConfig configures watchdog health checks for scheduler lifecycle.
func (s *DepScheduler) SetWatchdogConfig(cfg config.WatchdogConfig) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchdogCfg = cfg
}

func (s *DepScheduler) Start(ctx context.Context) error {
	if s == nil || s.bus == nil {
		return nil
	}

	s.mu.Lock()
	if s.loopCancel != nil {
		s.mu.Unlock()
		return nil
	}
	sub, err := s.bus.Subscribe(core.WithName("dep-scheduler"))
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("dep-scheduler subscribe: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.loopCancel = cancel
	reconcileRun := s.reconcileRun
	reconcileInterval := s.reconcileInterval
	watchdogCfg := s.watchdogCfg
	if reconcileInterval <= 0 {
		reconcileInterval = 10 * time.Minute
	}
	s.loopWG.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.loopWG.Done()
		defer sub.Unsubscribe()
		for {
			select {
			case <-runCtx.Done():
				return
			case evt, ok := <-sub.C:
				if !ok {
					return
				}
				_ = s.OnEvent(context.Background(), evt)
			}
		}
	}()

	if reconcileRun != nil {
		s.reconcileWG.Add(1)
		go func(runCtx context.Context, interval time.Duration, runFn func(context.Context) error) {
			defer s.reconcileWG.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					_ = runFn(context.Background())
				}
			}
		}(runCtx, reconcileInterval, reconcileRun)
	}

	if watchdogCfg.Enabled {
		s.StartWatchdog(runCtx, watchdogCfg)
	}

	return nil
}

func (s *DepScheduler) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}

	s.stopWatchdog()

	s.mu.Lock()
	cancel := s.loopCancel
	s.loopCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.loopWG.Wait()
		s.reconcileWG.Wait()
		s.watchdogWG.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ScheduleIssues puts issues into profile-aware ready queues and dispatches runnable items.
func (s *DepScheduler) ScheduleIssues(ctx context.Context, issues []*core.Issue) error {
	if s == nil || s.store == nil {
		return errors.New("scheduler store is not configured")
	}
	if len(issues) == 0 {
		return nil
	}

	grouped, err := groupIssuesBySession(issues)
	if err != nil {
		return err
	}

	for _, sessionID := range sortedSessionIDs(grouped) {
		if err := s.scheduleSession(ctx, sessionID, grouped[sessionID]); err != nil {
			return err
		}
	}
	return nil
}

func (s *DepScheduler) scheduleSession(ctx context.Context, sessionID string, issues []*core.Issue) error {
	if len(issues) == 0 {
		return nil
	}

	projectID := strings.TrimSpace(issues[0].ProjectID)
	rs := newRunningSession(sessionID, projectID, issues)

	for _, issueID := range sortedIssueIDs(rs.IssueByID) {
		issue := rs.IssueByID[issueID]
		if issue == nil {
			continue
		}
		if issue.FailPolicy == "" {
			issue.FailPolicy = core.FailBlock
		}
		if isIssueTerminal(issue.Status) {
			continue
		}

		recoveredToQueued := false
		switch issue.Status {
		case core.IssueStatusExecuting, core.IssueStatusMerging:
			if strings.TrimSpace(issue.RunID) == "" {
				if err := transitionIssueStatus(issue, core.IssueStatusQueued); err != nil {
					return err
				}
				recoveredToQueued = true
			} else {
				rs.Running[issueID] = issue.RunID
			}
		case core.IssueStatusReady, core.IssueStatusQueued:
		default:
			if err := transitionIssueStatus(issue, core.IssueStatusQueued); err != nil {
				return err
			}
			issue.RunID = ""
			recoveredToQueued = true
		}

		if err := s.saveIssue(issue); err != nil {
			return err
		}
		if recoveredToQueued {
			s.recordTaskStep(issue, core.StepQueued, "system", "recovered on restart")
		}
		if issue.Status == core.IssueStatusQueued {
			s.publishIssueEvent(core.EventIssueQueued, issue, nil, "")
		}
	}
	if err := s.markReadyByProfileQueueLocked(rs); err != nil {
		return err
	}

	if err := s.registerSessionRuntime(sessionID, rs); err != nil {
		return err
	}
	return s.dispatchReadyAcrossSessions(ctx)
}

// RecoverExecutingIssues is the crash-recovery entrypoint in issue semantics.
func (s *DepScheduler) RecoverExecutingIssues(ctx context.Context, projectID string) error {
	if s == nil || s.store == nil {
		return errors.New("scheduler store is not configured")
	}

	active, err := s.store.GetActiveIssues(strings.TrimSpace(projectID))
	if err != nil {
		return err
	}
	if len(active) == 0 {
		return nil
	}

	ptrs := make([]*core.Issue, 0, len(active))
	for i := range active {
		ptrs = append(ptrs, &active[i])
	}

	grouped, err := groupIssuesBySession(ptrs)
	if err != nil {
		return err
	}

	for _, sessionID := range sortedSessionIDs(grouped) {
		if err := s.recoverSession(ctx, sessionID, grouped[sessionID]); err != nil {
			return err
		}
	}
	return nil
}

func (s *DepScheduler) recoverSession(ctx context.Context, sessionID string, issues []*core.Issue) error {
	if len(issues) == 0 {
		return nil
	}

	projectID := strings.TrimSpace(issues[0].ProjectID)
	rs := newRunningSession(sessionID, projectID, issues)
	rs.Recovered = true
	replayEvents := make([]core.Event, 0)

	for _, issueID := range sortedIssueIDs(rs.IssueByID) {
		issue := rs.IssueByID[issueID]
		if issue == nil {
			continue
		}
		if issue.FailPolicy == "" {
			issue.FailPolicy = core.FailBlock
		}

		switch issue.Status {
		case core.IssueStatusDone:
		case core.IssueStatusExecuting, core.IssueStatusMerging:
			if strings.TrimSpace(issue.RunID) == "" {
				if err := transitionIssueStatus(issue, core.IssueStatusQueued); err != nil {
					return err
				}
				if err := s.saveIssue(issue); err != nil {
					return err
				}
				s.recordTaskStep(issue, core.StepQueued, "system", "recovered on restart")
				continue
			}
			Run, getErr := s.store.GetRun(issue.RunID)
			if getErr != nil {
				return fmt.Errorf("recover issue %s Run %s: %w", issueID, issue.RunID, getErr)
			}
			rs.Running[issueID] = issue.RunID
			if evtType, terminal := RunRecoveryEvent(Run.Status, Run.Conclusion); terminal {
				replayEvents = append(replayEvents, core.Event{
					Type:      evtType,
					RunID:     issue.RunID,
					Error:     Run.ErrorMessage,
					Timestamp: time.Now(),
				})
			}
		case core.IssueStatusReady, core.IssueStatusQueued:
		default:
			if isIssueTerminal(issue.Status) {
				continue
			}
			if err := transitionIssueStatus(issue, core.IssueStatusQueued); err != nil {
				return err
			}
			issue.RunID = ""
			if err := s.saveIssue(issue); err != nil {
				return err
			}
			s.recordTaskStep(issue, core.StepQueued, "system", "recovered on restart")
			s.publishIssueEvent(core.EventIssueQueued, issue, nil, "")
		}
	}

	if err := s.markReadyByProfileQueueLocked(rs); err != nil {
		return err
	}
	if err := s.registerSessionRuntime(sessionID, rs); err != nil {
		return err
	}

	for i := range replayEvents {
		if err := s.OnEvent(ctx, replayEvents[i]); err != nil {
			return err
		}
	}
	if rs.HaltNew {
		return nil
	}
	return s.dispatchReadyAcrossSessions(ctx)
}

func (s *DepScheduler) registerSessionRuntime(sessionID string, rs *runningSession) error {
	if rs == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, exists := s.sessions[sessionID]; exists && existing != nil {
		for issueID, issue := range rs.IssueByID {
			if issue == nil {
				continue
			}
			existing.IssueByID[issueID] = issue
		}
		for issueID, RunID := range rs.Running {
			if strings.TrimSpace(RunID) == "" {
				continue
			}
			existing.Running[issueID] = RunID
		}
		rs = existing
	} else {
		s.sessions[sessionID] = rs
	}

	for issueID, RunID := range rs.Running {
		if strings.TrimSpace(RunID) == "" {
			continue
		}
		if _, exists := s.RunIndex[RunID]; exists {
			continue
		}
		s.RunIndex[RunID] = RunRef{sessionID: sessionID, issueID: issueID}
		if !s.tryAcquireSlot() {
			delete(s.RunIndex, RunID)
			return fmt.Errorf("recover session %s exceeds max concurrency %d", sessionID, cap(s.sem))
		}
	}
	return nil
}

// OnEvent handles run and merge lifecycle events and advances Issue state.
func (s *DepScheduler) OnEvent(ctx context.Context, evt core.Event) error {
	if s == nil {
		return nil
	}
	if !isSchedulerHandledEvent(evt.Type) {
		return nil
	}
	if strings.TrimSpace(evt.RunID) == "" && strings.TrimSpace(evt.IssueID) == "" {
		return nil
	}

	s.mu.Lock()
	err := s.handleRunEventLocked(evt)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.dispatchReadyAcrossSessions(ctx)
}
