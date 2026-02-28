package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/user/ai-workflow/internal/core"
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

func (s *Scheduler) Enqueue(pipelineID string) error {
	p, err := s.store.GetPipeline(pipelineID)
	if err != nil {
		return err
	}

	switch p.Status {
	case core.StatusDone, core.StatusFailed, core.StatusAborted:
		return fmt.Errorf("pipeline %s status %s cannot be enqueued", p.ID, p.Status)
	}

	p.Status = core.StatusCreated
	p.QueuedAt = time.Now()
	p.UpdatedAt = time.Now()
	return s.store.SavePipeline(p)
}

func (s *Scheduler) RunOnce(ctx context.Context) error {
	active, err := s.store.GetActivePipelines()
	if err != nil {
		return err
	}

	runningCount := 0
	busyWorktrees := map[string]struct{}{}
	for _, p := range active {
		if p.Status != core.StatusRunning {
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
	runnable, err := s.store.ListRunnablePipelines(limit)
	if err != nil {
		return err
	}

	for _, p := range runnable {
		if slots <= 0 {
			break
		}

		if s.maxPerProject > 0 {
			runningByProject, err := s.store.CountRunningPipelinesByProject(p.ProjectID)
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

		ok, err := s.store.TryMarkPipelineRunning(p.ID, core.StatusCreated)
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
		go func(pipelineID string) {
			defer s.runWG.Done()
			if runErr := s.run(ctx, pipelineID); runErr != nil {
				s.logger.Error("pipeline execution failed", "pipeline_id", pipelineID, "error", runErr)
			}
		}(p.ID)
	}

	return nil
}
