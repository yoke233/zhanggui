package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

const defaultSchedulerPollInterval = 200 * time.Millisecond

type Scheduler struct {
	store core.Store
	run   func(context.Context, string) error

	logger        *slog.Logger
	maxGlobal     int
	maxPerProject int
	pollInterval  time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc

	loopWG sync.WaitGroup
	runWG  sync.WaitGroup
}

func NewScheduler(store core.Store, exec *Executor, logger *slog.Logger, maxGlobal, maxPerProject int) *Scheduler {
	return NewSchedulerWithRunner(store, exec.RunScheduled, logger, maxGlobal, maxPerProject, defaultSchedulerPollInterval)
}

func NewSchedulerWithRunner(
	store core.Store,
	runner func(context.Context, string) error,
	logger *slog.Logger,
	maxGlobal int,
	maxPerProject int,
	pollInterval time.Duration,
) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	if maxGlobal <= 0 {
		maxGlobal = 1
	}
	if maxPerProject <= 0 {
		maxPerProject = 1
	}
	if pollInterval <= 0 {
		pollInterval = defaultSchedulerPollInterval
	}
	return &Scheduler{
		store:         store,
		run:           runner,
		logger:        logger,
		maxGlobal:     maxGlobal,
		maxPerProject: maxPerProject,
		pollInterval:  pollInterval,
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.loopWG.Add(1)
	go func() {
		defer s.loopWG.Done()

		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if err := s.RunOnce(runCtx); err != nil {
					s.logger.Error("scheduler tick failed", "error", err)
				}
			}
		}
	}()
	return nil
}

func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.loopWG.Wait()
		s.runWG.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) Enqueue(RunID string) error {
	p, err := s.store.GetRun(RunID)
	if err != nil {
		return err
	}

	if err := p.TransitionStatus(core.StatusQueued); err != nil {
		return fmt.Errorf("enqueue Run %s: %w", p.ID, err)
	}
	p.QueuedAt = time.Now()
	return s.store.SaveRun(p)
}

func (s *Scheduler) RunOnce(ctx context.Context) error {
	active, err := s.store.GetActiveRuns()
	if err != nil {
		return err
	}

	runningCount := 0
	busyWorktrees := map[string]struct{}{}
	for _, p := range active {
		if p.Status != core.StatusInProgress {
			continue
		}
		runningCount++
		if p.WorktreePath != "" {
			busyWorktrees[p.WorktreePath] = struct{}{}
		}
	}

	slots := s.maxGlobal - runningCount
	if slots <= 0 {
		return nil
	}

	limit := s.maxGlobal * 4
	if limit < slots {
		limit = slots
	}
	runnable, err := s.store.ListRunnableRuns(limit)
	if err != nil {
		return err
	}

	for _, p := range runnable {
		if slots <= 0 {
			break
		}

		if s.maxPerProject > 0 {
			runningByProject, err := s.store.CountInProgressRunsByProject(p.ProjectID)
			if err != nil {
				return err
			}
			if runningByProject >= s.maxPerProject {
				continue
			}
		}

		if p.WorktreePath != "" {
			if _, busy := busyWorktrees[p.WorktreePath]; busy {
				continue
			}
		}

		ok, err := s.store.TryMarkRunInProgress(p.ID, core.StatusQueued)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		if p.WorktreePath != "" {
			busyWorktrees[p.WorktreePath] = struct{}{}
		}
		slots--

		s.runWG.Add(1)
		go func(RunID string) {
			defer s.runWG.Done()
			if runErr := s.run(ctx, RunID); runErr != nil {
				s.logger.Error("Run execution failed", "run_id", RunID, "error", runErr)
			}
		}(p.ID)
	}

	return nil
}

// FindRunByIssueNumber returns an existing Run bound to issue_number in Run config/artifacts.
func FindRunByIssueNumber(store core.Store, projectID string, issueNumber int) (*core.Run, error) {
	if store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if strings.TrimSpace(projectID) == "" || issueNumber <= 0 {
		return nil, nil
	}

	Runs, err := store.ListRuns(projectID, core.RunFilter{Limit: 500})
	if err != nil {
		return nil, err
	}
	for i := range Runs {
		summary := Runs[i]
		Run, err := store.GetRun(summary.ID)
		if err != nil {
			return nil, err
		}
		if issueNumberFromConfigMap(Run.Config) == issueNumber || issueNumberFromArtifacts(Run.Artifacts) == issueNumber {
			return Run, nil
		}
	}
	return nil, nil
}

func issueNumberFromConfigMap(config map[string]any) int {
	if config == nil {
		return 0
	}
	raw, ok := config["issue_number"]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return n
		}
	}
	return 0
}

func issueNumberFromArtifacts(artifacts map[string]string) int {
	if artifacts == nil {
		return 0
	}
	raw := strings.TrimSpace(artifacts["issue_number"])
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}
