package outbox

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGitWorktreeManagerCleanupManualSkipsGitOps(t *testing.T) {
	tempDir := t.TempDir()
	workflowFile := filepath.Join(tempDir, "workflow.toml")

	mgr, err := newGitWorktreeManager(workflowWorkdirConfig{
		Enabled: true,
		Backend: "git-worktree",
		Root:    "runs",
		Cleanup: "manual",
		Roles:   []string{"backend"},
	}, workflowFile, tempDir)
	if err != nil {
		t.Fatalf("newGitWorktreeManager() error = %v", err)
	}

	// Ensure no git command is called when cleanup policy is manual.
	mgr.runGit = func(_ context.Context, _ ...string) ([]byte, error) {
		t.Fatalf("runGit should not be called for manual cleanup")
		return nil, nil
	}

	workdir := filepath.Join(tempDir, "runs", "backend", "local_1", "2026-02-15-backend-0001")
	if err := mgr.Cleanup(context.Background(), "backend", "local#1", "2026-02-15-backend-0001", workdir); err != nil {
		t.Fatalf("Cleanup(manual) error = %v", err)
	}
}
