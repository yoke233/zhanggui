package provider

import (
	"context"
	"fmt"
	"path/filepath"

	workspacegit "github.com/yoke233/ai-workflow/internal/adapters/workspace/git"
	"github.com/yoke233/ai-workflow/internal/core"
)

type LocalGitProvider struct{}

func (p *LocalGitProvider) Prepare(_ context.Context, _ *core.Project, bindings []*core.ResourceBinding, issueID int64) (*core.Workspace, error) {
	var gitBindings []*core.ResourceBinding
	for _, b := range bindings {
		if b == nil || b.Kind != "git" {
			continue
		}
		gitBindings = append(gitBindings, b)
	}
	if len(gitBindings) == 0 {
		return nil, fmt.Errorf("no git resource binding found")
	}
	if len(gitBindings) > 1 {
		return nil, fmt.Errorf("multiple git resource bindings found; issue must select one binding explicitly")
	}

	b := gitBindings[0]
	repoPath := b.URI
	branchName := fmt.Sprintf("ai-flow/issue-%d", issueID)
	worktreePath := filepath.Join(repoPath, ".worktrees", fmt.Sprintf("issue-%d", issueID))

	runner := workspacegit.NewRunner(repoPath)
	if err := runner.WorktreeAdd(worktreePath, branchName); err != nil {
		return nil, fmt.Errorf("create worktree for issue %d: %w", issueID, err)
	}

	defaultBranch := DefaultBranchFromBinding(b)
	if defaultBranch == "" {
		defaultBranch = workspacegit.DetectDefaultBranch(repoPath)
	}

	metadata := map[string]any{
		"binding_id":     b.ID,
		"kind":           "git",
		"branch":         branchName,
		"default_branch": defaultBranch,
		"repo_path":      repoPath,
	}
	MergeSCMBindingMetadata(metadata, b.Config)

	return &core.Workspace{
		Path:     worktreePath,
		Metadata: metadata,
	}, nil
}

func (p *LocalGitProvider) Release(_ context.Context, ws *core.Workspace) error {
	if ws == nil || ws.Metadata == nil {
		return nil
	}
	repoPath, _ := ws.Metadata["repo_path"].(string)
	if repoPath == "" {
		return nil
	}
	runner := workspacegit.NewRunner(repoPath)
	return runner.WorktreeRemove(ws.Path)
}

func DefaultBranchFromBinding(b *core.ResourceBinding) string {
	if b == nil || b.Config == nil {
		return "main"
	}
	for _, key := range []string{"base_branch", "default_branch"} {
		if v, ok := b.Config[key].(string); ok && v != "" {
			return v
		}
	}
	return "main"
}

func MergeSCMBindingMetadata(dst map[string]any, cfg map[string]any) {
	if dst == nil || cfg == nil {
		return
	}
	for _, key := range []string{
		"provider",
		"default_branch",
		"base_branch",
		"organization_id",
		"repository_id",
		"project_id",
		"source_project_id",
		"target_project_id",
		"reviewer_user_ids",
		"trigger_ai_review_run",
		"work_item_ids",
		"remove_source_branch",
		"merge_method",
	} {
		if value, ok := cfg[key]; ok {
			dst[key] = value
		}
	}
}
