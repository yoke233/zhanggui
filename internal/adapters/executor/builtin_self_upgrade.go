package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// UpgradeFunc is called after a successful build to trigger a restart with the new binary.
// The string argument is the absolute path to the newly built binary.
type UpgradeFunc func(binaryPath string)

func runBuiltinSelfUpgrade(ctx context.Context, store core.Store, bus core.EventBus, step *core.Step, execRec *core.Execution, upgradeFn UpgradeFunc) error {
	if store == nil {
		return fmt.Errorf("builtin self_upgrade: store is nil")
	}
	if upgradeFn == nil {
		return fmt.Errorf("builtin self_upgrade: upgrade function not configured (restart not supported)")
	}

	// --- Resolve config ---
	repoPath := "."
	defaultBranch := "main"
	binaryOutput := ""
	triggerRestart := true

	if step.Config != nil {
		if v, ok := step.Config["repo_path"].(string); ok && strings.TrimSpace(v) != "" {
			repoPath = strings.TrimSpace(v)
		}
		if v, ok := step.Config["default_branch"].(string); ok && strings.TrimSpace(v) != "" {
			defaultBranch = strings.TrimSpace(v)
		}
		if v, ok := step.Config["binary_output"].(string); ok && strings.TrimSpace(v) != "" {
			binaryOutput = strings.TrimSpace(v)
		}
		if v, ok := step.Config["restart"].(bool); ok {
			triggerRestart = v
		}
	}
	if defaultBranch == "main" {
		if inferred, err := gitOutput(ctx, repoPath, nil, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
			inferred = strings.TrimSpace(inferred)
			if strings.HasPrefix(inferred, "origin/") {
				trimmed := strings.TrimSpace(strings.TrimPrefix(inferred, "origin/"))
				if trimmed != "" {
					defaultBranch = trimmed
				}
			}
		}
	}

	// Default binary output: same as current executable.
	if binaryOutput == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("builtin self_upgrade: cannot determine current executable: %w", err)
		}
		binaryOutput = exe
	}

	// --- Pre-flight check 1: current branch must be default branch ---
	currentBranch, err := gitOutput(ctx, repoPath, nil, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("builtin self_upgrade: failed to detect current branch: %w", err)
	}
	currentBranch = strings.TrimSpace(currentBranch)
	if currentBranch != defaultBranch {
		return fmt.Errorf("builtin self_upgrade: refusing to upgrade — current branch is %q, expected %q; switch to %q first",
			currentBranch, defaultBranch, defaultBranch)
	}

	// --- Pre-flight check 2: working tree must be clean ---
	hasChanges, err := gitHasChanges(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("builtin self_upgrade: failed to check working tree: %w", err)
	}
	if hasChanges {
		return fmt.Errorf("builtin self_upgrade: refusing to upgrade — working tree has uncommitted changes; commit or stash them first")
	}

	// --- Fetch latest ---
	if err := gitRun(ctx, repoPath, nil, "fetch", "origin", defaultBranch); err != nil {
		return fmt.Errorf("builtin self_upgrade: fetch failed: %w", err)
	}

	// --- Check if behind ---
	behindCount, err := gitOutput(ctx, repoPath, nil, "rev-list", "--count", "HEAD..origin/"+defaultBranch)
	if err != nil {
		return fmt.Errorf("builtin self_upgrade: failed to check remote status: %w", err)
	}
	behind := strings.TrimSpace(behindCount)

	pulled := false
	if behind != "0" {
		// --- Pull (fast-forward only) ---
		if err := gitRun(ctx, repoPath, nil, "pull", "--ff-only", "origin", defaultBranch); err != nil {
			return fmt.Errorf("builtin self_upgrade: pull --ff-only failed (diverged?): %w", err)
		}
		pulled = true
	}

	// --- Build ---
	headSHA, _ := gitOutput(ctx, repoPath, nil, "rev-parse", "HEAD")
	headSHA = strings.TrimSpace(headSHA)

	buildArgs := []string{"build", "-o", binaryOutput, "./cmd/ai-flow"}
	if err := goRun(ctx, repoPath, buildArgs...); err != nil {
		return fmt.Errorf("builtin self_upgrade: go build failed: %w", err)
	}

	// --- Store artifact BEFORE triggering restart ---
	result := map[string]any{
		"branch":        currentBranch,
		"head_sha":      headSHA,
		"pulled":        pulled,
		"behind_count":  behind,
		"binary_output": binaryOutput,
		"restart":       triggerRestart,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}
	markdown := fmt.Sprintf("self_upgrade: built %s at %s (pulled=%v)", headSHA[:minLen(headSHA, 8)], binaryOutput, pulled)

	if err := storeBuiltinArtifact(ctx, store, bus, step, execRec, markdown, result); err != nil {
		return fmt.Errorf("builtin self_upgrade: failed to store artifact: %w", err)
	}

	// --- Trigger restart with new binary ---
	if triggerRestart {
		upgradeFn(binaryOutput)
	}

	return nil
}

func goRun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("go %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func minLen(s string, n int) int {
	if len(s) < n {
		return len(s)
	}
	return n
}
