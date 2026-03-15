package agentruntime

import (
	"context"
	"path/filepath"
	"testing"

	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

type stubAgentRegistry struct{}

func (stubAgentRegistry) GetProfile(context.Context, string) (*core.AgentProfile, error) {
	return nil, core.ErrProfileNotFound
}
func (stubAgentRegistry) ListProfiles(context.Context) ([]*core.AgentProfile, error) {
	return nil, nil
}
func (stubAgentRegistry) CreateProfile(context.Context, *core.AgentProfile) error {
	return nil
}
func (stubAgentRegistry) UpdateProfile(context.Context, *core.AgentProfile) error {
	return nil
}
func (stubAgentRegistry) DeleteProfile(context.Context, string) error {
	return nil
}
func (stubAgentRegistry) ResolveForAction(context.Context, *core.Action) (*core.AgentProfile, error) {
	return nil, core.ErrProfileNotFound
}
func (stubAgentRegistry) ResolveByID(context.Context, string) (*core.AgentProfile, error) {
	return nil, core.ErrProfileNotFound
}

func newThreadSessionPoolTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.New(filepath.Join(t.TempDir(), "thread-session-pool.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestPrepareThreadWorkspaceBuildsMountConfig(t *testing.T) {
	store := newThreadSessionPoolTestStore(t)
	ctx := context.Background()
	dataDir := t.TempDir()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "thread-1", OwnerID: "owner-1"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := store.AddThreadMember(ctx, &core.ThreadMember{
		ThreadID: threadID,
		Kind:     core.ThreadMemberKindHuman,
		UserID:   "owner-1",
		Role:     "owner",
	}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	projectID, err := store.CreateProject(ctx, &core.Project{Name: "Project Delta", Kind: core.ProjectGeneral})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	projectDir := t.TempDir()
	if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   projectDir,
		Config: map[string]any{
			"check_commands": []string{"go test ./..."},
		},
	}); err != nil {
		t.Fatalf("create resource space: %v", err)
	}
	if _, err := store.CreateThreadContextRef(ctx, &core.ThreadContextRef{
		ThreadID:  threadID,
		ProjectID: projectID,
		Access:    core.ContextAccessCheck,
	}); err != nil {
		t.Fatalf("create context ref: %v", err)
	}

	pool := NewThreadSessionPool(store, membus.NewBus(), stubAgentRegistry{}, dataDir)
	workspaceDir, cfg, err := pool.prepareThreadWorkspace(ctx, threadID)
	if err != nil {
		t.Fatalf("prepareThreadWorkspace: %v", err)
	}
	if workspaceDir == "" || cfg.WorkspaceDir != workspaceDir {
		t.Fatalf("unexpected workspace dir: %q / %+v", workspaceDir, cfg)
	}
	if len(cfg.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %+v", cfg.Mounts)
	}
	if cfg.Mounts[0].Alias != "project-delta" || cfg.Mounts[0].TargetPath != projectDir {
		t.Fatalf("unexpected mount config: %+v", cfg.Mounts[0])
	}
}

func TestPrepareThreadWorkspaceRequiresDataDir(t *testing.T) {
	store := newThreadSessionPoolTestStore(t)
	pool := NewThreadSessionPool(store, membus.NewBus(), stubAgentRegistry{}, "")
	if _, _, err := pool.prepareThreadWorkspace(context.Background(), 1); err == nil {
		t.Fatal("expected prepareThreadWorkspace to fail without dataDir")
	}
}
