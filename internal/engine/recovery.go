package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

const recoveryInterruptedCheckpointError = "recovered: previous in_progress checkpoint interrupted by crash"

func (e *Executor) RecoverActiveRuns(ctx context.Context) error {
	Runs, err := e.store.GetActiveRuns()
	if err != nil {
		return err
	}

	for i := range Runs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		p := Runs[i]
		switch p.Status {
		case core.StatusWaitingReview:
			// waiting_review runs wait for explicit resume action.
			continue
		case core.StatusRunning:
			if err := e.recoverRunningRun(ctx, &p); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Executor) recoverRunningRun(ctx context.Context, p *core.Run) error {
	checkpoints, err := e.store.GetCheckpoints(p.ID)
	if err != nil {
		return err
	}

	if cp := latestInProgressCheckpoint(checkpoints); cp != nil {
		failed := &core.Checkpoint{
			RunID:      cp.RunID,
			StageName:  cp.StageName,
			Status:     core.CheckpointFailed,
			StartedAt:  cp.StartedAt,
			FinishedAt: time.Now(),
			AgentUsed:  cp.AgentUsed,
			RetryCount: cp.RetryCount,
			Error:      recoveryInterruptedCheckpointError,
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
	if err := e.store.SaveRun(p); err != nil {
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
