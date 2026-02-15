package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
)

var errWorkdirDirty = errors.New("workdir is dirty")

type workdirManager interface {
	Prepare(ctx context.Context, role string, issueRef string, runID string) (string, error)
	Cleanup(ctx context.Context, role string, issueRef string, runID string, workdir string) error
}

type gitWorktreeManager struct {
	repoDir     string
	allowedRoot string
	runGit      func(context.Context, ...string) ([]byte, error)
}

func newGitWorktreeManager(cfg workflowWorkdirConfig, workflowFile string, repoDir string) (*gitWorktreeManager, error) {
	if !cfg.Enabled {
		return nil, errors.New("workdir is disabled")
	}

	backend := strings.TrimSpace(cfg.Backend)
	if backend != "git-worktree" {
		return nil, fmt.Errorf("unsupported workdir backend %q", backend)
	}
	cleanup := strings.TrimSpace(cfg.Cleanup)
	if cleanup != "immediate" {
		return nil, fmt.Errorf("unsupported workdir cleanup policy %q", cleanup)
	}

	workflowFile = strings.TrimSpace(workflowFile)
	if workflowFile == "" {
		return nil, errors.New("workflow file is required")
	}
	workflowAbs, err := filepath.Abs(workflowFile)
	if err != nil {
		return nil, errs.Wrap(err, "resolve workflow file abs path")
	}

	repoDir = strings.TrimSpace(repoDir)
	if repoDir == "" {
		return nil, errors.New("repo dir is required")
	}
	repoAbs, err := filepath.Abs(repoDir)
	if err != nil {
		return nil, errs.Wrap(err, "resolve repo dir abs path")
	}

	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		return nil, errors.New("workdir root is required when enabled")
	}

	workflowDir := filepath.Dir(workflowAbs)
	var rootAbs string
	if filepath.IsAbs(root) {
		rootAbs = filepath.Clean(root)
	} else {
		rootAbs = filepath.Clean(filepath.Join(workflowDir, root))
	}
	rootAbs, err = filepath.Abs(rootAbs)
	if err != nil {
		return nil, errs.Wrap(err, "resolve workdir root abs path")
	}

	return &gitWorktreeManager{
		repoDir:     repoAbs,
		allowedRoot: rootAbs,
		runGit: func(ctx context.Context, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, "git", args...)
			return cmd.CombinedOutput()
		},
	}, nil
}

func (m *gitWorktreeManager) Prepare(ctx context.Context, role string, issueRef string, runID string) (string, error) {
	if ctx == nil {
		return "", errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	workdir, err := m.workdirPath(role, issueRef, runID)
	if err != nil {
		return "", err
	}
	if err := ensurePathInsideDir(m.allowedRoot, workdir); err != nil {
		return "", err
	}
	if _, err := os.Stat(workdir); err == nil {
		return "", fmt.Errorf("workdir already exists: %s", workdir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errs.Wrap(err, "check workdir path")
	}
	if err := os.MkdirAll(filepath.Dir(workdir), 0o755); err != nil {
		return "", errs.Wrap(err, "ensure workdir parent directory")
	}

	if err := m.ensureRepoIsGit(ctx); err != nil {
		return "", err
	}

	output, err := m.gitC(ctx, m.repoDir, "worktree", "add", "--detach", workdir, "HEAD")
	if err != nil {
		m.cleanupFailedPrepare(ctx, workdir)
		return "", errs.Wrapf(err, "git worktree add failed: %s", strings.TrimSpace(string(output)))
	}

	logging.Info(
		logging.WithAttrs(ctx, slog.String("component", "outbox.workdir")),
		"git worktree prepared",
		slog.String("repo_dir", m.repoDir),
		slog.String("workdir", workdir),
		slog.String("issue_ref", issueRef),
		slog.String("run_id", runID),
		slog.String("role", role),
	)
	return workdir, nil
}

func (m *gitWorktreeManager) Cleanup(ctx context.Context, role string, issueRef string, runID string, workdir string) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return nil
	}
	if err := ensurePathInsideDir(m.allowedRoot, workdir); err != nil {
		return err
	}

	if _, err := os.Stat(workdir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = m.pruneBestEffort(ctx)
			return nil
		}
		return errs.Wrap(err, "stat workdir before cleanup")
	}

	statusOutput, err := m.gitC(ctx, workdir, "status", "--porcelain")
	if err != nil {
		return errs.Wrapf(err, "git status failed: %s", strings.TrimSpace(string(statusOutput)))
	}
	if strings.TrimSpace(string(statusOutput)) != "" {
		return errs.Wrapf(errWorkdirDirty, "git status reports changes: %s", strings.TrimSpace(string(statusOutput)))
	}

	removeOutput, err := m.gitC(ctx, m.repoDir, "worktree", "remove", workdir)
	if err != nil {
		return errs.Wrapf(err, "git worktree remove failed: %s", strings.TrimSpace(string(removeOutput)))
	}

	if err := m.pruneBestEffort(ctx); err != nil {
		return err
	}

	logging.Info(
		logging.WithAttrs(ctx, slog.String("component", "outbox.workdir")),
		"git worktree cleaned up",
		slog.String("repo_dir", m.repoDir),
		slog.String("workdir", workdir),
		slog.String("issue_ref", issueRef),
		slog.String("run_id", runID),
		slog.String("role", role),
	)
	return nil
}

func (m *gitWorktreeManager) ensureRepoIsGit(ctx context.Context) error {
	output, err := m.gitC(ctx, m.repoDir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return errs.Wrapf(err, "git rev-parse failed: %s", strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(strings.ToLower(string(output))) != "true" {
		return fmt.Errorf("repo is not a git working tree: %s", m.repoDir)
	}
	return nil
}

func (m *gitWorktreeManager) pruneBestEffort(ctx context.Context) error {
	output, err := m.gitC(ctx, m.repoDir, "worktree", "prune")
	if err != nil {
		return errs.Wrapf(err, "git worktree prune failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *gitWorktreeManager) cleanupFailedPrepare(ctx context.Context, workdir string) {
	logCtx := logging.WithAttrs(ctx, slog.String("component", "outbox.workdir"))

	_ = m.pruneBestEffort(ctx)

	if err := ensurePathInsideDir(m.allowedRoot, workdir); err != nil {
		logging.Warn(logCtx, "skip removing failed workdir because it is outside allowed root", slog.Any("err", err))
		return
	}
	if err := os.RemoveAll(workdir); err != nil {
		logging.Warn(logCtx, "remove failed workdir directory failed", slog.Any("err", errs.Loggable(err)), slog.String("workdir", workdir))
	}
}

func (m *gitWorktreeManager) gitC(ctx context.Context, dir string, args ...string) ([]byte, error) {
	all := make([]string, 0, len(args)+2)
	all = append(all, "-C", dir)
	all = append(all, args...)
	return m.runGit(ctx, all...)
}

func (m *gitWorktreeManager) workdirPath(role string, issueRef string, runID string) (string, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return "", errors.New("role is required")
	}
	issueRef = strings.TrimSpace(issueRef)
	if issueRef == "" {
		return "", errors.New("issue ref is required")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", errors.New("run id is required")
	}

	sanitizedIssue := sanitizeWorkdirSegment(issueRef)
	sanitizedRole := sanitizeWorkdirSegment(role)
	sanitizedRun := sanitizeWorkdirSegment(runID)
	return filepath.Join(m.allowedRoot, sanitizedRole, sanitizedIssue, sanitizedRun), nil
}

func sanitizeWorkdirSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		"#", "_",
		":", "_",
		" ", "_",
	)
	return replacer.Replace(trimmed)
}

func ensurePathInsideDir(root string, target string) error {
	rootAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
	if err != nil {
		return errs.Wrap(err, "resolve root abs path")
	}
	targetAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(target)))
	if err != nil {
		return errs.Wrap(err, "resolve target abs path")
	}

	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return errs.Wrap(err, "resolve target relative path")
	}

	rel = filepath.Clean(rel)
	if rel == "." {
		return fmt.Errorf("target path is the root directory: %s", targetAbs)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("target path escapes root directory: %s (root=%s)", targetAbs, rootAbs)
	}
	return nil
}
