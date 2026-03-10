package engine

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/yoke233/ai-workflow/internal/git"
	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// LocalGitProvider creates git worktree-based workspaces for dev projects.
// Each flow gets an isolated worktree with a dedicated branch.
type LocalGitProvider struct{}

func (p *LocalGitProvider) Prepare(_ context.Context, _ *core.Project, bindings []*core.ResourceBinding, flowID int64) (*core.Workspace, error) {
	for _, b := range bindings {
		if b.Kind != "git" {
			continue
		}
		repoPath := b.URI
		branchName := fmt.Sprintf("ai-flow/v2-%d", flowID)
		worktreePath := filepath.Join(repoPath, ".worktrees", fmt.Sprintf("flow-%d", flowID))

		runner := git.NewRunner(repoPath)
		if err := runner.WorktreeAdd(worktreePath, branchName); err != nil {
			return nil, fmt.Errorf("create worktree for flow %d: %w", flowID, err)
		}

		defaultBranch := "main"
		if db, ok := b.Config["default_branch"].(string); ok && db != "" {
			defaultBranch = db
		} else {
			defaultBranch = git.DetectDefaultBranch(repoPath)
		}

		return &core.Workspace{
			Path: worktreePath,
			Metadata: map[string]any{
				"binding_id":     b.ID,
				"kind":           "git",
				"branch":         branchName,
				"default_branch": defaultBranch,
				"repo_path":      repoPath,
			},
		}, nil
	}
	return nil, fmt.Errorf("no git resource binding found")
}

func (p *LocalGitProvider) Release(_ context.Context, ws *core.Workspace) error {
	if ws == nil || ws.Metadata == nil {
		return nil
	}
	repoPath, _ := ws.Metadata["repo_path"].(string)
	if repoPath == "" {
		return nil
	}
	runner := git.NewRunner(repoPath)
	return runner.WorktreeRemove(ws.Path)
}
