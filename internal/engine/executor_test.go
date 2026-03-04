package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type singleStreamEventParser struct {
	event   core.StreamEvent
	emitted bool
}

func (p *singleStreamEventParser) Next() (*core.StreamEvent, error) {
	if p.emitted {
		return nil, io.EOF
	}
	p.emitted = true
	evt := p.event
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	return &evt, nil
}

func TestNewRunID(t *testing.T) {
	id := NewRunID()
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

func TestDefaultStageConfig_RoleOnlyDefaults(t *testing.T) {
	t.Run("agent field is empty for role-driven stages", func(t *testing.T) {
		for _, stageID := range []core.StageID{
			core.StageRequirements,
			core.StageImplement,
			core.StageCodeReview,
			core.StageFixup,
			core.StageE2ETest,
		} {
			cfg := defaultStageConfig(stageID)
			if cfg.Agent != "" {
				t.Fatalf("stage %s should not default stage.agent, got %q", stageID, cfg.Agent)
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

func TestCreateRun_FillsStageRolesFromBindings(t *testing.T) {
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
	execEngine.SetRunstageRoles(map[string]string{
		"requirements": "worker",
		"implement":    "worker",
		"code_review":  "reviewer",
	})

	p, err := execEngine.CreateRun(project.ID, "pipe-role", "desc", "quick")
	if err != nil {
		t.Fatalf("create Run: %v", err)
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

func TestPromptVars_NoLegacyRunspecFields(t *testing.T) {
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

func TestExecutorRun_AppendsLogsForStageLifecycleAndAgentOutput(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{nil}}
	agent := &fakeAgent{
		name: "codex",
		parserFn: func(io.Reader) core.StreamParser {
			return &singleStreamEventParser{
				event: core.StreamEvent{
					Type:      "stdout",
					Content:   "agent says hello",
					Timestamp: time.Now(),
				},
			}
		},
	}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:         core.StageImplement,
			Agent:        "codex",
			OnFailure:    core.OnFailureAbort,
			MaxRetries:   0,
			RequireHuman: true,
		},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatalf("save Run: %v", err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	if err := execEngine.Run(context.Background(), p.ID); err != nil {
		t.Fatalf("run should stop at human gate without error, got: %v", err)
	}

	logs, total, err := store.GetLogs(p.ID, "", 100, 0)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if total != 4 {
		t.Fatalf("expected 4 logs, got total=%d entries=%d", total, len(logs))
	}
	if len(logs) != 4 {
		t.Fatalf("expected 4 log entries, got %d", len(logs))
	}

	gotTypes := []string{logs[0].Type, logs[1].Type, logs[2].Type, logs[3].Type}
	wantTypes := []string{
		string(core.EventStageStart),
		string(core.EventAgentOutput),
		string(core.EventStageComplete),
		string(core.EventHumanRequired),
	}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("log types mismatch, got=%v want=%v", gotTypes, wantTypes)
	}

	agentOut := logs[1]
	if agentOut.Stage != string(core.StageImplement) {
		t.Fatalf("agent_output stage mismatch, got=%q want=%q", agentOut.Stage, core.StageImplement)
	}
	if agentOut.Agent != "codex" {
		t.Fatalf("agent_output agent mismatch, got=%q want=%q", agentOut.Agent, "codex")
	}
	if agentOut.Content != "agent says hello" {
		t.Fatalf("agent_output content mismatch, got=%q", agentOut.Content)
	}
}

func TestExecutorRun_AppendsLogForStageFailed(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	workDir := t.TempDir()
	runtime := &fakeRuntime{waitResults: []error{errors.New("fatal-run")}}
	agent := &fakeAgent{name: "codex"}

	p := setupProjectAndRun(t, store, workDir, []core.StageConfig{
		{
			Name:       core.StageImplement,
			Agent:      "codex",
			OnFailure:  core.OnFailureAbort,
			MaxRetries: 0,
		},
	})
	p.WorktreePath = workDir
	if err := store.SaveRun(p); err != nil {
		t.Fatalf("save Run: %v", err)
	}

	execEngine := newExecutor(store, map[string]core.AgentPlugin{"codex": agent}, runtime)
	if err := execEngine.Run(context.Background(), p.ID); err == nil {
		t.Fatal("run should fail for abort policy")
	}

	logs, total, err := store.GetLogs(p.ID, "", 100, 0)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 logs, got total=%d entries=%d", total, len(logs))
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(logs))
	}
	if logs[0].Type != string(core.EventStageStart) {
		t.Fatalf("first log should be stage_start, got=%q", logs[0].Type)
	}
	if logs[1].Type != string(core.EventStageFailed) {
		t.Fatalf("second log should be stage_failed, got=%q", logs[1].Type)
	}
	if !strings.Contains(logs[1].Content, "fatal-run") {
		t.Fatalf("stage_failed log should contain runtime error, got=%q", logs[1].Content)
	}
}
