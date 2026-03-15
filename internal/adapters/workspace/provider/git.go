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

// GitProvider handles workspace preparation for git resource spaces.
// It supports two URI modes:
//
//  1. Local path (e.g. "/home/user/my-repo") — creates a worktree directly.
//  2. Remote URL (e.g. "https://github.com/org/repo.git") — clones to a
//     local data directory first, then creates a worktree from the clone.
//
// The clone directory defaults to ".ai-workflow/repos/{owner}/{repo}" under
// the current working directory, but can be overridden via the space's
// Config["clone_dir"] field.
type GitProvider struct {
	// DataDir is the base directory for cloning remote repos.
	// If empty, defaults to ".ai-workflow/repos" under cwd.
	DataDir string
}

func (p *GitProvider) Prepare(_ context.Context, _ *core.Project, spaces []*core.ResourceSpace, workItemID int64) (*core.Workspace, error) {
	var gitSpaces []*core.ResourceSpace
	for _, space := range spaces {
		if space == nil || space.Kind != core.ResourceKindGit {
			continue
		}
		gitSpaces = append(gitSpaces, space)
	}
	if len(gitSpaces) == 0 {
		return nil, fmt.Errorf("no git resource space found")
	}
	if len(gitSpaces) > 1 {
		return nil, fmt.Errorf("multiple git resource spaces found; work item must select one space explicitly")
	}

	space := gitSpaces[0]
	repoPath, err := p.resolveRepoPath(space)
	if err != nil {
		return nil, fmt.Errorf("resolve git repo for space %d: %w", space.ID, err)
	}

	runner := workspacegit.NewRunner(repoPath)

	// Determine the base branch for the new worktree.
	baseBranch := DefaultBranchFromSpace(space)
	if baseBranch == "" {
		baseBranch = workspacegit.DetectDefaultBranch(repoPath)
	}

	// Determine the start point for the new worktree branch.
	//
	// Priority order:
	//   1. origin/{baseBranch} (after fetch) — latest remote state
	//   2. local {baseBranch}                — if no remote or fetch failed
	//   3. HEAD                              — last resort
	//
	// Warnings are collected and stored in workspace metadata so the UI
	// can surface them to the user (e.g. "fetch failed, using local base").
	var warnings []string
	startPoint := ""
	remoteRef := "origin/" + baseBranch

	if runner.HasRemote("origin") {
		if fetchErr := runner.Fetch("origin"); fetchErr != nil {
			warnings = append(warnings, fmt.Sprintf(
				"无法从远端拉取最新代码 (git fetch origin): %v。工作区将基于本地缓存的版本创建。",
				fetchErr,
			))
		}

		if runner.RefExists(remoteRef) {
			startPoint = remoteRef
		} else {
			warnings = append(warnings, fmt.Sprintf(
				"远端分支 %s 不存在，将使用本地分支 %s 作为起点。",
				remoteRef, baseBranch,
			))
		}
	} else {
		// Pure local repo — no remote configured.
		warnings = append(warnings, "该仓库没有配置远端 (origin)，工作区将基于本地分支创建。")
	}

	// Fall back to local base branch if remote ref not available.
	if startPoint == "" && runner.RefExists(baseBranch) {
		startPoint = baseBranch
	}
	// If even the local base branch doesn't exist, startPoint stays "" and
	// WorktreeAdd will create the branch from the current HEAD.

	branchName := fmt.Sprintf("ai-flow/workitem-%d", workItemID)
	worktreePath := filepath.Join(repoPath, ".worktrees", fmt.Sprintf("workitem-%d", workItemID))

	if err := runner.WorktreeAdd(worktreePath, branchName, startPoint); err != nil {
		return nil, fmt.Errorf("create worktree for work item %d: %w", workItemID, err)
	}

	metadata := map[string]any{
		"space_id":       space.ID,
		"kind":           core.ResourceKindGit,
		"branch":         branchName,
		"default_branch": baseBranch,
		"repo_path":      repoPath,
	}
	if len(warnings) > 0 {
		metadata["warnings"] = warnings
	}
	MergeSCMSpaceMetadata(metadata, space.Config)

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
func (p *GitProvider) resolveRepoPath(space *core.ResourceSpace) (string, error) {
	uri := strings.TrimSpace(space.RootURI)
	if uri == "" {
		return "", fmt.Errorf("git resource space has empty root_uri")
	}

	// Detect remote URL: contains "://" or starts with "git@".
	if isRemoteGitURI(uri) {
		return p.ensureClone(space, uri)
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
func (p *GitProvider) ensureClone(space *core.ResourceSpace, remoteURL string) (string, error) {
	// Determine clone target directory.
	cloneDir := ""
	if space.Config != nil {
		if d, ok := space.Config["clone_dir"].(string); ok && d != "" {
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
	if space.Config != nil {
		if r, ok := space.Config["ref"].(string); ok {
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

func DefaultBranchFromSpace(space *core.ResourceSpace) string {
	if space == nil || space.Config == nil {
		return "main"
	}
	for _, key := range []string{"base_branch", "default_branch"} {
		if v, ok := space.Config[key].(string); ok && v != "" {
			return v
		}
	}
	return "main"
}

func MergeSCMSpaceMetadata(dst map[string]any, cfg map[string]any) {
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
