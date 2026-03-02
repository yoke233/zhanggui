package engine

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/user/ai-workflow/internal/core"
)

func TestNewPipelineID(t *testing.T) {
	id := NewPipelineID()
	if len(id) != 8+1+12 {
		t.Errorf("unexpected ID length: %s (len=%d)", id, len(id))
	}
}

func TestTemplatesDefined(t *testing.T) {
	for _, name := range []string{"full", "standard", "quick", "hotfix"} {
		stages, ok := Templates[name]
		if !ok {
			t.Errorf("template %s not defined", name)
		}
		if len(stages) == 0 {
			t.Errorf("template %s has no stages", name)
		}
	}

	for _, name := range []string{"quick", "hotfix"} {
		stages := Templates[name]
		hasWT := false
		hasCL := false
		for _, s := range stages {
			if s == "worktree_setup" {
				hasWT = true
			}
			if s == "cleanup" {
				hasCL = true
			}
		}
		if !hasWT {
			t.Errorf("%s missing worktree_setup", name)
		}
		if !hasCL {
			t.Errorf("%s missing cleanup", name)
		}
	}
}

func TestFullTemplateOrder(t *testing.T) {
	got := Templates["full"]
	want := []core.StageID{
		core.StageWorktreeSetup,
		core.StageRequirements,
		core.StageImplement,
		core.StageCodeReview,
		core.StageFixup,
		core.StageE2ETest,
		core.StageMerge,
		core.StageCleanup,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("full template stages mismatch, got=%v want=%v", got, want)
	}
}

func TestTemplate_NoLegacySpecStages(t *testing.T) {
	legacySpecGenerate := core.StageID("spec" + "_gen")
	legacySpecReview := core.StageID("spec" + "_review")
	for name, stages := range Templates {
		for _, stage := range stages {
			if stage == legacySpecGenerate || stage == legacySpecReview {
				t.Fatalf("template %s still contains legacy spec stage: %s", name, stage)
			}
		}
	}
}

func TestWorktreeSetupBeforeRequirements(t *testing.T) {
	stages := Templates["full"]
	worktreeIdx := -1
	requirementsIdx := -1
	for i, stage := range stages {
		if stage == core.StageWorktreeSetup {
			worktreeIdx = i
		}
		if stage == core.StageRequirements {
			requirementsIdx = i
		}
	}

	if worktreeIdx < 0 || requirementsIdx < 0 {
		t.Fatalf("full template must contain worktree_setup and requirements, got=%v", stages)
	}
	if worktreeIdx > requirementsIdx {
		t.Fatalf("worktree_setup must come before requirements, got=%v", stages)
	}
}

func TestDefaultStageConfig_DefaultAgentAndE2E(t *testing.T) {
	t.Run("requirements and code_review use codex", func(t *testing.T) {
		for _, stageID := range []core.StageID{
			core.StageRequirements,
			core.StageCodeReview,
		} {
			cfg := defaultStageConfig(stageID)
			if cfg.Agent != "codex" {
				t.Fatalf("stage %s should default to codex, got %q", stageID, cfg.Agent)
			}
		}
	})

	t.Run("implement fixup and e2e use codex", func(t *testing.T) {
		for _, stageID := range []core.StageID{
			core.StageImplement,
			core.StageFixup,
			core.StageE2ETest,
		} {
			cfg := defaultStageConfig(stageID)
			if cfg.Agent != "codex" {
				t.Fatalf("stage %s should default to codex, got %q", stageID, cfg.Agent)
			}
		}
	})

	t.Run("e2e timeout is 15m", func(t *testing.T) {
		cfg := defaultStageConfig(core.StageE2ETest)
		if cfg.Timeout != 15*time.Minute {
			t.Fatalf("e2e_test timeout mismatch, got %s want %s", cfg.Timeout, 15*time.Minute)
		}
	})
}

func TestCreatePipeline_FillsStageRolesFromBindings(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	project := &core.Project{
		ID:       "proj-role-bindings",
		Name:     "proj-role-bindings",
		RepoPath: t.TempDir(),
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{}, nil)
	execEngine.SetPipelineStageRoles(map[string]string{
		"requirements": "worker",
		"implement":    "worker",
		"code_review":  "reviewer",
	})

	p, err := execEngine.CreatePipeline(project.ID, "pipe-role", "desc", "quick")
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}

	roleByStage := make(map[core.StageID]string, len(p.Stages))
	for _, stage := range p.Stages {
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

func TestPromptVars_NoLegacyPipelineSpecFields(t *testing.T) {
	content, err := os.ReadFile("prompts.go")
	if err != nil {
		t.Fatalf("read prompts.go: %v", err)
	}

	src := string(content)
	for _, legacy := range []string{
		"ChangeName",
		"SpecPath",
		"TasksMD",
	} {
		if strings.Contains(src, legacy) {
			t.Fatalf("legacy prompt field %q should be removed from PromptVars", legacy)
		}
	}
}

func TestPromptTemplateImplement_NoLegacyTasksMD(t *testing.T) {
	content, err := os.ReadFile("prompt_templates/implement.tmpl")
	if err != nil {
		t.Fatalf("read implement template: %v", err)
	}
	if strings.Contains(string(content), ".TasksMD") {
		t.Fatal("implement template should not reference TasksMD")
	}
}

func TestRenderPrompt_ImplementWorksWithoutLegacyFields(t *testing.T) {
	got, err := RenderPrompt("implement", PromptVars{
		ProjectName:  "demo",
		WorktreePath: "C:/tmp/worktrees/demo",
		Requirements: "实现用户登录接口",
	})
	if err != nil {
		t.Fatalf("RenderPrompt(implement): %v", err)
	}
	if !strings.Contains(got, "实现用户登录接口") {
		t.Fatalf("rendered prompt should contain requirements, got: %s", got)
	}
}
