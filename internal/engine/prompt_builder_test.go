package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

type mockMemory struct {
	cold string
	warm string
	hot  string
}

func (m *mockMemory) RecallCold(issueID string) (string, error) {
	return m.cold, nil
}

func (m *mockMemory) RecallWarm(issueID string) (string, error) {
	return m.warm, nil
}

func (m *mockMemory) RecallHot(issueID string, runID string) (string, error) {
	return m.hot, nil
}

func TestPromptBuilder_WithAllLayers(t *testing.T) {
	mem := &mockMemory{
		cold: "## 任务背景\n标题: Auth system",
		warm: "## 父任务\n标题: Platform",
		hot:  "## 最近事件\n- review_approved",
	}
	builder := NewPromptBuilder(mem)

	prompt, err := builder.Build("issue-1", "run-1", "implement", PromptVars{
		ProjectName:  "test-project",
		WorktreePath: "/tmp/wt",
		Requirements: "Build login page",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []string{"Auth system", "Platform", "review_approved", "Build login page"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestPromptBuilder_NoMemory(t *testing.T) {
	builder := NewPromptBuilder(nil)

	prompt, err := builder.Build("issue-1", "run-1", "implement", PromptVars{
		ProjectName:  "test-project",
		WorktreePath: "/tmp/wt",
		Requirements: "Build login page",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(prompt, "Build login page") {
		t.Fatalf("prompt missing requirements: %s", prompt)
	}
}

func TestPromptBuilder_PartialMemory(t *testing.T) {
	mem := &mockMemory{cold: "## 任务背景\n标题: Auth"}
	builder := NewPromptBuilder(mem)

	prompt, err := builder.Build("issue-1", "run-1", "implement", PromptVars{
		ProjectName:  "test-project",
		WorktreePath: "/tmp/wt",
		Requirements: "Build login page",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(prompt, "Auth") {
		t.Fatalf("prompt missing cold context: %s", prompt)
	}
	if !strings.Contains(prompt, "Build login page") {
		t.Fatalf("prompt missing requirements: %s", prompt)
	}
}

func TestExecutorExecuteStage_UsesPromptBuilderMemory(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	worktreePath := t.TempDir()
	run := setupProjectAndRun(t, store, worktreePath, []core.StageConfig{
		{
			Name:           core.StageImplement,
			PromptTemplate: "implement",
			OnFailure:      core.OnFailureAbort,
		},
	})
	run.WorktreePath = worktreePath
	run.IssueID = "issue-memory-1"
	if err := store.SaveRun(run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if err := store.CreateIssue(&core.Issue{
		ID:        run.IssueID,
		ProjectID: run.ProjectID,
		Title:     "Build login flow",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	project, err := store.GetProject(run.ProjectID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}

	execEngine := newExecutor(store, nil)
	execEngine.SetMemory(&mockMemory{
		cold: "## 任务背景\n标题: Build login flow",
		warm: "## 父任务\n标题: Auth epic",
		hot:  "## 最近事件\n- review_approved",
	})

	var capturedPrompt string
	execEngine.TestSetStageFunc(func(ctx context.Context, runID string, stage core.StageID, agentName, prompt string) error {
		capturedPrompt = prompt
		return nil
	})

	if err := execEngine.executeStage(context.Background(), project, run, &run.Stages[0]); err != nil {
		t.Fatalf("executeStage: %v", err)
	}

	for _, want := range []string{"Build login flow", "Auth epic", "review_approved"} {
		if !strings.Contains(capturedPrompt, want) {
			t.Fatalf("captured prompt missing %q: %s", want, capturedPrompt)
		}
	}
}

func TestExecutorExecuteStage_ReviewUsesLayeredTemplate(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	worktreePath := t.TempDir()
	run := setupProjectAndRun(t, store, worktreePath, []core.StageConfig{
		{
			Name:           core.StageReview,
			PromptTemplate: "review",
			OnFailure:      core.OnFailureAbort,
		},
	})
	run.WorktreePath = worktreePath
	run.IssueID = "issue-review-memory-1"
	run.Description = "Review login page"
	if err := store.SaveRun(run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if err := store.CreateIssue(&core.Issue{
		ID:        run.IssueID,
		ProjectID: run.ProjectID,
		Title:     "Review login flow",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	project, err := store.GetProject(run.ProjectID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}

	execEngine := newExecutor(store, nil)
	execEngine.SetMemory(&mockMemory{
		cold: "## 任务背景\n标题: Review login flow",
		warm: "## 父任务\n标题: Auth epic",
		hot:  "## 最近事件\n- review_rejected",
	})

	var capturedPrompt string
	execEngine.TestSetStageFunc(func(ctx context.Context, runID string, stage core.StageID, agentName, prompt string) error {
		capturedPrompt = prompt
		return nil
	})

	if err := execEngine.executeStage(context.Background(), project, run, &run.Stages[0]); err != nil {
		t.Fatalf("executeStage: %v", err)
	}

	for _, want := range []string{
		"Review login flow",
		"Auth epic",
		"review_rejected",
		"你正在对项目 proj 的改动进行代码审查",
	} {
		if !strings.Contains(capturedPrompt, want) {
			t.Fatalf("captured prompt missing %q: %s", want, capturedPrompt)
		}
	}
}

func TestRenderPrompt_LayeredContextTemplates(t *testing.T) {
	tests := []struct {
		stage       string
		requirement string
		requireReq  bool
		extraCheck  string
	}{
		{stage: "implement", requirement: "Build login page", requireReq: true, extraCheck: "你正在项目 demo 的 worktree"},
		{stage: "review", requirement: "Review login page", requireReq: true, extraCheck: "你正在对项目 demo 的改动进行代码审查"},
		{stage: "fixup", requirement: "Fix login page", requireReq: false, extraCheck: "你正在项目 demo 中修复上一轮审查问题"},
		{stage: "requirements", requirement: "Structure login requirements", requireReq: true, extraCheck: "你正在项目 demo (D:/repo) 中工作"},
	}

	for _, tc := range tests {
		t.Run(tc.stage, func(t *testing.T) {
			prompt, err := RenderPrompt(tc.stage, PromptVars{
				ProjectName:      "demo",
				RepoPath:         "D:/repo",
				WorktreePath:     "D:/repo/.worktrees/demo",
				Requirements:     tc.requirement,
				ExecutionContext: `{"run_id":"run-1"}`,
				PreviousReview:   "Need stronger validation",
				RetryError:       "compile failed",
				ColdContext:      "## 任务背景\n标题: Login",
				WarmContext:      "## 父任务\n标题: Auth",
				HotContext:       "## 最近事件\n- review_rejected",
			})
			if err != nil {
				t.Fatalf("RenderPrompt(%s): %v", tc.stage, err)
			}

			wants := []string{
				"## 任务背景\n标题: Login",
				"## 父任务\n标题: Auth",
				"## 最近事件\n- review_rejected",
				tc.extraCheck,
			}
			if tc.requireReq {
				wants = append(wants, tc.requirement)
			}

			for _, want := range wants {
				if !strings.Contains(prompt, want) {
					t.Fatalf("RenderPrompt(%s) missing %q: %s", tc.stage, want, prompt)
				}
			}
		})
	}
}
