package provider

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
)

// CompositeProvider routes workspace preparation to the appropriate provider
// based on the selected ResourceBinding's Kind. If the WorkItem has a specific
// ResourceBindingID, that binding is used; otherwise the first binding is used.
//
// Routing:
//
//	kind="git"      → GitProvider (local path or remote URL)
//	kind="local_fs" → LocalDirProvider
//	(fallback)      → error
type CompositeProvider struct {
	providers map[string]core.WorkspaceProvider
}

func NewCompositeProvider() *CompositeProvider {
	return &CompositeProvider{
		providers: map[string]core.WorkspaceProvider{
			core.ResourceKindGit:     &GitProvider{},
			core.ResourceKindLocalFS: &LocalDirProvider{},
		},
	}
}

func (c *CompositeProvider) RegisterProvider(kind string, p core.WorkspaceProvider) {
	c.providers[kind] = p
}

func (c *CompositeProvider) Prepare(ctx context.Context, project *core.Project, bindings []*core.ResourceBinding, issueID int64) (*core.Workspace, error) {
	if len(bindings) == 0 {
		return nil, fmt.Errorf("no resource bindings available for workspace preparation")
	}

	// Use the first binding to determine provider kind.
	binding := bindings[0]
	p, ok := c.providers[binding.Kind]
	if !ok {
		return nil, fmt.Errorf("no workspace provider for resource kind %q", binding.Kind)
	}
	return p.Prepare(ctx, project, bindings, issueID)
}

func (c *CompositeProvider) Release(ctx context.Context, ws *core.Workspace) error {
	if ws == nil || ws.Metadata == nil {
		return nil
	}
	kind, _ := ws.Metadata["kind"].(string)
	p, ok := c.providers[kind]
	if !ok {
		return nil
	}
	return p.Release(ctx, ws)
}
