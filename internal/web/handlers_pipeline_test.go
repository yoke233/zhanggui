package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/user/ai-workflow/internal/core"
)

func TestListPipelinesInvalidLimitReturns400(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-limit",
		Name:     "project-limit",
		RepoPath: filepath.Join(t.TempDir(), "repo-limit"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/proj-limit/pipelines?limit=bad")
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/pipelines: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid limit, got %d", resp.StatusCode)
	}
}

func TestCreatePipelineThenGetPipelineByProjectAndGlobal(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-pipe",
		Name:     "project-pipe",
		RepoPath: filepath.Join(t.TempDir(), "repo-pipe"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody := map[string]any{
		"name":        "pipeline-one",
		"description": "pipeline for api test",
		"template":    "quick",
	}
	rawBody, err := json.Marshal(createBody)
	if err != nil {
		t.Fatalf("marshal create pipeline body: %v", err)
	}

	createResp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-pipe/pipelines",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/pipelines: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	var created core.Pipeline
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created pipeline: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected created pipeline id")
	}
	if created.ProjectID != "proj-pipe" {
		t.Fatalf("expected project_id proj-pipe, got %s", created.ProjectID)
	}

	getByProjectResp, err := http.Get(ts.URL + "/api/v1/projects/proj-pipe/pipelines/" + created.ID)
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/pipelines/{id}: %v", err)
	}
	defer getByProjectResp.Body.Close()
	if getByProjectResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getByProjectResp.StatusCode)
	}

	getByGlobalResp, err := http.Get(ts.URL + "/api/v1/pipelines/" + created.ID)
	if err != nil {
		t.Fatalf("GET /api/v1/pipelines/{id}: %v", err)
	}
	defer getByGlobalResp.Body.Close()
	if getByGlobalResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getByGlobalResp.StatusCode)
	}
}

func TestCreatePipeline_StageRoleBindingsApplied(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-pipe-role-bindings",
		Name:     "project-pipe-role-bindings",
		RepoPath: filepath.Join(t.TempDir(), "repo-pipe-role-bindings"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{
		Store: store,
		PipelineStageRoles: map[string]string{
			"requirements": "worker",
			"implement":    "worker",
			"code_review":  "reviewer",
		},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody := map[string]any{
		"name":        "pipeline-role",
		"description": "pipeline role bindings",
		"template":    "quick",
	}
	rawBody, err := json.Marshal(createBody)
	if err != nil {
		t.Fatalf("marshal create pipeline body: %v", err)
	}

	createResp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-pipe-role-bindings/pipelines",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/pipelines: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	var created core.Pipeline
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created pipeline: %v", err)
	}

	roleByStage := make(map[core.StageID]string, len(created.Stages))
	for _, stage := range created.Stages {
		roleByStage[stage.Name] = stage.Role
	}

	if got := roleByStage[core.StageRequirements]; got != "worker" {
		t.Fatalf("expected requirements role worker, got %q", got)
	}
	if got := roleByStage[core.StageImplement]; got != "worker" {
		t.Fatalf("expected implement role worker, got %q", got)
	}
	if got := roleByStage[core.StageCodeReview]; got != "reviewer" {
		t.Fatalf("expected code_review role reviewer, got %q", got)
	}
}

func TestGetPipelineCheckpoints(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-pipe-checkpoint",
		Name:     "project-pipe-checkpoint",
		RepoPath: filepath.Join(t.TempDir(), "repo-pipe-checkpoint"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	now := time.Now()
	pipeline := &core.Pipeline{
		ID:              "pipe-checkpoint-1",
		ProjectID:       project.ID,
		Name:            "checkpoint-pipeline",
		Template:        "quick",
		Status:          core.StatusRunning,
		CurrentStage:    core.StageImplement,
		Stages:          []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := store.SavePipeline(pipeline); err != nil {
		t.Fatalf("seed pipeline: %v", err)
	}
	if err := store.SaveCheckpoint(&core.Checkpoint{
		PipelineID: pipeline.ID,
		StageName:  core.StageImplement,
		Status:     core.CheckpointSuccess,
		StartedAt:  now,
		FinishedAt: now,
		AgentUsed:  "codex",
	}); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/proj-pipe-checkpoint/pipelines/pipe-checkpoint-1/checkpoints")
	if err != nil {
		t.Fatalf("GET checkpoints: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var checkpoints []core.Checkpoint
	if err := json.NewDecoder(resp.Body).Decode(&checkpoints); err != nil {
		t.Fatalf("decode checkpoints response: %v", err)
	}
	if len(checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(checkpoints))
	}
	if checkpoints[0].StageName != core.StageImplement {
		t.Fatalf("expected stage implement, got %s", checkpoints[0].StageName)
	}
}

func TestApplyPipelineAction(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-pipe-action",
		Name:     "project-pipe-action",
		RepoPath: filepath.Join(t.TempDir(), "repo-pipe-action"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	now := time.Now()
	pipeline := &core.Pipeline{
		ID:              "pipe-action-1",
		ProjectID:       project.ID,
		Name:            "action-pipeline",
		Template:        "quick",
		Status:          core.StatusRunning,
		CurrentStage:    core.StageImplement,
		Stages:          []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := store.SavePipeline(pipeline); err != nil {
		t.Fatalf("seed pipeline: %v", err)
	}

	execCalled := false
	executor := &testPipelineExecutor{
		applyActionFn: func(_ context.Context, action core.PipelineAction) error {
			execCalled = true
			if action.Type != core.ActionAbort {
				t.Fatalf("expected action abort, got %s", action.Type)
			}
			loaded, err := store.GetPipeline(action.PipelineID)
			if err != nil {
				return err
			}
			loaded.Status = core.StatusAborted
			loaded.UpdatedAt = time.Now()
			return store.SavePipeline(loaded)
		},
	}

	srv := NewServer(Config{
		Store:        store,
		PipelineExec: executor,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-pipe-action/pipelines/pipe-action-1/action",
		"application/json",
		bytes.NewBufferString(`{"action":"abort","message":"manual stop"}`),
	)
	if err != nil {
		t.Fatalf("POST pipeline action: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !execCalled {
		t.Fatal("expected pipeline action to delegate to executor")
	}

	var out struct {
		Status       string `json:"status"`
		CurrentStage string `json:"current_stage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode action response: %v", err)
	}
	if out.Status != string(core.StatusAborted) {
		t.Fatalf("expected status aborted, got %s", out.Status)
	}
}

func TestPipelineActionChangeRole(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-pipe-change-role",
		Name:     "project-pipe-change-role",
		RepoPath: filepath.Join(t.TempDir(), "repo-pipe-change-role"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	now := time.Now()
	pipeline := &core.Pipeline{
		ID:           "pipe-change-role-1",
		ProjectID:    project.ID,
		Name:         "change-role-pipeline",
		Template:     "quick",
		Status:       core.StatusRunning,
		CurrentStage: core.StageImplement,
		Stages: []core.StageConfig{
			{Name: core.StageImplement, Agent: "codex"},
		},
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := store.SavePipeline(pipeline); err != nil {
		t.Fatalf("seed pipeline: %v", err)
	}

	execCalled := false
	executor := &testPipelineExecutor{
		applyActionFn: func(_ context.Context, action core.PipelineAction) error {
			execCalled = true
			if action.Type != core.ActionChangeRole {
				t.Fatalf("expected action change_role, got %s", action.Type)
			}
			if action.Role != "reviewer" {
				t.Fatalf("expected role reviewer, got %q", action.Role)
			}
			loaded, err := store.GetPipeline(action.PipelineID)
			if err != nil {
				return err
			}
			loaded.Stages[0].Role = action.Role
			loaded.Status = core.StatusRunning
			loaded.UpdatedAt = time.Now()
			return store.SavePipeline(loaded)
		},
	}

	srv := NewServer(Config{
		Store:        store,
		PipelineExec: executor,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-pipe-change-role/pipelines/pipe-change-role-1/action",
		"application/json",
		bytes.NewBufferString(`{"action":"change_role","role":"reviewer"}`),
	)
	if err != nil {
		t.Fatalf("POST pipeline action change_role: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !execCalled {
		t.Fatal("expected pipeline action to delegate change_role to executor")
	}

	updated, err := store.GetPipeline("pipe-change-role-1")
	if err != nil {
		t.Fatalf("reload pipeline: %v", err)
	}
	if got := updated.Stages[0].Role; got != "reviewer" {
		t.Fatalf("expected stage role reviewer, got %q", got)
	}
	if got := updated.Stages[0].Agent; got != "codex" {
		t.Fatalf("expected stage agent unchanged codex, got %q", got)
	}
}

func TestDefaultPipelineStageConfig_DefaultAgentAndE2E(t *testing.T) {
	for _, stageID := range []core.StageID{
		core.StageRequirements,
		core.StageCodeReview,
	} {
		cfg := defaultPipelineStageConfig(stageID)
		if cfg.Agent != "claude" {
			t.Fatalf("stage %s should default to claude, got %q", stageID, cfg.Agent)
		}
	}

	for _, stageID := range []core.StageID{
		core.StageImplement,
		core.StageFixup,
		core.StageE2ETest,
	} {
		cfg := defaultPipelineStageConfig(stageID)
		if cfg.Agent != "codex" {
			t.Fatalf("stage %s should default to codex, got %q", stageID, cfg.Agent)
		}
	}

	cfg := defaultPipelineStageConfig(core.StageE2ETest)
	if cfg.Timeout != 15*time.Minute {
		t.Fatalf("e2e_test timeout mismatch, got %s want %s", cfg.Timeout, 15*time.Minute)
	}
}

func TestGetPipeline_IncludesTaskItemID(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-pipe-task-item-id",
		Name:     "project-pipe-task-item-id",
		RepoPath: filepath.Join(t.TempDir(), "repo-pipe-task-item-id"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	now := time.Now()
	pipeline := &core.Pipeline{
		ID:              "pipe-task-item-id-1",
		ProjectID:       project.ID,
		Name:            "task-item-pipeline",
		Template:        "quick",
		Status:          core.StatusCreated,
		TaskItemID:      "task-a3f1b2c0-1",
		Stages:          []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := store.SavePipeline(pipeline); err != nil {
		t.Fatalf("seed pipeline: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/proj-pipe-task-item-id/pipelines/pipe-task-item-id-1")
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/pipelines/{id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got struct {
		ID         string `json:"id"`
		TaskItemID string `json:"task_item_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode pipeline response: %v", err)
	}
	if got.ID != pipeline.ID {
		t.Fatalf("expected pipeline id %s, got %s", pipeline.ID, got.ID)
	}
	if got.TaskItemID != pipeline.TaskItemID {
		t.Fatalf("expected task_item_id %s, got %s", pipeline.TaskItemID, got.TaskItemID)
	}
}

type testPipelineExecutor struct {
	applyActionFn func(ctx context.Context, action core.PipelineAction) error
}

func (e *testPipelineExecutor) ApplyAction(ctx context.Context, action core.PipelineAction) error {
	if e.applyActionFn == nil {
		return nil
	}
	return e.applyActionFn(ctx, action)
}
