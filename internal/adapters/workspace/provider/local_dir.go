package provider

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
)

type LocalDirProvider struct{}

func (p *LocalDirProvider) Prepare(_ context.Context, _ *core.Project, spaces []*core.ResourceSpace, _ int64) (*core.Workspace, error) {
	for _, space := range spaces {
		if space.Kind == core.ResourceKindLocalFS {
			return &core.Workspace{
				Path: space.RootURI,
				Metadata: map[string]any{
					"space_id": space.ID,
					"kind":     core.ResourceKindLocalFS,
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("no local_fs resource space found")
}

func (p *LocalDirProvider) Release(_ context.Context, _ *core.Workspace) error {
	return nil
}
