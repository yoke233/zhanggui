package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	workspaceclone "github.com/yoke233/ai-workflow/internal/adapters/workspace/clone"
	workspacegit "github.com/yoke233/ai-workflow/internal/adapters/workspace/git"
	"github.com/yoke233/ai-workflow/internal/core"
)

// GitProvider handles workspace preparation for git resource bindings.
// It supports two URI modes:
//
//  1. Local path (e.g. "/home/user/my-repo") — creates a worktree directly.
//  2. Remote URL (e.g. "https://github.com/org/repo.git") — clones to a
//     local data directory first, then creates a worktree from the clone.
//
// The clone directory defaults to ".ai-workflow/repos/{owner}/{repo}" under
// the current working directory, but can be overridden via the binding's
// Config["clone_dir"] field.
type GitProvider struct {
	// DataDir is the base directory for cloning remote repos.
	// If empty, defaults to ".ai-workflow/repos" under cwd.
	DataDir string
}

func (p *GitProvider) Prepare(_ context.Context, _ *core.Project, bindings []*core.ResourceBinding, issueID int64) (*core.Workspace, error) {
	var gitBindings []*core.ResourceBinding
	for _, b := range bindings {
		if b == nil || b.Kind != core.ResourceKindGit {
			continue
		}
		gitBindings = append(gitBindings, b)
	}
	if len(gitBindings) == 0 {
		return nil, fmt.Errorf("no git resource binding found")
	}
	if len(gitBindings) > 1 {
		return nil, fmt.Errorf("multiple git resource bindings found; work item must select one binding explicitly")
	}

	b := gitBindings[0]
	repoPath, err := p.resolveRepoPath(b)
	if err != nil {
		return nil, fmt.Errorf("resolve git repo for binding %d: %w", b.ID, err)
	}

	branchName := fmt.Sprintf("ai-flow/issue-%d", issueID)
	worktreePath := filepath.Join(repoPath, ".worktrees", fmt.Sprintf("issue-%d", issueID))

	runner := workspacegit.NewRunner(repoPath)
	if err := runner.WorktreeAdd(worktreePath, branchName); err != nil {
		return nil, fmt.Errorf("create worktree for issue %d: %w", issueID, err)
	}

	defaultBranch := DefaultBranchFromBinding(b)
	if defaultBranch == "" {
		defaultBranch = workspacegit.DetectDefaultBranch(repoPath)
	}

	metadata := map[string]any{
		"binding_id":     b.ID,
		"kind":           core.ResourceKindGit,
		"branch":         branchName,
		"default_branch": defaultBranch,
		"repo_path":      repoPath,
	}
	MergeSCMBindingMetadata(metadata, b.Config)

	return &core.Workspace{
		Path:     worktreePath,
		Metadata: metadata,
	}, nil
}

func (p *GitProvider) Release(_ context.Context, ws *core.Workspace) error {
	if ws == nil || ws.Metadata == nil {
		return nil
	}
	repoPath, _ := ws.Metadata["repo_path"].(string)
	if repoPath == "" {
		return nil
	}
	runner := workspacegit.NewRunner(repoPath)
	return runner.WorktreeRemove(ws.Path)
}

// resolveRepoPath returns the local path to the git repository.
// For local paths it returns the URI directly.
// For remote URLs it clones (or fetches) into the data directory.
func (p *GitProvider) resolveRepoPath(b *core.ResourceBinding) (string, error) {
	uri := strings.TrimSpace(b.URI)
	if uri == "" {
		return "", fmt.Errorf("git resource binding has empty URI")
	}

	// Detect remote URL: contains "://" or starts with "git@".
	if isRemoteGitURI(uri) {
		return p.ensureClone(b, uri)
	}

	// Local path — verify it exists and has .git.
	info, err := os.Stat(uri)
	if err != nil {
		return "", fmt.Errorf("local git path %s: %w", uri, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("local git path %s is not a directory", uri)
	}

	// Auto-detect: if it's a local dir with .git, use it as-is.
	// If it's a local dir without .git, it might still work as a workspace
	// but we should warn/proceed.
	return uri, nil
}

// ensureClone clones a remote git repo or fetches updates if already cloned.
func (p *GitProvider) ensureClone(b *core.ResourceBinding, remoteURL string) (string, error) {
	// Determine clone target directory.
	cloneDir := ""
	if b.Config != nil {
		if d, ok := b.Config["clone_dir"].(string); ok && d != "" {
			cloneDir = d
		}
	}
	if cloneDir == "" {
		// Parse remote URL to derive a reasonable local directory name.
		meta, err := workspaceclone.ParseRemoteURL(remoteURL)
		if err != nil {
			return "", fmt.Errorf("parse remote URL: %w", err)
		}
		base := p.DataDir
		if base == "" {
			base = ".ai-workflow/repos"
		}
		cloneDir = filepath.Join(base, meta.Owner, meta.Repo)
	}

	cloner := workspaceclone.New()
	ref := ""
	if b.Config != nil {
		if r, ok := b.Config["ref"].(string); ok {
			ref = r
		}
	}
	result, err := cloner.Clone(context.Background(), workspaceclone.CloneRequest{
		RemoteURL:  remoteURL,
		TargetPath: cloneDir,
		Ref:        ref,
	})
	if err != nil {
		return "", fmt.Errorf("clone/fetch %s: %w", remoteURL, err)
	}
	return result.RepoPath, nil
}

// isRemoteGitURI returns true if the URI looks like a remote git URL
// (https://, ssh://, git@host:path, etc.)
func isRemoteGitURI(uri string) bool {
	if strings.Contains(uri, "://") {
		return true
	}
	// git@host:owner/repo.git format
	if strings.HasPrefix(uri, "git@") && strings.Contains(uri, ":") {
		return true
	}
	return false
}

func DefaultBranchFromBinding(b *core.ResourceBinding) string {
	if b == nil || b.Config == nil {
		return "main"
	}
	for _, key := range []string{"base_branch", "default_branch"} {
		if v, ok := b.Config[key].(string); ok && v != "" {
			return v
		}
	}
	return "main"
}

func MergeSCMBindingMetadata(dst map[string]any, cfg map[string]any) {
	if dst == nil || cfg == nil {
		return
	}
	for _, key := range []string{
		"provider",
		"default_branch",
		"base_branch",
		"organization_id",
		"repository_id",
		"project_id",
		"source_project_id",
		"target_project_id",
		"reviewer_user_ids",
		"trigger_ai_review_run",
		"work_item_ids",
		"remove_source_branch",
		"merge_method",
	} {
		if value, ok := cfg[key]; ok {
			dst[key] = value
		}
	}
}
