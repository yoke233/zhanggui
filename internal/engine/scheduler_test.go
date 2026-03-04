package engine

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestScheduler_RespectGlobalLimit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	project := mustCreateProject(t, store, "proj-a")
	RunIDs := []string{"p-a-1", "p-a-2", "p-a-3"}
	for i, id := range RunIDs {
		mustSaveCreatedRun(t, store, id, project.ID, time.Now().Add(time.Duration(i)*time.Second), "")
	}

	var (
		mu          sync.Mutex
		current     int
		maxObserved int
	)
	runner := func(_ context.Context, RunID string) error {
		mu.Lock()
		current++
		if current > maxObserved {
			maxObserved = current
		}
		mu.Unlock()

		time.Sleep(80 * time.Millisecond)
		markRunDone(t, store, RunID)

		mu.Lock()
		current--
		mu.Unlock()
		return nil
	}

	scheduler := NewSchedulerWithRunner(store, runner, testLogger(), 1, 3, 10*time.Millisecond)
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer stopScheduler(t, scheduler)

	waitRunsDone(t, store, RunIDs, 5*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if maxObserved > 1 {
		t.Fatalf("expected max global concurrency <= 1, got %d", maxObserved)
	}
}

func TestScheduler_RespectPerProjectLimit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	projectA := mustCreateProject(t, store, "proj-a")
	projectB := mustCreateProject(t, store, "proj-b")

	RunIDs := []string{"a-1", "a-2", "b-1", "b-2"}
	mustSaveCreatedRun(t, store, "a-1", projectA.ID, time.Now().Add(1*time.Second), "")
	mustSaveCreatedRun(t, store, "a-2", projectA.ID, time.Now().Add(2*time.Second), "")
	mustSaveCreatedRun(t, store, "b-1", projectB.ID, time.Now().Add(3*time.Second), "")
	mustSaveCreatedRun(t, store, "b-2", projectB.ID, time.Now().Add(4*time.Second), "")

	var (
		mu                 sync.Mutex
		perProjectRunning  = map[string]int{}
		perProjectObserved = map[string]int{}
	)
	runner := func(_ context.Context, RunID string) error {
		p, err := store.GetRun(RunID)
		if err != nil {
			return err
		}
		projectID := p.ProjectID

		mu.Lock()
		perProjectRunning[projectID]++
		if perProjectRunning[projectID] > perProjectObserved[projectID] {
			perProjectObserved[projectID] = perProjectRunning[projectID]
		}
		mu.Unlock()

		time.Sleep(80 * time.Millisecond)
		markRunDone(t, store, RunID)

		mu.Lock()
		perProjectRunning[projectID]--
		mu.Unlock()
		return nil
	}

	scheduler := NewSchedulerWithRunner(store, runner, testLogger(), 4, 1, 10*time.Millisecond)
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer stopScheduler(t, scheduler)

	waitRunsDone(t, store, RunIDs, 5*time.Second)

	mu.Lock()
	defer mu.Unlock()
	for projectID, observed := range perProjectObserved {
		if observed > 1 {
			t.Fatalf("expected max concurrency for project %s <= 1, got %d", projectID, observed)
		}
	}
}

func TestScheduler_WorktreeExclusive(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	project := mustCreateProject(t, store, "proj-a")
	sharedWorktree := "C:/tmp/worktrees/shared"
	RunIDs := []string{"w-1", "w-2"}
	mustSaveCreatedRun(t, store, "w-1", project.ID, time.Now().Add(1*time.Second), sharedWorktree)
	mustSaveCreatedRun(t, store, "w-2", project.ID, time.Now().Add(2*time.Second), sharedWorktree)

	var (
		mu          sync.Mutex
		running     int
		maxObserved int
	)
	runner := func(_ context.Context, RunID string) error {
		mu.Lock()
		running++
		if running > maxObserved {
			maxObserved = running
		}
		mu.Unlock()

		time.Sleep(80 * time.Millisecond)
		markRunDone(t, store, RunID)

		mu.Lock()
		running--
		mu.Unlock()
		return nil
	}

	scheduler := NewSchedulerWithRunner(store, runner, testLogger(), 4, 4, 10*time.Millisecond)
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer stopScheduler(t, scheduler)

	waitRunsDone(t, store, RunIDs, 5*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if maxObserved > 1 {
		t.Fatalf("expected shared worktree concurrency <= 1, got %d", maxObserved)
	}
}

func TestScheduler_FIFO(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	project := mustCreateProject(t, store, "proj-a")
	RunIDs := []string{"f-1", "f-2", "f-3"}
	mustSaveCreatedRun(t, store, "f-1", project.ID, time.Now().Add(1*time.Second), "")
	mustSaveCreatedRun(t, store, "f-2", project.ID, time.Now().Add(2*time.Second), "")
	mustSaveCreatedRun(t, store, "f-3", project.ID, time.Now().Add(3*time.Second), "")

	var (
		mu    sync.Mutex
		order []string
	)
	runner := func(_ context.Context, RunID string) error {
		mu.Lock()
		order = append(order, RunID)
		mu.Unlock()

		time.Sleep(40 * time.Millisecond)
		markRunDone(t, store, RunID)
		return nil
	}

	scheduler := NewSchedulerWithRunner(store, runner, testLogger(), 1, 1, 10*time.Millisecond)
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer stopScheduler(t, scheduler)

	waitRunsDone(t, store, RunIDs, 5*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 {
		t.Fatalf("expected 3 executions, got %d (%v)", len(order), order)
	}
	if order[0] != "f-1" || order[1] != "f-2" || order[2] != "f-3" {
		t.Fatalf("expected FIFO order [f-1 f-2 f-3], got %v", order)
	}
}

func mustCreateProject(t *testing.T, store core.Store, id string) *core.Project {
	t.Helper()
	p := &core.Project{
		ID:       id,
		Name:     id,
		RepoPath: t.TempDir(),
	}
	if err := store.CreateProject(p); err != nil {
		t.Fatalf("create project %s: %v", id, err)
	}
	return p
}

func mustSaveCreatedRun(
	t *testing.T,
	store core.Store,
	id string,
	projectID string,
	queuedAt time.Time,
	worktreePath string,
) {
	t.Helper()

	p := &core.Run{
		ID:           id,
		ProjectID:    projectID,
		Name:         id,
		Template:     "quick",
		Status:       core.StatusQueued,
		QueuedAt:     queuedAt,
		WorktreePath: worktreePath,
		Stages:       []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := store.SaveRun(p); err != nil {
		t.Fatalf("save Run %s: %v", id, err)
	}
}

func markRunDone(t *testing.T, store core.Store, RunID string) {
	t.Helper()
	p, err := store.GetRun(RunID)
	if err != nil {
		t.Fatalf("get Run %s: %v", RunID, err)
	}
	p.Status = core.StatusCompleted
	p.Conclusion = core.ConclusionSuccess
	p.FinishedAt = time.Now()
	p.UpdatedAt = time.Now()
	if err := store.SaveRun(p); err != nil {
		t.Fatalf("save done Run %s: %v", RunID, err)
	}
}

func waitRunsDone(t *testing.T, store core.Store, ids []string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
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
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting Runs to complete: %v", ids)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func stopScheduler(t *testing.T, scheduler *Scheduler) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := scheduler.Stop(ctx); err != nil {
		t.Fatalf("stop scheduler: %v", err)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
