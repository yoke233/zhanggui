package engine

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

type workspaceKey struct{}

// ContextWithWorkspace stores a Workspace in the context.
func ContextWithWorkspace(ctx context.Context, ws *core.Workspace) context.Context {
	return context.WithValue(ctx, workspaceKey{}, ws)
}

// WorkspaceFromContext retrieves the Workspace from the context, or nil if absent.
func WorkspaceFromContext(ctx context.Context) *core.Workspace {
	ws, _ := ctx.Value(workspaceKey{}).(*core.Workspace)
	return ws
}
