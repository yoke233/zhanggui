package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	gitops "github.com/yoke233/ai-workflow/internal/git"
)

func (e *Executor) executeStage(ctx context.Context, project *core.Project, p *core.Run, stage *core.StageConfig) error {
	switch stage.Name {
	case core.StageSetup:
		return e.runWorktreeSetup(project, p)
	case core.StageMerge:
		return e.runMerge(project, p)
	case core.StageCleanup:
		return e.runCleanup(project, p)
	}

	if p.WorktreePath == "" {
		return fmt.Errorf("worktree path is empty for agent stage %s", stage.Name)
	}

	roleName := strings.TrimSpace(stage.Role)
	if roleName == "" {
		return fmt.Errorf("stage role is required for stage %q", stage.Name)
	}
	if e.roleResolver == nil {
		return fmt.Errorf("role resolver is not configured for stage %q", stage.Name)
	}
	agentProfile, roleProfile, err := e.roleResolver.Resolve(roleName)
	if err != nil {
		return fmt.Errorf("resolve role %q for stage %q: %w", roleName, stage.Name, err)
	}
	agentName := strings.TrimSpace(agentProfile.ID)
	if agentName == "" {
		return fmt.Errorf("resolved empty agent id for stage %q role %q", stage.Name, roleName)
	}

	promptStage := stage.PromptTemplate
	if promptStage == "" {
		promptStage = string(stage.Name)
	}
	executionContext, err := buildPromptExecutionContext(p, stage.Name)
	if err != nil {
		return fmt.Errorf("build prompt execution context: %w", err)
	}
	prompt, err := RenderPrompt(promptStage, PromptVars{
		ProjectName:       project.Name,
		RepoPath:          project.RepoPath,
		WorktreePath:      p.WorktreePath,
		Requirements:      p.Description,
		ExecutionContext:   executionContext,
		RetryError:        p.ErrorMessage,
		MergeConflictHint: mergeConflictHintFromConfig(p.Config),
		RetryCount:        p.TotalRetries,
	})
	if err != nil {
		return fmt.Errorf("render prompt: %w", err)
	}

	if e.testStageFunc != nil {
		testCtx := ctx
		if stage.IdleTimeout > 0 {
			var lastActivity atomic.Int64
			lastActivity.Store(time.Now().UnixNano())
			var testCancel context.CancelFunc
			testCtx, testCancel = startIdleChecker(ctx, &lastActivity, stage.IdleTimeout, e.logger, map[string]string{
				"run_id": p.ID,
				"stage":  string(stage.Name),
			})
			defer testCancel()
		} else if stage.Timeout > 0 {
			var testCancel context.CancelFunc
			testCtx, testCancel = context.WithTimeout(ctx, stage.Timeout)
			defer testCancel()
		}
		return e.testStageFunc(testCtx, p.ID, stage.Name, agentName, prompt)
	}

	return e.runACPStage(ctx, agentName, agentProfile, roleProfile, p, stage, prompt)
}

func (e *Executor) resolveStageAgentName(stage *core.StageConfig) (string, acpclient.ClientCapabilities, error) {
	if stage == nil {
		return "", acpclient.ClientCapabilities{}, errors.New("stage config is nil")
	}
	if !stageRequiresRole(stage.Name) {
		return "", acpclient.ClientCapabilities{}, nil
	}

	roleName := strings.TrimSpace(stage.Role)
	if roleName == "" {
		return "", acpclient.ClientCapabilities{}, fmt.Errorf("stage role is required for stage %q", stage.Name)
	}
	if e.roleResolver == nil {
		return "", acpclient.ClientCapabilities{}, fmt.Errorf("stage role resolver is not configured for stage %q (role=%q)", stage.Name, roleName)
	}

	resolvedAgent, roleProfile, err := e.roleResolver.Resolve(roleName)
	if err != nil {
		return "", acpclient.ClientCapabilities{}, fmt.Errorf("stage role not resolved for stage %q (role=%q): %w", stage.Name, roleName, err)
	}
	agentName := strings.TrimSpace(resolvedAgent.ID)
	if agentName == "" {
		return "", acpclient.ClientCapabilities{}, fmt.Errorf("stage role not resolved for stage %q (role=%q): resolved empty agent id", stage.Name, roleName)
	}
	return agentName, roleProfile.Capabilities, nil
}

func stageRequiresRole(stage core.StageID) bool {
	switch stage {
	case core.StageSetup, core.StageMerge, core.StageCleanup:
		return false
	default:
		return true
	}
}

func buildPromptExecutionContext(p *core.Run, stage core.StageID) (string, error) {
	ctx := map[string]string{
		"run_id":      p.ID,
		"run_name":    p.Name,
		"stage":       string(stage),
		"template":    p.Template,
		"branch_name": p.BranchName,
	}
	payload, err := json.Marshal(ctx)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func (e *Executor) runWorktreeSetup(project *core.Project, p *core.Run) error {
	if p.Config == nil {
		p.Config = map[string]any{}
	}
	if e.workspace == nil {
		return errors.New("workspace plugin is not configured")
	}

	result, err := e.workspace.Setup(context.Background(), core.WorkspaceSetupRequest{
		RepoPath:      project.RepoPath,
		RunID:         p.ID,
		BranchName:    p.BranchName,
		WorktreePath:  p.WorktreePath,
		DefaultBranch: project.DefaultBranch,
	})
	if err != nil {
		return err
	}
	p.BranchName = result.BranchName
	p.WorktreePath = result.WorktreePath
	if result.BaseBranch != "" {
		p.Config["base_branch"] = result.BaseBranch
	}

	return e.store.SaveRun(p)
}

func (e *Executor) runMerge(project *core.Project, p *core.Run) error {
	if p.BranchName == "" {
		return errors.New("branch name is empty")
	}
	runner := gitops.NewRunner(project.RepoPath)

	baseBranch := ""
	if p.Config != nil {
		baseBranch, _ = p.Config["base_branch"].(string)
	}
	if baseBranch == "" {
		var err error
		baseBranch, err = runner.CurrentBranch()
		if err != nil {
			return err
		}
	}

	if err := runner.Checkout(baseBranch); err != nil {
		return err
	}
	_, err := runner.Merge(p.BranchName)
	return err
}

func (e *Executor) runCleanup(project *core.Project, p *core.Run) error {
	if p.WorktreePath == "" {
		return nil
	}
	if e.workspace == nil {
		return errors.New("workspace plugin is not configured")
	}
	return e.workspace.Cleanup(context.Background(), core.WorkspaceCleanupRequest{
		RepoPath:     project.RepoPath,
		WorktreePath: p.WorktreePath,
	})
}

func defaultStageConfig(id core.StageID) core.StageConfig {
	cfg := core.StageConfig{
		Name:        id,
		Timeout:     0,
		IdleTimeout: 5 * time.Minute,
		MaxRetries:  1,
		OnFailure:   core.OnFailureHuman,
	}
	switch id {
	case core.StageRequirements, core.StageReview:
		cfg.Agent = ""
	case core.StageImplement:
		cfg.Agent = ""
	case core.StageFixup:
		cfg.Agent = ""
		cfg.ReuseSessionFrom = core.StageImplement
	case core.StageTest:
		cfg.Agent = ""
		cfg.IdleTimeout = 3 * time.Minute
	case core.StageSetup, core.StageMerge, core.StageCleanup:
		cfg.Agent = ""
		cfg.IdleTimeout = 1 * time.Minute
	}
	cfg.PromptTemplate = string(id)
	return cfg
}
