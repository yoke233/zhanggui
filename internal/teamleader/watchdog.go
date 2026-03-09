package teamleader

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
)

func watchdogDefaults(cfg config.WatchdogConfig) config.WatchdogConfig {
	if cfg.Interval.Duration <= 0 {
		cfg.Interval.Duration = 5 * time.Minute
	}
	if cfg.StuckRunTTL.Duration <= 0 {
		cfg.StuckRunTTL.Duration = 30 * time.Minute
	}
	if cfg.StuckMergeTTL.Duration <= 0 {
		cfg.StuckMergeTTL.Duration = 15 * time.Minute
	}
	if cfg.QueueStaleTTL.Duration <= 0 {
		cfg.QueueStaleTTL.Duration = 60 * time.Minute
	}
	return cfg
}

func (s *DepScheduler) StartWatchdog(ctx context.Context, cfg config.WatchdogConfig) {
	if s == nil || !cfg.Enabled {
		return
	}

	cfg = watchdogDefaults(cfg)
	runCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	if s.watchdogCancel != nil {
		s.mu.Unlock()
		cancel()
		return
	}
	s.watchdogCancel = cancel
	s.watchdogWG.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.watchdogWG.Done()
		defer func() {
			s.mu.Lock()
			s.watchdogCancel = nil
			s.mu.Unlock()
		}()
		ticker := time.NewTicker(cfg.Interval.Duration)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				s.watchdogOnce(runCtx, cfg)
			}
		}
	}()

	slog.Info("watchdog started",
		"interval", cfg.Interval.Duration,
		"stuck_run_ttl", cfg.StuckRunTTL.Duration,
		"stuck_merge_ttl", cfg.StuckMergeTTL.Duration,
		"queue_stale_ttl", cfg.QueueStaleTTL.Duration,
	)
}

func (s *DepScheduler) stopWatchdog() {
	if s == nil {
		return
	}

	s.mu.Lock()
	cancel := s.watchdogCancel
	s.watchdogCancel = nil
	s.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
}

func (s *DepScheduler) watchdogOnce(ctx context.Context, cfg config.WatchdogConfig) {
	if s == nil {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	cfg = watchdogDefaults(cfg)
	s.checkStuckRuns(ctx, cfg.StuckRunTTL.Duration)
	s.checkStuckMerging(ctx, cfg.StuckMergeTTL.Duration)
	s.checkQueueStale(cfg.QueueStaleTTL.Duration)
	s.checkSemLeak()
}

func (s *DepScheduler) checkStuckRuns(ctx context.Context, ttl time.Duration) {
	if s == nil || s.store == nil || ttl <= 0 {
		return
	}

	now := time.Now()
	type stuckRunCandidate struct {
		sessionID string
		issueID   string
		runID     string
	}
	var candidates []stuckRunCandidate

	s.mu.Lock()
	for sessionID, rs := range s.sessions {
		if rs == nil {
			continue
		}
		for issueID, runID := range rs.Running {
			issue := rs.IssueByID[issueID]
			if issue == nil || issue.Status != core.IssueStatusExecuting {
				continue
			}
			candidates = append(candidates, stuckRunCandidate{
				sessionID: sessionID,
				issueID:   issueID,
				runID:     runID,
			})
		}
	}
	s.mu.Unlock()

	for _, candidate := range candidates {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		run, err := s.store.GetRun(candidate.runID)
		if err != nil {
			slog.Warn("watchdog: failed to load run", "run_id", candidate.runID, "error", err)
			continue
		}
		if run == nil {
			continue
		}
		if evtType, terminal := RunRecoveryEvent(run.Status, run.Conclusion); terminal {
			slog.Info("watchdog: reconciling terminal run", "run_id", candidate.runID, "status", run.Status, "conclusion", run.Conclusion)
			_ = s.OnEvent(ctx, core.Event{
				Type:      evtType,
				RunID:     candidate.runID,
				Error:     run.ErrorMessage,
				Timestamp: now,
			})
			continue
		}
		if run.Status != core.StatusInProgress {
			continue
		}

		lastSeen := run.LastHeartbeatAt
		if lastSeen.IsZero() {
			lastSeen = run.UpdatedAt
		}
		age := now.Sub(lastSeen)
		if age < ttl {
			continue
		}

		s.mu.Lock()
		rs := s.sessions[candidate.sessionID]
		if rs == nil {
			s.mu.Unlock()
			continue
		}
		issue := rs.IssueByID[candidate.issueID]
		currentRunID, running := rs.Running[candidate.issueID]
		stillExecuting := issue != nil && issue.Status == core.IssueStatusExecuting
		s.mu.Unlock()
		if !running || currentRunID != candidate.runID || !stillExecuting {
			continue
		}

		slog.Warn("watchdog: stuck run detected", "run_id", candidate.runID, "age", age)
		s.cancelRun(candidate.runID)
		if err := markRunTimedOut(s.store, run, now, fmt.Sprintf("watchdog: run stuck for %v", age)); err != nil {
			slog.Warn("watchdog: failed to persist timed-out run", "run_id", candidate.runID, "error", err)
		}
		_ = s.OnEvent(ctx, core.Event{
			Type:      core.EventRunFailed,
			RunID:     candidate.runID,
			Error:     fmt.Sprintf("watchdog: run stuck for %v", age),
			Timestamp: now,
		})
	}
}

func (s *DepScheduler) checkStuckMerging(ctx context.Context, ttl time.Duration) {
	if s == nil || ttl <= 0 {
		return
	}

	now := time.Now()
	type stuckMergeCandidate struct {
		sessionID string
		issueID   string
		runID     string
	}
	var candidates []stuckMergeCandidate

	s.mu.Lock()
	for sessionID, rs := range s.sessions {
		if rs == nil {
			continue
		}
		for issueID, runID := range rs.Running {
			issue := rs.IssueByID[issueID]
			if issue == nil || issue.Status != core.IssueStatusMerging {
				continue
			}
			candidates = append(candidates, stuckMergeCandidate{
				sessionID: sessionID,
				issueID:   issueID,
				runID:     runID,
			})
		}
	}
	s.mu.Unlock()

	for _, candidate := range candidates {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		rs := s.sessions[candidate.sessionID]
		if rs == nil {
			s.mu.Unlock()
			continue
		}
		issue := rs.IssueByID[candidate.issueID]
		currentRunID, running := rs.Running[candidate.issueID]
		if !running || currentRunID != candidate.runID || issue == nil || issue.Status != core.IssueStatusMerging {
			s.mu.Unlock()
			continue
		}
		age := now.Sub(issue.UpdatedAt)
		s.mu.Unlock()
		if age < ttl {
			continue
		}

		slog.Warn("watchdog: stuck merging detected", "issue_id", candidate.issueID, "age", age)
		_ = s.OnEvent(ctx, core.Event{
			Type:      core.EventMergeFailed,
			RunID:     candidate.runID,
			IssueID:   candidate.issueID,
			Error:     fmt.Sprintf("watchdog: merging stuck for %v", age),
			Timestamp: now,
		})
	}
}

func (s *DepScheduler) checkQueueStale(ttl time.Duration) {
	if s == nil || ttl <= 0 {
		return
	}

	now := time.Now()
	type staleQueueCandidate struct {
		issueID string
		status  core.IssueStatus
		age     time.Duration
	}
	candidates := make([]staleQueueCandidate, 0)

	s.mu.Lock()
	semAvailable := len(s.sem) < cap(s.sem)

	for _, rs := range s.sessions {
		if rs == nil || rs.HaltNew {
			continue
		}
		for issueID, issue := range rs.IssueByID {
			if issue == nil {
				continue
			}
			shouldLog := false
			switch issue.Status {
			case core.IssueStatusReady:
				shouldLog = semAvailable
			case core.IssueStatusQueued:
				ready, err := s.dependenciesSatisfiedLocked(rs, issue)
				shouldLog = err == nil && ready && semAvailable
			default:
				continue
			}
			if !shouldLog {
				continue
			}
			age := now.Sub(issue.UpdatedAt)
			if age < ttl {
				continue
			}
			candidates = append(candidates, staleQueueCandidate{issueID: issueID, status: issue.Status, age: age})
		}
	}
	s.mu.Unlock()

	for _, candidate := range candidates {
		slog.Warn("watchdog: stale queue item",
			"issue_id", candidate.issueID,
			"status", candidate.status,
			"age", candidate.age,
		)
	}
}

func (s *DepScheduler) checkSemLeak() {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	semUsed := len(s.sem)
	actualRunning := 0
	for _, rs := range s.sessions {
		if rs == nil {
			continue
		}
		actualRunning += len(rs.Running)
	}

	if semUsed <= actualRunning {
		return
	}

	leaked := semUsed - actualRunning
	slog.Warn("watchdog: semaphore leak detected",
		"sem_used", semUsed,
		"actual_running", actualRunning,
		"leaked", leaked,
	)
	for i := 0; i < leaked; i++ {
		s.releaseSlot()
	}
}

func markRunTimedOut(store core.Store, run *core.Run, finishedAt time.Time, message string) error {
	if store == nil || run == nil {
		return nil
	}
	if run.Status == core.StatusCompleted {
		return nil
	}
	if err := run.TransitionStatus(core.StatusCompleted); err != nil {
		return err
	}
	run.Conclusion = core.ConclusionTimedOut
	run.ErrorMessage = message
	run.FinishedAt = finishedAt
	run.LastHeartbeatAt = finishedAt
	return store.SaveRun(run)
}
