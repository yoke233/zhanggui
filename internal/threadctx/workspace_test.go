package threadctx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

func newTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.New(filepath.Join(t.TempDir(), "threadctx.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestResolveMount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	projectID, err := store.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
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

	mount, err := ResolveMount(ctx, store, &core.ThreadContextRef{
		ThreadID:  1,
		ProjectID: projectID,
		Access:    core.ContextAccessCheck,
	})
	if err != nil {
		t.Fatalf("ResolveMount: %v", err)
	}
	if mount.Slug != "project-alpha" {
		t.Fatalf("expected slug project-alpha, got %q", mount.Slug)
	}
	if mount.TargetPath != projectDir {
		t.Fatalf("expected target path %q, got %q", projectDir, mount.TargetPath)
	}
	if len(mount.CheckCommands) != 1 || mount.CheckCommands[0] != "go test ./..." {
		t.Fatalf("unexpected check commands: %+v", mount.CheckCommands)
	}
}

func TestBuildWorkspaceContext(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "Thread Alpha", OwnerID: "owner-1"})
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

	projectID, err := store.CreateProject(ctx, &core.Project{Name: "Project Beta", Kind: core.ProjectGeneral})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   t.TempDir(),
	}); err != nil {
		t.Fatalf("create resource space: %v", err)
	}
	if _, err := store.CreateThreadContextRef(ctx, &core.ThreadContextRef{
		ThreadID:  threadID,
		ProjectID: projectID,
		Access:    core.ContextAccessRead,
	}); err != nil {
		t.Fatalf("create context ref: %v", err)
	}

	payload, err := BuildWorkspaceContext(ctx, store, t.TempDir(), threadID)
	if err != nil {
		t.Fatalf("BuildWorkspaceContext: %v", err)
	}
	if payload.ThreadID != threadID {
		t.Fatalf("unexpected thread id: %d", payload.ThreadID)
	}
	if payload.Workspace != "." || payload.Archive != "../archive" {
		t.Fatalf("unexpected workspace payload: %+v", payload)
	}
	mount, ok := payload.Mounts["project-beta"]
	if !ok {
		t.Fatalf("expected project-beta mount, got %+v", payload.Mounts)
	}
	if mount.Access != core.ContextAccessRead {
		t.Fatalf("expected read access, got %q", mount.Access)
	}
	if len(payload.Members) != 1 || payload.Members[0] != "owner-1" {
		t.Fatalf("unexpected members: %+v", payload.Members)
	}
}

func TestBuildWorkspaceContextIncludesArchiveSnapshots(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dataDir := t.TempDir()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "Thread Alpha", OwnerID: "owner-1"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	paths, err := EnsureLayout(dataDir, threadID)
	if err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.WorkspaceDir, "notes.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}
	if err := SyncDailyArchive(paths, time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SyncDailyArchive: %v", err)
	}

	payload, err := BuildWorkspaceContext(ctx, store, dataDir, threadID)
	if err != nil {
		t.Fatalf("BuildWorkspaceContext: %v", err)
	}
	if len(payload.Archives) != 1 {
		t.Fatalf("expected 1 archive snapshot, got %+v", payload.Archives)
	}
	if payload.Archives[0].Date != "2026-03-15" || payload.Archives[0].Manifest == "" || payload.Archives[0].FileCount != 1 {
		t.Fatalf("unexpected archive snapshot metadata: %+v", payload.Archives[0])
	}
}

func TestSyncContextFileAndLoadContextFileRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dataDir := t.TempDir()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "Thread Alpha", OwnerID: "owner-1"})
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

	projectID, _ := store.CreateProject(ctx, &core.Project{Name: "Project Gamma", Kind: core.ProjectGeneral})
	if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   t.TempDir(),
		Config: map[string]any{
			"check_commands": []any{"go test ./...", "npm test"},
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

	if _, err := SyncContextFile(ctx, store, dataDir, threadID); err != nil {
		t.Fatalf("SyncContextFile: %v", err)
	}
	loaded, err := LoadContextFile(dataDir, threadID)
	if err != nil {
		t.Fatalf("LoadContextFile: %v", err)
	}
	if loaded.ThreadID != threadID {
		t.Fatalf("unexpected thread id: %d", loaded.ThreadID)
	}
	if len(loaded.Mounts["project-gamma"].CheckCommands) != 2 {
		t.Fatalf("expected 2 check commands, got %+v", loaded.Mounts["project-gamma"].CheckCommands)
	}
}

func TestSyncContextFileCreatesArchiveSnapshotAndManifest(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	dataDir := t.TempDir()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "Thread Alpha", OwnerID: "owner-1"})
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
	paths, err := EnsureLayout(dataDir, threadID)
	if err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.WorkspaceDir, "plan.md"), []byte("draft"), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	if _, err := SyncContextFile(ctx, store, dataDir, threadID); err != nil {
		t.Fatalf("SyncContextFile: %v", err)
	}

	snapshotDir := filepath.Join(paths.ArchiveDir, time.Now().UTC().Format("2006-01-02"))
	if _, err := os.Stat(filepath.Join(snapshotDir, "plan.md")); err != nil {
		t.Fatalf("expected archived plan.md: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(snapshotDir, archiveManifestName))
	if err != nil {
		t.Fatalf("read archive manifest: %v", err)
	}
	if !strings.Contains(string(raw), "plan.md") {
		t.Fatalf("expected manifest to contain plan.md, got %s", string(raw))
	}

	loaded, err := LoadContextFile(dataDir, threadID)
	if err != nil {
		t.Fatalf("LoadContextFile: %v", err)
	}
	if len(loaded.Archives) != 1 || loaded.Archives[0].Manifest == "" {
		t.Fatalf("expected archive snapshot in context file, got %+v", loaded.Archives)
	}
}

func TestResolveMountUsesGitCloneDirForRemoteBinding(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	projectID, _ := store.CreateProject(ctx, &core.Project{Name: "Remote Repo", Kind: core.ProjectGeneral})
	cloneDir := t.TempDir()
	if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindGit,
		RootURI:   "https://github.com/acme/demo.git",
		Config: map[string]any{
			"clone_dir":      cloneDir,
			"check_commands": []string{"go test ./..."},
		},
	}); err != nil {
		t.Fatalf("create git resource space: %v", err)
	}

	mount, err := ResolveMount(ctx, store, &core.ThreadContextRef{
		ThreadID:  1,
		ProjectID: projectID,
		Access:    core.ContextAccessCheck,
	})
	if err != nil {
		t.Fatalf("ResolveMount: %v", err)
	}
	if mount.TargetPath != cloneDir {
		t.Fatalf("expected clone_dir %q, got %q", cloneDir, mount.TargetPath)
	}
}

type threadctxStoreStub struct {
	getThreadErr         error
	getProjectErr        error
	listMembers          []*core.ThreadMember
	listThreadContextRef []*core.ThreadContextRef
	listSpaces           []*core.ResourceSpace
	listMembersErr       error
	listRefsErr          error
	listSpacesErr        error
	project              *core.Project
}

func (s *threadctxStoreStub) GetThread(context.Context, int64) (*core.Thread, error) {
	if s.getThreadErr != nil {
		return nil, s.getThreadErr
	}
	return &core.Thread{ID: 1, Title: "thread"}, nil
}

func (s *threadctxStoreStub) GetProject(context.Context, int64) (*core.Project, error) {
	if s.getProjectErr != nil {
		return nil, s.getProjectErr
	}
	if s.project != nil {
		return s.project, nil
	}
	return &core.Project{ID: 1, Name: "Project"}, nil
}

func (s *threadctxStoreStub) ListThreadMembers(context.Context, int64) ([]*core.ThreadMember, error) {
	return s.listMembers, s.listMembersErr
}

func (s *threadctxStoreStub) ListThreadContextRefs(context.Context, int64) ([]*core.ThreadContextRef, error) {
	return s.listThreadContextRef, s.listRefsErr
}

func (s *threadctxStoreStub) ListResourceSpaces(context.Context, int64) ([]*core.ResourceSpace, error) {
	return s.listSpaces, s.listSpacesErr
}

func TestPathsAndEnsureLayout(t *testing.T) {
	paths := Paths("  "+t.TempDir()+"  ", 42)
	if filepath.Base(paths.ThreadDir) != "42" {
		t.Fatalf("unexpected thread dir: %q", paths.ThreadDir)
	}
	if filepath.Base(paths.ContextFile) != ".context.json" {
		t.Fatalf("unexpected context file: %q", paths.ContextFile)
	}

	got, err := EnsureLayout("", 42)
	if err != nil {
		t.Fatalf("EnsureLayout(empty) error = %v", err)
	}
	if got != (PathsInfo{}) {
		t.Fatalf("EnsureLayout(empty) = %+v, want zero value", got)
	}

	dataDir := t.TempDir()
	got, err = EnsureLayout(dataDir, 99)
	if err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	for _, dir := range []string{got.ThreadDir, got.WorkspaceDir, got.MountsDir, got.ArchiveDir} {
		info, statErr := os.Stat(dir)
		if statErr != nil || !info.IsDir() {
			t.Fatalf("expected directory %q to exist, err=%v", dir, statErr)
		}
	}
}

func TestSyncDailyArchiveSkipsContextOnlyWorkspaceAndPrunesOldSnapshots(t *testing.T) {
	dataDir := t.TempDir()
	paths, err := EnsureLayout(dataDir, 9)
	if err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	if err := os.WriteFile(paths.ContextFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write context file: %v", err)
	}
	if err := SyncDailyArchive(paths, time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SyncDailyArchive(context-only): %v", err)
	}
	entries, err := os.ReadDir(paths.ArchiveDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no snapshots for context-only workspace, got %+v", entries)
	}

	oldDir := filepath.Join(paths.ArchiveDir, "2026-03-01")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("mkdir old archive: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.WorkspaceDir, "notes.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}
	if err := SyncDailyArchive(paths, time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SyncDailyArchive(with files): %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("expected old archive to be pruned, stat err=%v", err)
	}
}

func TestSyncMountAliasDirsRemovesStaleDirs(t *testing.T) {
	mountsDir := filepath.Join(t.TempDir(), "mounts")
	if err := os.MkdirAll(filepath.Join(mountsDir, "stale-project"), 0o755); err != nil {
		t.Fatalf("mkdir stale dir: %v", err)
	}
	payload := &core.ThreadWorkspaceContext{
		Mounts: map[string]core.ThreadWorkspaceMount{
			"project-alpha": {Path: "../mounts/project-alpha"},
		},
	}
	if err := syncMountAliasDirs(mountsDir, payload, nil); err != nil {
		t.Fatalf("syncMountAliasDirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mountsDir, "project-alpha")); err != nil {
		t.Fatalf("expected fresh mount dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mountsDir, "stale-project")); !os.IsNotExist(err) {
		t.Fatalf("expected stale mount dir removed, err=%v", err)
	}
}

func TestSyncMountAliasDirsCreatesMountDirForResolvedTarget(t *testing.T) {
	mountsDir := filepath.Join(t.TempDir(), "mounts")
	targetDir := filepath.Join(t.TempDir(), "project-alpha")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	payload := &core.ThreadWorkspaceContext{
		Mounts: map[string]core.ThreadWorkspaceMount{
			"project-alpha": {Path: "../mounts/project-alpha"},
		},
	}
	if err := syncMountAliasDirs(mountsDir, payload, map[string]string{"project-alpha": targetDir}); err != nil {
		t.Fatalf("syncMountAliasDirs: %v", err)
	}
	info, err := os.Stat(filepath.Join(mountsDir, "project-alpha"))
	if err != nil || !info.IsDir() {
		t.Fatalf("expected mount alias dir, err=%v", err)
	}
}

func TestLoadContextFileAndBuildWorkspaceContextErrors(t *testing.T) {
	if _, err := LoadContextFile(t.TempDir(), 1); err == nil {
		t.Fatal("expected missing context file to fail")
	}

	dataDir := t.TempDir()
	paths, err := EnsureLayout(dataDir, 2)
	if err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	if err := os.WriteFile(paths.ContextFile, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadContextFile(dataDir, 2); err == nil {
		t.Fatal("expected broken context file to fail")
	}

	if _, err := BuildWorkspaceContext(context.Background(), nil, dataDir, 1); err == nil {
		t.Fatal("expected nil store to fail")
	}
	if _, err := SyncContextFile(context.Background(), nil, dataDir, 1); err == nil {
		t.Fatal("expected nil store sync to fail")
	}

	store := &threadctxStoreStub{getThreadErr: core.ErrNotFound}
	if _, err := BuildWorkspaceContext(context.Background(), store, dataDir, 1); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected thread lookup error, got %v", err)
	}
}

func TestBuildWorkspaceContextSkipsBrokenMountsAndDeduplicatesMembers(t *testing.T) {
	store := &threadctxStoreStub{
		project: &core.Project{ID: 7, Name: "Project Name"},
		listMembers: []*core.ThreadMember{
			nil,
			{UserID: "owner-1"},
			{UserID: "owner-1"},
			{UserID: "member-2"},
			{UserID: "   "},
		},
		listThreadContextRef: []*core.ThreadContextRef{
			{ThreadID: 1, ProjectID: 7, Access: core.ContextAccessCheck},
			{ThreadID: 1, ProjectID: 8, Access: core.ContextAccessRead},
		},
		listSpaces: []*core.ResourceSpace{
			{ProjectID: 7, Kind: core.ResourceKindLocalFS, RootURI: t.TempDir()},
		},
	}

	payload, err := BuildWorkspaceContext(context.Background(), store, t.TempDir(), 1)
	if err != nil {
		t.Fatalf("BuildWorkspaceContext() error = %v", err)
	}
	if len(payload.Members) != 2 || payload.Members[0] != "member-2" || payload.Members[1] != "owner-1" {
		t.Fatalf("unexpected members: %+v", payload.Members)
	}
	if len(payload.Mounts) != 1 {
		t.Fatalf("expected broken mount to be skipped, got %+v", payload.Mounts)
	}
}

func TestResolveMountAndHelpersErrors(t *testing.T) {
	store := &threadctxStoreStub{}
	if _, err := ResolveMount(context.Background(), nil, &core.ThreadContextRef{}); err == nil {
		t.Fatal("expected nil store to fail")
	}
	if _, err := ResolveMount(context.Background(), store, nil); err == nil {
		t.Fatal("expected nil ref to fail")
	}

	store.project = &core.Project{ID: 3, Name: "Project"}
	store.listSpaces = []*core.ResourceSpace{{Kind: core.ResourceKindGit, RootURI: "https://example.com/repo.git"}}
	if _, err := ResolveMount(context.Background(), store, &core.ThreadContextRef{ProjectID: 3, Access: core.ContextAccessRead}); err == nil {
		t.Fatal("expected unresolved binding to fail")
	}

	if path, checks := resolveSpaceTarget([]*core.ResourceSpace{
		nil,
		{Kind: core.ResourceKindGit, RootURI: "git@github.com:org/repo.git", Config: map[string]any{"clone_dir": "C:/repo", "check_commands": []any{"go test ./...", "  ", 123}}},
	}); path != "C:/repo" || len(checks) != 1 || checks[0] != "go test ./..." {
		t.Fatalf("unexpected resolveSpaceTarget result: path=%q checks=%v", path, checks)
	}
	if path := resolveGitSpacePath(&core.ResourceSpace{Kind: core.ResourceKindGit, RootURI: "C:/repo"}); path != "C:/repo" {
		t.Fatalf("resolveGitSpacePath(local) = %q", path)
	}
	if !looksLikeRemoteGitURI("git@github.com:org/repo.git") || !looksLikeRemoteGitURI("https://github.com/org/repo.git") {
		t.Fatal("expected remote git uris to be detected")
	}
	if looksLikeRemoteGitURI("C:/repo") {
		t.Fatal("expected local path not to be treated as remote")
	}
	if got := readCheckCommands(map[string]any{"check_commands": []string{"go test ./...", "  "}}); len(got) != 1 {
		t.Fatalf("unexpected []string check commands: %v", got)
	}
	if got := readCheckCommands(map[string]any{"check_commands": "go test ./..."}); got != nil {
		t.Fatalf("expected unsupported check_commands type to be ignored, got %v", got)
	}
}

func TestProjectSlugFallbacks(t *testing.T) {
	if got := projectSlug(nil); got != "project" {
		t.Fatalf("projectSlug(nil) = %q", got)
	}
	if got := projectSlug(&core.Project{ID: 12, Name: "  "}); got != "project-12" {
		t.Fatalf("projectSlug(blank) = %q", got)
	}
	if got := projectSlug(&core.Project{ID: 13, Name: "Hello__World !!"}); got != "hello-world" {
		t.Fatalf("projectSlug(normalized) = %q", got)
	}
}

func TestSyncContextFileEmptyDataDirAndResolveMountErrors(t *testing.T) {
	store := &threadctxStoreStub{}
	payload, err := SyncContextFile(context.Background(), store, "", 1)
	if err != nil {
		t.Fatalf("SyncContextFile(empty data dir) error = %v", err)
	}
	if payload != nil {
		t.Fatalf("SyncContextFile(empty data dir) = %+v, want nil", payload)
	}

	store = &threadctxStoreStub{
		getProjectErr: core.ErrNotFound,
		listSpacesErr: errors.New("bindings failed"),
	}
	if _, err := ResolveMount(context.Background(), store, &core.ThreadContextRef{ProjectID: 1}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected project lookup error, got %v", err)
	}

	store = &threadctxStoreStub{
		project:       &core.Project{ID: 1, Name: "Project"},
		listSpacesErr: errors.New("bindings failed"),
	}
	if _, err := ResolveMount(context.Background(), store, &core.ThreadContextRef{ProjectID: 1}); err == nil || err.Error() != "bindings failed" {
		t.Fatalf("expected space list error, got %v", err)
	}

	if got := resolveGitSpacePath(&core.ResourceSpace{Kind: core.ResourceKindGit, RootURI: "https://example.com/repo.git"}); got != "" {
		t.Fatalf("resolveGitSpacePath(remote without clone dir) = %q, want empty", got)
	}
	if got := resolveGitSpacePath(nil); got != "" {
		t.Fatalf("resolveGitSpacePath(nil) = %q, want empty", got)
	}
}
