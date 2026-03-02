package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Runner struct {
	repoDir string
}

func NewRunner(repoDir string) *Runner {
	return &Runner{repoDir: repoDir}
}

func (r *Runner) run(args ...string) (string, error) {
	stdout, stderr, _, err := r.runRaw(args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr), err)
	}
	return strings.TrimSpace(stdout), nil
}

func (r *Runner) runRaw(args ...string) (string, string, int, error) {
	cmd := exec.Command("git", append([]string{"-C", r.repoDir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), stderr.String(), exitErr.ExitCode(), err
		}
		return stdout.String(), stderr.String(), -1, err
	}
	return stdout.String(), stderr.String(), 0, nil
}
