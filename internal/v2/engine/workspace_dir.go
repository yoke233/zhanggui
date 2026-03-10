package engine

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// LocalDirProvider creates workspaces from local_fs resource bindings.
// Prepare returns the first local_fs binding's URI as the working directory.
// Release is a no-op since no temporary resources are created.
type LocalDirProvider struct{}

func (p *LocalDirProvider) Prepare(_ context.Context, _ *core.Project, bindings []*core.ResourceBinding, _ int64) (*core.Workspace, error) {
	for _, b := range bindings {
		if b.Kind == "local_fs" {
			return &core.Workspace{
				Path: b.URI,
				Metadata: map[string]any{
					"binding_id": b.ID,
					"kind":       "local_fs",
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("no local_fs resource binding found")
}

func (p *LocalDirProvider) Release(_ context.Context, _ *core.Workspace) error {
	return nil
}
