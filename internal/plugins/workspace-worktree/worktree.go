package workspaceworktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
	gitops "github.com/yoke233/ai-workflow/internal/git"
)

type WorktreePlugin struct{}

func New() *WorktreePlugin {
	return &WorktreePlugin{}
}

func (p *WorktreePlugin) Name() string {
	return "workspace-worktree"
}

func (p *WorktreePlugin) Init(context.Context) error {
	return nil
}

func (p *WorktreePlugin) Close() error {
	return nil
}

func (p *WorktreePlugin) Setup(_ context.Context, req core.WorkspaceSetupRequest) (core.WorkspaceSetupResult, error) {
	repoPath := strings.TrimSpace(req.RepoPath)
	if repoPath == "" {
		return core.WorkspaceSetupResult{}, errors.New("workspace repo path is empty")
	}

	RunID := strings.TrimSpace(req.RunID)
	if RunID == "" {
		return core.WorkspaceSetupResult{}, errors.New("workspace Run id is empty")
	}

	branchName := strings.TrimSpace(req.BranchName)
	if branchName == "" {
		branchName = "ai-flow/" + RunID
	}

	worktreePath := strings.TrimSpace(req.WorktreePath)
	if worktreePath == "" {
		worktreePath = filepath.Join(repoPath, ".worktrees", RunID)
	}
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return core.WorkspaceSetupResult{}, fmt.Errorf("ensure worktree parent dir: %w", err)
	}

	runner := gitops.NewRunner(repoPath)
	if _, err := os.Stat(worktreePath); errors.Is(err, os.ErrNotExist) {
		if err := runner.WorktreeAdd(worktreePath, branchName); err != nil {
			return core.WorkspaceSetupResult{}, err
		}
	} else if err != nil {
		return core.WorkspaceSetupResult{}, err
	}

	baseBranch, err := runner.CurrentBranch()
	if err != nil {
		return core.WorkspaceSetupResult{}, err
	}

	return core.WorkspaceSetupResult{
		BranchName:   branchName,
		WorktreePath: worktreePath,
		BaseBranch:   baseBranch,
	}, nil
}

func (p *WorktreePlugin) Cleanup(_ context.Context, req core.WorkspaceCleanupRequest) error {
	worktreePath := strings.TrimSpace(req.WorktreePath)
	if worktreePath == "" {
		return nil
	}

	repoPath := strings.TrimSpace(req.RepoPath)
	if repoPath == "" {
		return errors.New("workspace repo path is empty")
	}

	runner := gitops.NewRunner(repoPath)
	return runner.WorktreeRemove(worktreePath)
}

func Module() core.PluginModule {
	return core.PluginModule{
		Name: "worktree",
		Slot: core.SlotWorkspace,
		Factory: func(map[string]any) (core.Plugin, error) {
			return New(), nil
		},
	}
}

var _ core.WorkspacePlugin = (*WorktreePlugin)(nil)
