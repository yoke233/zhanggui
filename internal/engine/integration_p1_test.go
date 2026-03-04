package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestIntegrationP1_MultiProjectSchedulerLimits(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	projectA := mustCreateProject(t, store, "proj-a")
	projectB := mustCreateProject(t, store, "proj-b")
	projectC := mustCreateProject(t, store, "proj-c")

	RunIDs := []string{"a-1", "a-2", "b-1", "b-2", "c-1"}
	mustSaveCreatedRun(t, store, "a-1", projectA.ID, time.Now().Add(1*time.Second), "wt-shared")
	mustSaveCreatedRun(t, store, "a-2", projectA.ID, time.Now().Add(2*time.Second), "wt-a2")
	mustSaveCreatedRun(t, store, "b-1", projectB.ID, time.Now().Add(3*time.Second), "wt-shared")
	mustSaveCreatedRun(t, store, "b-2", projectB.ID, time.Now().Add(4*time.Second), "wt-b2")
	mustSaveCreatedRun(t, store, "c-1", projectC.ID, time.Now().Add(5*time.Second), "wt-c1")

	var (
		mu                  sync.Mutex
		currentGlobal       int
		maxGlobalObserved   int
		currentProject      = map[string]int{}
		maxProjectObserved  = map[string]int{}
		currentWorktree     = map[string]int{}
		maxWorktreeObserved = map[string]int{}
	)
	errCh := make(chan error, len(RunIDs))
	runner := func(_ context.Context, RunID string) error {
		p, err := store.GetRun(RunID)
		if err != nil {
			errCh <- err
			return err
		}

		mu.Lock()
		currentGlobal++
		if currentGlobal > maxGlobalObserved {
			maxGlobalObserved = currentGlobal
		}

		currentProject[p.ProjectID]++
		if currentProject[p.ProjectID] > maxProjectObserved[p.ProjectID] {
			maxProjectObserved[p.ProjectID] = currentProject[p.ProjectID]
		}

		if p.WorktreePath != "" {
			currentWorktree[p.WorktreePath]++
			if currentWorktree[p.WorktreePath] > maxWorktreeObserved[p.WorktreePath] {
				maxWorktreeObserved[p.WorktreePath] = currentWorktree[p.WorktreePath]
			}
		}
		mu.Unlock()
		defer func() {
			mu.Lock()
			currentGlobal--
			currentProject[p.ProjectID]--
			if p.WorktreePath != "" {
				currentWorktree[p.WorktreePath]--
			}
			mu.Unlock()
		}()

		time.Sleep(70 * time.Millisecond)
		if err := markRunDoneErr(store, RunID); err != nil {
			errCh <- err
			return err
		}
		return nil
	}

	scheduler := NewSchedulerWithRunner(store, runner, testLogger(), 2, 1, 10*time.Millisecond)
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer stopScheduler(t, scheduler)

	waitRunsDoneOrError(t, store, RunIDs, errCh, 8*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if maxGlobalObserved < 2 {
		t.Fatalf("expected observed global concurrency >= 2, got %d", maxGlobalObserved)
	}
	if maxGlobalObserved > 2 {
		t.Fatalf("expected max global concurrency <= 2, got %d", maxGlobalObserved)
	}
	for projectID, observed := range maxProjectObserved {
		if observed > 1 {
			t.Fatalf("expected max per-project concurrency <= 1 for %s, got %d", projectID, observed)
		}
	}
	if maxWorktreeObserved["wt-shared"] > 1 {
		t.Fatalf("expected shared worktree concurrency <= 1, got %d", maxWorktreeObserved["wt-shared"])
	}
}

func TestIntegrationP1_RecoveryAfterCrash(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
		{Name: core.StageFixup, Agent: "codex", OnFailure: core.OnFailureAbort, MaxRetries: 0},
	})
	p.Status = core.StatusInProgress
	p.CurrentStage = core.StageFixup
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveCheckpoint(&core.Checkpoint{
		RunID:      p.ID,
		StageName:  core.StageImplement,
		Status:     core.CheckpointSuccess,
		StartedAt:  time.Now().Add(-2 * time.Minute),
		FinishedAt: time.Now().Add(-90 * time.Second),
		AgentUsed:  "codex",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCheckpoint(&core.Checkpoint{
		RunID:     p.ID,
		StageName: core.StageFixup,
		Status:    core.CheckpointInProgress,
		StartedAt: time.Now().Add(-60 * time.Second),
		AgentUsed: "codex",
	}); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, []error{nil})

	if err := execEngine.RecoverActiveRuns(context.Background()); err != nil {
		t.Fatalf("RecoverActiveRuns failed: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusCompleted {
		t.Fatalf("expected recovered Run status done, got %s", got.Status)
	}

	checkpoints, err := store.GetCheckpoints(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundRecoveryFailure := false
	for _, cp := range checkpoints {
		if cp.Status == core.CheckpointFailed &&
			cp.StageName == core.StageFixup &&
			cp.Error == recoveryInterruptedCheckpointError {
			foundRecoveryFailure = true
			break
		}
	}
	if !foundRecoveryFailure {
		t.Fatalf("expected a recovery-generated failed checkpoint, got %+v", checkpoints)
	}
}

func markRunDoneErr(store core.Store, RunID string) error {
	p, err := store.GetRun(RunID)
	if err != nil {
		return err
	}
	p.Status = core.StatusCompleted
	p.FinishedAt = time.Now()
	p.UpdatedAt = time.Now()
	return store.SaveRun(p)
}

func waitRunsDoneOrError(t *testing.T, store core.Store, ids []string, errCh <-chan error, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		select {
		case err := <-errCh:
			t.Fatalf("runner error: %v", err)
		default:
		}

		allDone := true
		for _, id := range ids {
			p, err := store.GetRun(id)
			if err != nil {
				t.Fatalf("get Run %s: %v", id, err)
			}
			if p.Status != core.StatusCompleted {
				allDone = false
				break
			}
		}
		if allDone {
			select {
			case err := <-errCh:
				t.Fatalf("runner error: %v", err)
			default:
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting Runs to complete: %v", ids)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
