package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	workspaceclone "github.com/yoke233/ai-workflow/internal/adapters/workspace/clone"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

func runBuiltinGitCommitPush(ctx context.Context, store core.Store, bus core.EventBus, tokens flowapp.GitHubTokens, step *core.Step, execRec *core.Execution) error {
	if store == nil {
		return fmt.Errorf("builtin git_commit_push: store is nil")
	}
	ws := flowapp.WorkspaceFromContext(ctx)
	if ws == nil || strings.TrimSpace(ws.Path) == "" {
		return fmt.Errorf("builtin git_commit_push: workspace is required")
	}

	message := "chore: ai-flow runtime update"
	if step.Config != nil {
		if v, ok := step.Config["commit_message"].(string); ok && strings.TrimSpace(v) != "" {
			message = strings.TrimSpace(v)
		}
	}

	// Commit if there are changes.
	if err := gitRun(ctx, ws.Path, nil, "add", "-A"); err != nil {
		return err
	}
	hasChanges, err := gitHasChanges(ctx, ws.Path)
	if err != nil {
		return err
	}
	if !hasChanges {
		return storeBuiltinArtifact(ctx, store, bus, step, execRec, "git_commit_push: no changes to commit", map[string]any{
			"commit": "skipped",
		})
	}

	if err := gitRun(ctx, ws.Path, nil,
		"-c", "user.name=ai-flow",
		"-c", "user.email=ai-flow@local",
		"commit", "-m", message,
	); err != nil {
		return err
	}

	// Ensure origin is HTTPS so GIT_ASKPASS works for PAT auth (avoid SSH prompts).
	originURL, err := gitOutput(ctx, ws.Path, nil, "remote", "get-url", "origin")
	if err == nil {
		if remote, parseErr := workspaceclone.ParseRemoteURL(strings.TrimSpace(originURL)); parseErr == nil {
			if strings.EqualFold(strings.TrimSpace(remote.Host), "github.com") {
				httpsURL := fmt.Sprintf("https://github.com/%s/%s.git", remote.Owner, remote.Repo)
				_ = gitRun(ctx, ws.Path, nil, "remote", "set-url", "origin", httpsURL)
			}
		}
	}

	// Push using PAT via GIT_ASKPASS (token chosen by engine option; never logged).
	repoPath := ""
	if ws.Metadata != nil {
		if v, ok := ws.Metadata["repo_path"].(string); ok {
			repoPath = strings.TrimSpace(v)
		}
	}
	if repoPath == "" {
		repoPath = ws.Path
	}
	token := tokens.EffectiveCommitPAT()
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("builtin git_commit_push: missing commit PAT")
	}

	branch := ""
	if ws.Metadata != nil {
		if v, ok := ws.Metadata["branch"].(string); ok {
			branch = strings.TrimSpace(v)
		}
	}
	if branch == "" {
		branch = "HEAD"
	}

	pushWithToken := func(tok string) error {
		askpassPath, cleanup, err := writeAskPassCmd(tok)
		if err != nil {
			return err
		}
		defer cleanup()
		env := []string{
			"GIT_ASKPASS=" + askpassPath,
			"GIT_TERMINAL_PROMPT=0",
		}
		return gitRun(ctx, ws.Path, env, "push", "-u", "origin", branch)
	}

	if err := pushWithToken(token); err != nil {
		mergeTok := strings.TrimSpace(tokens.MergePAT)
		if mergeTok != "" && mergeTok != strings.TrimSpace(token) && isAuthError(err) {
			if err2 := pushWithToken(mergeTok); err2 != nil {
				return err2
			}
		} else {
			return err
		}
	}

	sha, _ := gitOutput(ctx, ws.Path, nil, "rev-parse", "HEAD")
	return storeBuiltinArtifact(ctx, store, bus, step, execRec, "git_commit_push: pushed changes", map[string]any{
		"commit":    "pushed",
		"branch":    branch,
		"head_sha":  strings.TrimSpace(sha),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"repo_path": repoPath,
		"worktree":  ws.Path,
	})
}

func gitHasChanges(ctx context.Context, dir string) (bool, error) {
	out, err := gitOutput(ctx, dir, nil, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func gitRun(ctx context.Context, dir string, extraEnv []string, args ...string) error {
	_, err := gitOutput(ctx, dir, extraEnv, args...)
	return err
}

func gitOutput(ctx context.Context, dir string, extraEnv []string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
