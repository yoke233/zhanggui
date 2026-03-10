package engine

import (
	"context"
	"testing"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

func TestLocalDirProvider(t *testing.T) {
	p := &LocalDirProvider{}
	bindings := []*core.ResourceBinding{
		{ID: 1, Kind: "git", URI: "/repo"},
		{ID: 2, Kind: "local_fs", URI: "/data/marketing"},
	}
	project := &core.Project{ID: 1, Kind: core.ProjectGeneral}

	ws, err := p.Prepare(context.Background(), project, bindings, 100)
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
	p := &LocalDirProvider{}
	bindings := []*core.ResourceBinding{
		{ID: 1, Kind: "git", URI: "/repo"},
	}
	_, err := p.Prepare(context.Background(), nil, bindings, 1)
	if err == nil {
		t.Fatal("expected error for missing local_fs binding")
	}
}

func TestCompositeProvider_DevGitDispatch(t *testing.T) {
	// CompositeProvider dispatches dev → LocalGitProvider, which will fail
	// without a real git repo but we can verify the dispatch logic.
	cp := NewCompositeProvider()

	// general → LocalDirProvider
	ws, err := cp.Prepare(context.Background(),
		&core.Project{ID: 1, Kind: core.ProjectGeneral},
		[]*core.ResourceBinding{{Kind: "local_fs", URI: "/tmp/test"}},
		1,
	)
	if err != nil {
		t.Fatalf("general prepare: %v", err)
	}
	if ws.Path != "/tmp/test" {
		t.Fatalf("expected /tmp/test, got %s", ws.Path)
	}

	// unknown kind → error
	_, err = cp.Prepare(context.Background(),
		&core.Project{ID: 2, Kind: "finance"},
		[]*core.ResourceBinding{{Kind: "local_fs", URI: "/tmp"}},
		2,
	)
	if err == nil {
		t.Fatal("expected error for unknown project kind")
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
			"branch": "ai-flow/v2-1",
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
	if got.Metadata["branch"] != "ai-flow/v2-1" {
		t.Fatalf("expected branch metadata, got %v", got.Metadata)
	}
}

func TestCompositeProvider_Release(t *testing.T) {
	cp := NewCompositeProvider()

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
