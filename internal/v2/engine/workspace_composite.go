package engine

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// CompositeProvider dispatches to the appropriate WorkspaceProvider based on project.Kind.
type CompositeProvider struct {
	providers map[core.ProjectKind]core.WorkspaceProvider
}

// NewCompositeProvider creates a CompositeProvider with default sub-providers:
// - ProjectDev → LocalGitProvider (worktree isolation)
// - ProjectGeneral → LocalDirProvider (direct directory)
func NewCompositeProvider() *CompositeProvider {
	return &CompositeProvider{
		providers: map[core.ProjectKind]core.WorkspaceProvider{
			core.ProjectDev:     &LocalGitProvider{},
			core.ProjectGeneral: &LocalDirProvider{},
		},
	}
}

// RegisterProvider adds or replaces a provider for a given project kind.
func (c *CompositeProvider) RegisterProvider(kind core.ProjectKind, p core.WorkspaceProvider) {
	c.providers[kind] = p
}

func (c *CompositeProvider) Prepare(ctx context.Context, project *core.Project, bindings []*core.ResourceBinding, flowID int64) (*core.Workspace, error) {
	p, ok := c.providers[project.Kind]
	if !ok {
		return nil, fmt.Errorf("no workspace provider for project kind %q", project.Kind)
	}
	return p.Prepare(ctx, project, bindings, flowID)
}

func (c *CompositeProvider) Release(ctx context.Context, ws *core.Workspace) error {
	if ws == nil || ws.Metadata == nil {
		return nil
	}
	kind, _ := ws.Metadata["kind"].(string)
	switch kind {
	case "git":
		return (&LocalGitProvider{}).Release(ctx, ws)
	default:
		return (&LocalDirProvider{}).Release(ctx, ws)
	}
}
