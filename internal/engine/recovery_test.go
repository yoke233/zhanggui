package engine

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/user/ai-workflow/internal/core"
)

func TestRecovery_RestoreWaitingHuman(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	p := setupProjectAndPipeline(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureAbort},
	})
	p.Status = core.StatusWaitingHuman
	p.CurrentStage = core.StageImplement
	if err := store.SavePipeline(p); err != nil {
		t.Fatal(err)
	}

	runtime := &fakeRuntime{}
	agent := &fakeAgent{name: "codex"}
	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)

	if err := execEngine.RecoverActivePipelines(context.Background()); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	got, err := store.GetPipeline(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusWaitingHuman {
		t.Fatalf("expected waiting_human to remain unchanged, got %s", got.Status)
	}
	if runtime.calls != 0 {
		t.Fatalf("expected no runtime execution for waiting_human, calls=%d", runtime.calls)
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

	p := setupProjectAndPipeline(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
	})
	p.Status = core.StatusRunning
	p.CurrentStage = core.StageImplement
	p.WorktreePath = workDir
	if err := store.SavePipeline(p); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveCheckpoint(&core.Checkpoint{
		PipelineID: p.ID,
		StageName:  core.StageImplement,
		Status:     core.CheckpointInProgress,
		StartedAt:  time.Now().Add(-1 * time.Minute),
		AgentUsed:  "codex",
		RetryCount: 1,
	}); err != nil {
		t.Fatal(err)
	}

	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{name: "codex"}
	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)

	if err := execEngine.RecoverActivePipelines(context.Background()); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	got, err := store.GetPipeline(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusDone {
		t.Fatalf("expected recovered pipeline to finish done, got %s", got.Status)
	}
	if runtime.calls != 1 {
		t.Fatalf("expected one rerun attempt after recovery, calls=%d", runtime.calls)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("expected recovery to clean worktree path, stat err=%v", err)
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
	p := setupProjectAndPipeline(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
		{Name: core.StageFixup, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
	})
	p.Status = core.StatusRunning
	p.CurrentStage = core.StageImplement
	p.WorktreePath = workDir
	if err := store.SavePipeline(p); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveCheckpoint(&core.Checkpoint{
		PipelineID: p.ID,
		StageName:  core.StageImplement,
		Status:     core.CheckpointSuccess,
		StartedAt:  time.Now().Add(-2 * time.Minute),
		FinishedAt: time.Now().Add(-1 * time.Minute),
		AgentUsed:  "codex",
	}); err != nil {
		t.Fatal(err)
	}

	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{name: "codex"}
	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)

	if err := execEngine.RecoverActivePipelines(context.Background()); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	got, err := store.GetPipeline(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusDone {
		t.Fatalf("expected done after resume-from-next recovery, got %s", got.Status)
	}
	if runtime.calls != 1 {
		t.Fatalf("expected only next stage to run after success checkpoint, calls=%d", runtime.calls)
	}
}
