package core

import "context"

// Workspace represents the prepared execution environment for a WorkItem.
type Workspace struct {
	Path     string            // agent working directory
	Env      map[string]string // extra environment variables
	Metadata map[string]any    // provider-specific data (branch name, repo path, etc.)
}

// WorkspaceProvider prepares and releases execution workspaces for WorkItems.
type WorkspaceProvider interface {
	Prepare(ctx context.Context, project *Project, bindings []*ResourceBinding, workItemID int64) (*Workspace, error)
	Release(ctx context.Context, ws *Workspace) error
}
