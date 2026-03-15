package flow

import (
	"context"
	"testing"

	workspaceprovider "github.com/yoke233/ai-workflow/internal/adapters/workspace/provider"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestLocalDirProvider(t *testing.T) {
	p := &workspaceprovider.LocalDirProvider{}
	spaces := []*core.ResourceSpace{
		{ID: 1, Kind: "git", RootURI: "/repo"},
		{ID: 2, Kind: "local_fs", RootURI: "/data/marketing"},
	}
	project := &core.Project{ID: 1, Kind: core.ProjectGeneral}

	ws, err := p.Prepare(context.Background(), project, spaces, 100)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if ws.Path != "/data/marketing" {
		t.Fatalf("expected /data/marketing, got %s", ws.Path)
	}
	if ws.Metadata["kind"] != "local_fs" {
		t.Fatalf("expected kind=local_fs, got %v", ws.Metadata["kind"])
	}

	// Release should be no-op
	if err := p.Release(context.Background(), ws); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestLocalDirProvider_NoBinding(t *testing.T) {
	p := &workspaceprovider.LocalDirProvider{}
	spaces := []*core.ResourceSpace{
		{ID: 1, Kind: "git", RootURI: "/repo"},
	}
	_, err := p.Prepare(context.Background(), nil, spaces, 1)
	if err == nil {
		t.Fatal("expected error for missing local_fs space")
	}
}

func TestCompositeProvider_SpaceKindDispatch(t *testing.T) {
	// CompositeProvider routes by space kind, not project kind.
	cp := workspaceprovider.NewCompositeProvider()

	// local_fs space → LocalDirProvider (any project kind)
	ws, err := cp.Prepare(context.Background(),
		&core.Project{ID: 1, Kind: core.ProjectGeneral},
		[]*core.ResourceSpace{{Kind: "local_fs", RootURI: "/tmp/test"}},
		1,
	)
	if err != nil {
		t.Fatalf("local_fs prepare: %v", err)
	}
	if ws.Path != "/tmp/test" {
		t.Fatalf("expected /tmp/test, got %s", ws.Path)
	}

	// dev project with local_fs space still works
	ws2, err := cp.Prepare(context.Background(),
		&core.Project{ID: 2, Kind: core.ProjectDev},
		[]*core.ResourceSpace{{Kind: "local_fs", RootURI: "/tmp/dev"}},
		2,
	)
	if err != nil {
		t.Fatalf("dev+local_fs prepare: %v", err)
	}
	if ws2.Path != "/tmp/dev" {
		t.Fatalf("expected /tmp/dev, got %s", ws2.Path)
	}

	// unknown space kind → error
	_, err = cp.Prepare(context.Background(),
		&core.Project{ID: 3, Kind: core.ProjectGeneral},
		[]*core.ResourceSpace{{Kind: "unknown_kind", RootURI: "/tmp"}},
		3,
	)
	if err == nil {
		t.Fatal("expected error for unknown space kind")
	}
}

func TestWorkspaceContext(t *testing.T) {
	ctx := context.Background()

	// No workspace in context
	if ws := WorkspaceFromContext(ctx); ws != nil {
		t.Fatal("expected nil workspace from empty context")
	}

	// Set workspace
	ws := &core.Workspace{
		Path: "/test/path",
		Env:  map[string]string{"FOO": "bar"},
		Metadata: map[string]any{
			"branch": "ai-flow/flow-1",
		},
	}
	ctx = ContextWithWorkspace(ctx, ws)

	got := WorkspaceFromContext(ctx)
	if got == nil {
		t.Fatal("expected workspace from context")
	}
	if got.Path != "/test/path" {
		t.Fatalf("expected /test/path, got %s", got.Path)
	}
	if got.Env["FOO"] != "bar" {
		t.Fatalf("expected FOO=bar, got %v", got.Env)
	}
	if got.Metadata["branch"] != "ai-flow/flow-1" {
		t.Fatalf("expected branch metadata, got %v", got.Metadata)
	}
}

func TestCompositeProvider_Release(t *testing.T) {
	cp := workspaceprovider.NewCompositeProvider()

	// Release nil workspace — should not error
	if err := cp.Release(context.Background(), nil); err != nil {
		t.Fatalf("release nil: %v", err)
	}

	// Release local_fs workspace — no-op
	ws := &core.Workspace{
		Path:     "/tmp/test",
		Metadata: map[string]any{"kind": "local_fs"},
	}
	if err := cp.Release(context.Background(), ws); err != nil {
		t.Fatalf("release local_fs: %v", err)
	}
}

func TestDefaultBranchFromSpace_PrefersBaseBranch(t *testing.T) {
	space := &core.ResourceSpace{
		Kind: "git",
		Config: map[string]any{
			"default_branch": "main",
			"base_branch":    "release/2026.03",
		},
	}
	if got := workspaceprovider.DefaultBranchFromSpace(space); got != "release/2026.03" {
		t.Fatalf("DefaultBranchFromSpace = %q", got)
	}
}

func TestMergeSCMSpaceMetadata_CopiesCodeupDefaults(t *testing.T) {
	dst := map[string]any{}
	workspaceprovider.MergeSCMSpaceMetadata(dst, map[string]any{
		"provider":             "codeup",
		"organization_id":      "5f6ea0829cffa29cfdd39a7f",
		"project_id":           2369234,
		"base_branch":          "main",
		"remove_source_branch": true,
	})
	if dst["provider"] != "codeup" {
		t.Fatalf("provider = %#v", dst["provider"])
	}
	if dst["organization_id"] != "5f6ea0829cffa29cfdd39a7f" {
		t.Fatalf("organization_id = %#v", dst["organization_id"])
	}
	if dst["base_branch"] != "main" {
		t.Fatalf("base_branch = %#v", dst["base_branch"])
	}
}
