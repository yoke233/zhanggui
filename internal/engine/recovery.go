package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/user/ai-workflow/internal/core"
)

func (e *Executor) RecoverActivePipelines(ctx context.Context) error {
	pipelines, err := e.store.GetActivePipelines()
	if err != nil {
		return err
	}

	for i := range pipelines {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		p := pipelines[i]
		switch p.Status {
		case core.StatusWaitingHuman:
			// Human-gated pipelines stay as-is after restart.
			continue
		case core.StatusPaused:
			// Paused pipelines wait for explicit resume action.
			continue
		case core.StatusRunning:
			if err := e.recoverRunningPipeline(ctx, &p); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Executor) recoverRunningPipeline(ctx context.Context, p *core.Pipeline) error {
	checkpoints, err := e.store.GetCheckpoints(p.ID)
	if err != nil {
		return err
	}

	if cp := latestInProgressCheckpoint(checkpoints); cp != nil {
		failed := &core.Checkpoint{
			PipelineID: cp.PipelineID,
			StageName:  cp.StageName,
			Status:     core.CheckpointFailed,
			StartedAt:  cp.StartedAt,
			FinishedAt: time.Now(),
			AgentUsed:  cp.AgentUsed,
			RetryCount: cp.RetryCount,
			Error:      "recovered: previous in_progress checkpoint interrupted by crash",
		}
		if err := e.store.SaveCheckpoint(failed); err != nil {
			return err
		}
		if err := cleanupWorktree(p.WorktreePath); err != nil {
			return fmt.Errorf("cleanup worktree for recovery: %w", err)
		}
	}

	p.Status = core.StatusRunning
	p.UpdatedAt = time.Now()
	if err := e.store.SavePipeline(p); err != nil {
		return err
	}
	return e.RunScheduled(ctx, p.ID)
}

func latestInProgressCheckpoint(checkpoints []core.Checkpoint) *core.Checkpoint {
	for i := len(checkpoints) - 1; i >= 0; i-- {
		if checkpoints[i].Status == core.CheckpointInProgress {
			return &checkpoints[i]
		}
	}
	return nil
}

func cleanupWorktree(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	// Preferred cleanup path for git worktrees.
	reset := exec.Command("git", "-C", path, "reset", "--hard")
	if err := reset.Run(); err == nil {
		clean := exec.Command("git", "-C", path, "clean", "-fd")
		if err := clean.Run(); err != nil {
			return err
		}
		return nil
	}

	// Fallback for non-git directories: clear contents but keep worktree root.
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
