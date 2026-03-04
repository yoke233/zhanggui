package storesqlite

import (
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestSchedulerListRunnableRunsFIFO(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-a", Name: "A", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	base := time.Now().Add(-1 * time.Hour)
	runs := []*core.Run{
		{
			ID:        "run-1",
			ProjectID: project.ID,
			Name:      "one",
			Template:  "quick",
			Status:    core.StatusQueued,
			QueuedAt:  base.Add(1 * time.Minute),
			Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		},
		{
			ID:        "run-2",
			ProjectID: project.ID,
			Name:      "two",
			Template:  "quick",
			Status:    core.StatusQueued,
			QueuedAt:  base.Add(2 * time.Minute),
			Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		},
		{
			ID:        "run-3",
			ProjectID: project.ID,
			Name:      "three",
			Template:  "quick",
			Status:    core.StatusInProgress,
			QueuedAt:  base.Add(3 * time.Minute),
			Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		},
	}
	for _, p := range runs {
		p.CreatedAt = time.Now()
		p.UpdatedAt = time.Now()
		if err := s.SaveRun(p); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListRunnableRuns(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 runnable runs, got %d", len(got))
	}
	if got[0].ID != "run-1" || got[1].ID != "run-2" {
		t.Fatalf("expected FIFO order [run-1, run-2], got [%s, %s]", got[0].ID, got[1].ID)
	}
}

func TestSchedulerCountRunningByProject(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	projectA := &core.Project{ID: "proj-a", Name: "A", RepoPath: t.TempDir()}
	projectB := &core.Project{ID: "proj-b", Name: "B", RepoPath: t.TempDir()}
	if err := s.CreateProject(projectA); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProject(projectB); err != nil {
		t.Fatal(err)
	}

	saveRun := func(id, projectID string, status core.RunStatus) {
		t.Helper()
		p := &core.Run{
			ID:        id,
			ProjectID: projectID,
			Name:      id,
			Template:  "quick",
			Status:    status,
			Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := s.SaveRun(p); err != nil {
			t.Fatal(err)
		}
	}

	saveRun("a-running-1", projectA.ID, core.StatusInProgress)
	saveRun("a-running-2", projectA.ID, core.StatusInProgress)
	saveRun("a-created", projectA.ID, core.StatusQueued)
	saveRun("b-running-1", projectB.ID, core.StatusInProgress)

	countA, err := s.CountInProgressRunsByProject(projectA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countA != 2 {
		t.Fatalf("expected project A running count=2, got %d", countA)
	}

	countB, err := s.CountInProgressRunsByProject(projectB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countB != 1 {
		t.Fatalf("expected project B running count=1, got %d", countB)
	}
}

func TestSchedulerTryMarkRunningCAS(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-a", Name: "A", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	p := &core.Run{
		ID:        "run-1",
		ProjectID: project.ID,
		Name:      "one",
		Template:  "quick",
		Status:    core.StatusQueued,
		Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	ok, err := s.TryMarkRunInProgress(p.ID, core.StatusQueued)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected first CAS mark to succeed")
	}

	ok, err = s.TryMarkRunInProgress(p.ID, core.StatusQueued)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected second CAS mark to fail when status already running")
	}

	got, err := s.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusInProgress {
		t.Fatalf("expected running status, got %s", got.Status)
	}
	if got.RunCount != 1 {
		t.Fatalf("expected run_count=1, got %d", got.RunCount)
	}
}
