package provider

import (
	"context"
	"fmt"

	"github.com/yoke233/zhanggui/internal/core"
)

// CompositeProvider routes workspace preparation to the appropriate provider
// based on the selected ResourceSpace's Kind. If the WorkItem has a specific
// ResourceSpaceID, that space is used; otherwise the first space is used.
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

func (c *CompositeProvider) Prepare(ctx context.Context, project *core.Project, spaces []*core.ResourceSpace, workItemID int64) (*core.Workspace, error) {
	if len(spaces) == 0 {
		return nil, fmt.Errorf("no resource spaces available for workspace preparation")
	}

	// Use the first space to determine provider kind.
	space := spaces[0]
	p, ok := c.providers[space.Kind]
	if !ok {
		return nil, fmt.Errorf("no workspace provider for resource kind %q", space.Kind)
	}
	return p.Prepare(ctx, project, spaces, workItemID)
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
