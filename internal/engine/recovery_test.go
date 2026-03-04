package engine

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestRecovery_RestoreWaitingHuman(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureAbort},
	})
	p.Status = core.StatusActionRequired
	p.CurrentStage = core.StageImplement
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, nil)

	if err := execEngine.RecoverActiveRuns(context.Background()); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusActionRequired {
		t.Fatalf("expected waiting_review to remain unchanged, got %s", got.Status)
	}
}

func TestRecovery_ReRunInProgressCheckpoint(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	staleFile := workDir + "/stale.txt"
	if err := os.WriteFile(staleFile, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
	})
	p.Status = core.StatusInProgress
	p.CurrentStage = core.StageImplement
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveCheckpoint(&core.Checkpoint{
		RunID:      p.ID,
		StageName:  core.StageImplement,
		Status:     core.CheckpointInProgress,
		StartedAt:  time.Now().Add(-1 * time.Minute),
		AgentUsed:  "codex",
		RetryCount: 1,
	}); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, []error{nil})

	if err := execEngine.RecoverActiveRuns(context.Background()); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusCompleted {
		t.Fatalf("expected recovered Run to finish completed, got %s", got.Status)
	}
	if got.Conclusion != core.ConclusionSuccess {
		t.Fatalf("expected success conclusion after recovery, got %s", got.Conclusion)
	}
	if _, err := os.Stat(workDir); err != nil {
		t.Fatalf("expected recovery to keep worktree root, stat err=%v", err)
	}
	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Fatalf("expected recovery to clear stale file, stat err=%v", err)
	}

	checkpoints, err := store.GetCheckpoints(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundRecoveryFailure := false
	for _, cp := range checkpoints {
		if cp.Status == core.CheckpointFailed && strings.Contains(cp.Error, "recovered: previous in_progress checkpoint") {
			foundRecoveryFailure = true
			break
		}
	}
	if !foundRecoveryFailure {
		t.Fatalf("expected a failed checkpoint produced by recovery, got %+v", checkpoints)
	}
}

func TestRecovery_ResumeFromNextAfterSuccessCheckpoint(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
		{Name: core.StageFixup, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
	})
	p.Status = core.StatusInProgress
	p.CurrentStage = core.StageImplement
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveCheckpoint(&core.Checkpoint{
		RunID:      p.ID,
		StageName:  core.StageImplement,
		Status:     core.CheckpointSuccess,
		StartedAt:  time.Now().Add(-2 * time.Minute),
		FinishedAt: time.Now().Add(-1 * time.Minute),
		AgentUsed:  "codex",
	}); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, []error{nil})

	if err := execEngine.RecoverActiveRuns(context.Background()); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusCompleted {
		t.Fatalf("expected completed after resume-from-next recovery, got %s", got.Status)
	}
	if got.Conclusion != core.ConclusionSuccess {
		t.Fatalf("expected success conclusion after resume-from-next recovery, got %s", got.Conclusion)
	}
}
