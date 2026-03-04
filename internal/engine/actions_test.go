package engine

import (
	"context"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestActionApprove_ContinueNextStage(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{nil, nil}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:         core.StageImplement,
			Agent:        "codex",
			OnFailure:    core.OnFailureAbort,
			MaxRetries:   0,
			RequireHuman: true,
		},
		{
			Name:       core.StageFixup,
			Agent:      "codex",
			OnFailure:  core.OnFailureAbort,
			MaxRetries: 0,
		},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	if err := execEngine.Run(context.Background(), p.ID); err != nil {
		t.Fatalf("initial run failed: %v", err)
	}

	waiting, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Status != core.StatusWaitingReview {
		t.Fatalf("expected waiting_review after first stage, got %s", waiting.Status)
	}

	err = execEngine.ApplyAction(context.Background(), core.RunAction{
		RunID:   p.ID,
		Type:    core.ActionApprove,
		Stage:   waiting.CurrentStage,
		Message: "继续执行",
	})
	if err != nil {
		t.Fatalf("approve action failed: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusDone {
		t.Fatalf("expected done after approve, got %s", got.Status)
	}
	if runtime.calls != 2 {
		t.Fatalf("expected only next stage to run after approve, runtime calls=%d", runtime.calls)
	}
}

func TestActionReject_InvalidateFollowingCheckpoints(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	project := &core.Project{ID: "proj-1", Name: "proj", RepoPath: t.TempDir()}
	if err := store.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	p := &core.Run{
		ID:           "pipe-reject",
		ProjectID:    project.ID,
		Name:         "pipe",
		Template:     "quick",
		Status:       core.StatusWaitingReview,
		CurrentStage: core.StageFixup,
		Stages: []core.StageConfig{
			{Name: core.StageImplement, Agent: "codex", Role: "worker"},
			{Name: core.StageFixup, Agent: "codex", Role: "worker"},
			{Name: core.StageCodeReview, Agent: "claude", Role: "reviewer"},
		},
		Artifacts: map[string]string{},
		Config:    map[string]any{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	checkpoints := []*core.Checkpoint{
		{RunID: p.ID, StageName: core.StageImplement, Status: core.CheckpointSuccess, StartedAt: now, FinishedAt: now},
		{RunID: p.ID, StageName: core.StageFixup, Status: core.CheckpointSuccess, StartedAt: now, FinishedAt: now},
		{RunID: p.ID, StageName: core.StageCodeReview, Status: core.CheckpointSuccess, StartedAt: now, FinishedAt: now},
	}
	for _, cp := range checkpoints {
		if err := store.SaveCheckpoint(cp); err != nil {
			t.Fatal(err)
		}
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{}, &fakeRuntime{})
	err := execEngine.ApplyAction(context.Background(), core.RunAction{
		RunID:   p.ID,
		Type:    core.ActionReject,
		Stage:   core.StageFixup,
		Message: "fixup 输出不符合预期",
	})
	if err != nil {
		t.Fatalf("reject action failed: %v", err)
	}

	after, err := store.GetCheckpoints(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) < 3 {
		t.Fatalf("expected >=3 checkpoints, got %d", len(after))
	}
	if after[0].Status != core.CheckpointSuccess {
		t.Fatalf("expected first checkpoint to remain success, got %s", after[0].Status)
	}
	if after[1].Status != core.CheckpointInvalidated {
		t.Fatalf("expected second checkpoint invalidated, got %s", after[1].Status)
	}
	if after[2].Status != core.CheckpointInvalidated {
		t.Fatalf("expected third checkpoint invalidated, got %s", after[2].Status)
	}

	logs, total, err := store.GetLogs(p.ID, "", 50, 0)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if total == 0 || len(logs) == 0 {
		t.Fatalf("expected action_applied log entry, got total=%d len=%d", total, len(logs))
	}
	found := false
	for i := range logs {
		if logs[i].Type == string(core.EventActionApplied) {
			found = true
			if logs[i].Stage != string(core.StageFixup) {
				t.Fatalf("expected action_applied log stage=%s, got %s", core.StageFixup, logs[i].Stage)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected action_applied log, got logs=%#v", logs)
	}
}

func TestActionPauseResume_ReRunCurrentStage(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	project := &core.Project{ID: "proj-1", Name: "proj", RepoPath: t.TempDir()}
	if err := store.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	p := &core.Run{
		ID:           "pipe-pause-resume",
		ProjectID:    project.ID,
		Name:         "pipe",
		Template:     "quick",
		Status:       core.StatusRunning,
		CurrentStage: core.StageImplement,
		Stages: []core.StageConfig{
			{Name: core.StageImplement, Agent: "codex", Role: "worker", OnFailure: core.OnFailureAbort, MaxRetries: 0},
		},
		Artifacts:    map[string]string{},
		Config:       map[string]any{},
		WorktreePath: workDir,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := store.SaveRun(p); err != nil {
		t.Fatal(err)
	}

	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{name: "codex"}
	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)

	if err := execEngine.ApplyAction(context.Background(), core.RunAction{
		RunID:   p.ID,
		Type:    core.ActionPause,
		Stage:   core.StageImplement,
		Message: "暂停等待人工确认",
	}); err != nil {
		t.Fatalf("pause action failed: %v", err)
	}

	waiting_review, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if waiting_review.Status != core.StatusWaitingReview {
		t.Fatalf("expected waiting_review status, got %s", waiting_review.Status)
	}

	if err := execEngine.ApplyAction(context.Background(), core.RunAction{
		RunID:   p.ID,
		Type:    core.ActionResume,
		Stage:   core.StageImplement,
		Message: "继续",
	}); err != nil {
		t.Fatalf("resume action failed: %v", err)
	}

	got, err := store.GetRun(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusDone {
		t.Fatalf("expected done after resume rerun, got %s", got.Status)
	}
	if runtime.calls != 1 {
		t.Fatalf("expected current stage rerun once after resume, calls=%d", runtime.calls)
	}
}
