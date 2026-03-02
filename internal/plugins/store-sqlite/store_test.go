package storesqlite

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/user/ai-workflow/internal/core"
)

func TestProjectCRUD(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	p := &core.Project{ID: "test-1", Name: "Test", RepoPath: "/tmp/test"}
	if err := s.CreateProject(p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetProject("test-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Test" {
		t.Errorf("expected Test, got %s", got.Name)
	}

	got.Name = "Updated"
	if err := s.UpdateProject(got); err != nil {
		t.Fatal(err)
	}

	got2, _ := s.GetProject("test-1")
	if got2.Name != "Updated" {
		t.Errorf("expected Updated, got %s", got2.Name)
	}

	if err := s.DeleteProject("test-1"); err != nil {
		t.Fatal(err)
	}
	_, err = s.GetProject("test-1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestPipelineSaveAndGet(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.CreateProject(&core.Project{ID: "proj-1", Name: "P", RepoPath: "/tmp/p"})

	pipe := &core.Pipeline{
		ID:        "20260228-aabbccddeeff",
		ProjectID: "proj-1",
		Name:      "test-pipe",
		Template:  "standard",
		Status:    core.StatusCreated,
		Stages:    []core.StageConfig{{Name: core.StageImplement, Agent: "claude"}},
		Artifacts: map[string]string{},

		MaxTotalRetries: 5,
	}
	if err := s.SavePipeline(pipe); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetPipeline("20260228-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	if got.Template != "standard" {
		t.Errorf("expected standard, got %s", got.Template)
	}
}

func TestTaskPlanRoundTrip_PersistsContractMeta(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-contract-meta", Name: "meta", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	plan := &core.TaskPlan{
		ID:               "plan-20260301-11223344",
		ProjectID:        project.ID,
		Name:             "contract-meta",
		Status:           core.PlanDraft,
		WaitReason:       core.WaitNone,
		FailPolicy:       core.FailBlock,
		SpecProfile:      "default",
		ContractVersion:  "v1",
		ContractChecksum: "sha256:11223344",
	}
	if err := s.SaveTaskPlan(plan); err != nil {
		t.Fatalf("save task plan: %v", err)
	}

	got, err := s.GetTaskPlan(plan.ID)
	if err != nil {
		t.Fatalf("get task plan: %v", err)
	}
	if got.SpecProfile != plan.SpecProfile || got.ContractVersion != plan.ContractVersion || got.ContractChecksum != plan.ContractChecksum {
		t.Fatalf("contract meta not persisted: got %#v", got)
	}
}

func TestTaskItemRoundTrip_PersistsInputsOutputsAcceptanceConstraints(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-contract-task", Name: "task", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	plan := &core.TaskPlan{
		ID:         "plan-20260301-55667788",
		ProjectID:  project.ID,
		Name:       "contract-task",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
	}
	if err := s.SaveTaskPlan(plan); err != nil {
		t.Fatalf("save task plan: %v", err)
	}

	item := &core.TaskItem{
		ID:          "task-55667788-1",
		PlanID:      plan.ID,
		Title:       "contract item",
		Description: "task with structured contract",
		Labels:      []string{"backend"},
		DependsOn:   []string{},
		Inputs:      []string{"oauth_app_id"},
		Outputs:     []string{"oauth_token"},
		Acceptance:  []string{"oauth callback returns 200"},
		Constraints: []string{"do not change existing api path"},
		Template:    "standard",
		Status:      core.ItemPending,
	}
	if err := s.CreateTaskItem(item); err != nil {
		t.Fatalf("create task item: %v", err)
	}

	got, err := s.GetTaskItem(item.ID)
	if err != nil {
		t.Fatalf("get task item: %v", err)
	}
	if !reflect.DeepEqual(got.Inputs, item.Inputs) || !reflect.DeepEqual(got.Outputs, item.Outputs) ||
		!reflect.DeepEqual(got.Acceptance, item.Acceptance) || !reflect.DeepEqual(got.Constraints, item.Constraints) {
		t.Fatalf("structured fields not persisted: got=%#v", got)
	}
}

func TestPipelineRoundTrip_PersistsTaskItemID(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-pipeline-task-item", Name: "pipe", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	p := &core.Pipeline{
		ID:         "pipe-task-item-1",
		ProjectID:  project.ID,
		Name:       "pipeline-with-task",
		Template:   "standard",
		Status:     core.StatusCreated,
		TaskItemID: "task-55667788-1",
		Stages:     []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		Artifacts:  map[string]string{},
	}
	if err := s.SavePipeline(p); err != nil {
		t.Fatalf("save pipeline: %v", err)
	}

	got, err := s.GetPipeline(p.ID)
	if err != nil {
		t.Fatalf("get pipeline: %v", err)
	}
	if got.TaskItemID != p.TaskItemID {
		t.Fatalf("pipeline task_item_id mismatch: got=%q want=%q", got.TaskItemID, p.TaskItemID)
	}
}

func TestTaskPlanRoundTrip_PersistsSourceFilesAndFileContents(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-source-files", Name: "source-files", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	plan := &core.TaskPlan{
		ID:         "plan-20260302-99aabbcc",
		ProjectID:  project.ID,
		Name:       "source-files",
		Status:     core.PlanExecuting,
		WaitReason: core.WaitParseFailed,
		FailPolicy: core.FailBlock,
		SourceFiles: []string{
			"internal/core/taskplan.go",
			"internal/plugins/store-sqlite/store.go",
		},
		FileContents: map[string]string{
			"internal/core/taskplan.go":              "package core",
			"internal/plugins/store-sqlite/store.go": "package storesqlite",
		},
	}
	if err := s.SaveTaskPlan(plan); err != nil {
		t.Fatalf("save task plan: %v", err)
	}

	got, err := s.GetTaskPlan(plan.ID)
	if err != nil {
		t.Fatalf("get task plan: %v", err)
	}
	if !reflect.DeepEqual(got.SourceFiles, plan.SourceFiles) {
		t.Fatalf("source_files mismatch from GetTaskPlan: got=%#v want=%#v", got.SourceFiles, plan.SourceFiles)
	}
	if !reflect.DeepEqual(got.FileContents, plan.FileContents) {
		t.Fatalf("file_contents mismatch from GetTaskPlan: got=%#v want=%#v", got.FileContents, plan.FileContents)
	}

	list, err := s.ListTaskPlans(project.ID, core.TaskPlanFilter{Status: string(core.PlanExecuting)})
	if err != nil {
		t.Fatalf("list task plans: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one task plan in list, got %d", len(list))
	}
	if !reflect.DeepEqual(list[0].SourceFiles, plan.SourceFiles) {
		t.Fatalf("source_files mismatch from ListTaskPlans: got=%#v want=%#v", list[0].SourceFiles, plan.SourceFiles)
	}
	if !reflect.DeepEqual(list[0].FileContents, plan.FileContents) {
		t.Fatalf("file_contents mismatch from ListTaskPlans: got=%#v want=%#v", list[0].FileContents, plan.FileContents)
	}

	active, err := s.GetActiveTaskPlans()
	if err != nil {
		t.Fatalf("get active task plans: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected one active task plan, got %d", len(active))
	}
	if !reflect.DeepEqual(active[0].SourceFiles, plan.SourceFiles) {
		t.Fatalf("source_files mismatch from GetActiveTaskPlans: got=%#v want=%#v", active[0].SourceFiles, plan.SourceFiles)
	}
	if !reflect.DeepEqual(active[0].FileContents, plan.FileContents) {
		t.Fatalf("file_contents mismatch from GetActiveTaskPlans: got=%#v want=%#v", active[0].FileContents, plan.FileContents)
	}

	updated := *got
	updated.SourceFiles = []string{"internal/core/taskplan_test.go"}
	updated.FileContents = map[string]string{
		"internal/core/taskplan_test.go": "package core",
	}
	if err := s.ReplaceTaskPlanAndItems(&updated, nil); err != nil {
		t.Fatalf("replace task plan and items: %v", err)
	}

	got2, err := s.GetTaskPlan(plan.ID)
	if err != nil {
		t.Fatalf("get task plan after replace: %v", err)
	}
	if !reflect.DeepEqual(got2.SourceFiles, updated.SourceFiles) {
		t.Fatalf("source_files mismatch after replace: got=%#v want=%#v", got2.SourceFiles, updated.SourceFiles)
	}
	if !reflect.DeepEqual(got2.FileContents, updated.FileContents) {
		t.Fatalf("file_contents mismatch after replace: got=%#v want=%#v", got2.FileContents, updated.FileContents)
	}
}

func TestTaskPlanMigrationCompatibility_SourceFilesAndFileContents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-task-plan.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := seedLegacySchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-legacy-src', 'legacy', '/tmp/proj-legacy-src')`); err != nil {
		_ = db.Close()
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_plans (id, project_id, name, status) VALUES ('plan-legacy-src', 'proj-legacy-src', 'legacy plan', 'done')`); err != nil {
		_ = db.Close()
		t.Fatalf("insert legacy task_plan: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer s.Close()

	got, err := s.GetTaskPlan("plan-legacy-src")
	if err != nil {
		t.Fatalf("get migrated task plan: %v", err)
	}
	if len(got.SourceFiles) != 0 {
		t.Fatalf("expected migrated source_files empty, got %#v", got.SourceFiles)
	}
	if len(got.FileContents) != 0 {
		t.Fatalf("expected migrated file_contents empty, got %#v", got.FileContents)
	}

	got.SourceFiles = []string{"README.md"}
	got.FileContents = map[string]string{"README.md": "# ai-workflow"}
	if err := s.SaveTaskPlan(got); err != nil {
		t.Fatalf("save migrated task plan: %v", err)
	}

	loaded, err := s.GetTaskPlan("plan-legacy-src")
	if err != nil {
		t.Fatalf("reload migrated task plan: %v", err)
	}
	if !reflect.DeepEqual(loaded.SourceFiles, got.SourceFiles) {
		t.Fatalf("source_files mismatch after migrated save: got=%#v want=%#v", loaded.SourceFiles, got.SourceFiles)
	}
	if !reflect.DeepEqual(loaded.FileContents, got.FileContents) {
		t.Fatalf("file_contents mismatch after migrated save: got=%#v want=%#v", loaded.FileContents, got.FileContents)
	}
}
