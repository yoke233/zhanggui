package outbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceCleanupWorkdirRemovesWorktree(t *testing.T) {
	repoDir := t.TempDir()

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v, out=%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}

	runGit("init")

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit("add", ".")
	runGit("-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init")

	workflowPath := filepath.Join(repoDir, "workflow.toml")
	workflow := `
version = 2

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[workdir]
enabled = true
backend = "git-worktree"
root = ".worktrees/runs"
cleanup = "manual"
roles = ["backend"]

[roles]
enabled = ["backend"]

[repos]
main = "."

[role_repo]
backend = "main"
`
	if err := os.WriteFile(workflowPath, []byte(workflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	issueRef := "local#1"
	runID := "2026-02-15-backend-0001"
	expectedWorkdir := filepath.Join(repoDir, ".worktrees", "runs", "backend", "local_1", runID)
	if err := os.MkdirAll(filepath.Dir(expectedWorkdir), 0o755); err != nil {
		t.Fatalf("MkdirAll(parent) error = %v", err)
	}
	runGit("worktree", "add", "--detach", expectedWorkdir, "HEAD")

	svc := &Service{}
	result, err := svc.CleanupWorkdir(context.Background(), CleanupWorkdirInput{
		WorkflowFile: workflowPath,
		Role:         "backend",
		IssueRef:     issueRef,
		RunID:        runID,
	})
	if err != nil {
		t.Fatalf("CleanupWorkdir() error = %v", err)
	}
	if filepath.Clean(result.Workdir) != filepath.Clean(expectedWorkdir) {
		t.Fatalf("workdir = %q, want %q", result.Workdir, expectedWorkdir)
	}

	if _, err := os.Stat(expectedWorkdir); err == nil {
		t.Fatalf("workdir should be removed: %s", expectedWorkdir)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat workdir error = %v", err)
	}
}

func TestServiceCleanupWorkdirErrorsWhenWorkdirDisabled(t *testing.T) {
	tempDir := t.TempDir()
	workflowPath := filepath.Join(tempDir, "workflow.toml")
	workflow := `
version = 2

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[workdir]
enabled = false
backend = "git-worktree"
root = ".worktrees/runs"
cleanup = "manual"
roles = ["backend"]

[roles]
enabled = ["backend"]

[repos]
main = "."

[role_repo]
backend = "main"
`
	if err := os.WriteFile(workflowPath, []byte(workflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	svc := &Service{}
	if _, err := svc.CleanupWorkdir(context.Background(), CleanupWorkdirInput{
		WorkflowFile: workflowPath,
		Role:         "backend",
		IssueRef:     "local#1",
		RunID:        "2026-02-15-backend-0001",
	}); err == nil {
		t.Fatalf("CleanupWorkdir() expected error for disabled workdir")
	}
}
