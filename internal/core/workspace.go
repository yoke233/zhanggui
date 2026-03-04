package core

import "context"

// WorkspaceSetupRequest defines inputs for provisioning Run workspace.
type WorkspaceSetupRequest struct {
	RepoPath     string `json:"repo_path"`
	RunID        string `json:"run_id"`
	BranchName   string `json:"branch_name"`
	WorktreePath string `json:"worktree_path"`
}

// WorkspaceSetupResult captures effective workspace details after setup.
type WorkspaceSetupResult struct {
	BranchName   string `json:"branch_name"`
	WorktreePath string `json:"worktree_path"`
	BaseBranch   string `json:"base_branch"`
}

// WorkspaceCleanupRequest defines inputs for cleaning up Run workspace.
type WorkspaceCleanupRequest struct {
	RepoPath     string `json:"repo_path"`
	WorktreePath string `json:"worktree_path"`
}

// WorkspacePlugin provisions and cleans stage execution workspaces.
type WorkspacePlugin interface {
	Plugin
	Setup(ctx context.Context, req WorkspaceSetupRequest) (WorkspaceSetupResult, error)
	Cleanup(ctx context.Context, req WorkspaceCleanupRequest) error
}
