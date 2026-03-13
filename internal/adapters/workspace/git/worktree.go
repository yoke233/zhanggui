package git

import (
	"os"
	"os/exec"
	"strings"
)

func (r *Runner) WorktreeAdd(path, branch string) error {
	// Try creating a new branch via "git worktree add -b <branch> <path>".
	_, err := r.run("worktree", "add", "-b", branch, path)
	if err == nil {
		return nil
	}

	// If the branch already exists, remove orphaned directory (if any) and
	// check out the existing branch: "git worktree add <path> <branch>".
	if strings.Contains(err.Error(), "already exists") {
		_ = os.RemoveAll(path) // remove stale empty dir that blocks git
		_, retryErr := r.run("worktree", "add", path, branch)
		return retryErr
	}
	return err
}

func (r *Runner) WorktreeRemove(path string) error {
	_, err := r.run("worktree", "remove", path, "--force")
	return err
}

func (r *Runner) WorktreeClean(path string) error {
	cmd1 := exec.Command("git", "-C", path, "checkout", ".")
	if err := cmd1.Run(); err != nil {
		return err
	}
	cmd2 := exec.Command("git", "-C", path, "clean", "-fd")
	return cmd2.Run()
}
