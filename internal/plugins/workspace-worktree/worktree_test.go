package workspaceworktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	commands := [][]string{
		{"git", "init", repo},
		{"git", "-C", repo, "config", "user.email", "test@example.com"},
		{"git", "-C", repo, "config", "user.name", "test-user"},
		{"git", "-C", repo, "commit", "--allow-empty", "-m", "init"},
	}
	for _, cmd := range commands {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("cmd %v failed: %s (%v)", cmd, string(out), err)
		}
	}
	return repo
}

func TestWorktreePlugin_SetupCreatesWorktree(t *testing.T) {
	repo := setupGitRepo(t)
	plugin := New()

	result, err := plugin.Setup(context.Background(), core.WorkspaceSetupRequest{
		RepoPath: repo,
		RunID:    "20260302-p35",
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if result.BranchName != "ai-flow/20260302-p35" {
		t.Fatalf("unexpected branch name: %q", result.BranchName)
	}
	if result.WorktreePath != filepath.Join(repo, ".worktrees", "20260302-p35") {
		t.Fatalf("unexpected worktree path: %q", result.WorktreePath)
	}
	if result.BaseBranch == "" {
		t.Fatal("base branch must not be empty")
	}
	if _, err := os.Stat(result.WorktreePath); err != nil {
		t.Fatalf("worktree path should exist after setup, stat err=%v", err)
	}
}

func TestWorktreePlugin_CleanupRemovesWorktree(t *testing.T) {
	repo := setupGitRepo(t)
	worktreePath := filepath.Join(t.TempDir(), "wt-cleanup")
	plugin := New()

	_, err := plugin.Setup(context.Background(), core.WorkspaceSetupRequest{
		RepoPath:     repo,
		RunID:        "pipe-cleanup",
		BranchName:   "ai-flow/pipe-cleanup",
		WorktreePath: worktreePath,
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := plugin.Cleanup(context.Background(), core.WorkspaceCleanupRequest{
		RepoPath:     repo,
		WorktreePath: worktreePath,
	}); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree path should be removed, stat err=%v", err)
	}
}

func TestWorktreePlugin_SetupRejectsEmptyRepoPath(t *testing.T) {
	plugin := New()
	_, err := plugin.Setup(context.Background(), core.WorkspaceSetupRequest{
		RunID: "pipe-err",
	})
	if err == nil {
		t.Fatal("expected error when repo path is empty")
	}
}
