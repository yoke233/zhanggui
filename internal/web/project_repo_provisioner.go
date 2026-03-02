package web

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	projectSourceTypeLocalPath   = "local_path"
	projectSourceTypeLocalNew    = "local_new"
	projectSourceTypeGitHubClone = "github_clone"
)

var (
	reProjectSlug          = regexp.MustCompile(`[^a-z0-9_-]+`)
	reGitHubNameCharacters = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// ProjectRepoProvisionInput carries source-specific fields for repository preparation.
type ProjectRepoProvisionInput struct {
	SourceType string

	RepoPath string
	Slug     string

	GitHubOwner string
	GitHubRepo  string
	GitHubRef   string

	Progress func(step, message string)
}

// ProjectRepoProvisionResult contains repository path plus source metadata.
type ProjectRepoProvisionResult struct {
	RepoPath string

	GitHubOwner string
	GitHubRepo  string
}

// ProjectRepoProvisioner prepares a usable repository for project creation.
type ProjectRepoProvisioner interface {
	Provision(ctx context.Context, input ProjectRepoProvisionInput) (ProjectRepoProvisionResult, error)
}

type gitCommandRunner interface {
	Run(ctx context.Context, args ...string) error
}

type shellGitCommandRunner struct{}

func (r shellGitCommandRunner) Run(ctx context.Context, args ...string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

type projectRepoProvisioner struct {
	reposRoot string
	gitRunner gitCommandRunner
}

// NewProjectRepoProvisioner creates a default provisioner. reposRoot can be empty to use ~/.ai-workflow/repos.
func NewProjectRepoProvisioner(reposRoot string) ProjectRepoProvisioner {
	return newProjectRepoProvisioner(reposRoot, shellGitCommandRunner{})
}

func newProjectRepoProvisioner(reposRoot string, gitRunner gitCommandRunner) *projectRepoProvisioner {
	if gitRunner == nil {
		gitRunner = shellGitCommandRunner{}
	}
	return &projectRepoProvisioner{
		reposRoot: strings.TrimSpace(reposRoot),
		gitRunner: gitRunner,
	}
}

func (p *projectRepoProvisioner) Provision(ctx context.Context, input ProjectRepoProvisionInput) (ProjectRepoProvisionResult, error) {
	sourceType := strings.TrimSpace(input.SourceType)
	switch sourceType {
	case projectSourceTypeLocalPath:
		return p.provisionLocalPath(input)
	case projectSourceTypeLocalNew:
		return p.provisionLocalNew(ctx, input)
	case projectSourceTypeGitHubClone:
		return p.provisionGitHubClone(ctx, input)
	default:
		return ProjectRepoProvisionResult{}, fmt.Errorf("unsupported source_type: %s", sourceType)
	}
}

func (p *projectRepoProvisioner) provisionLocalPath(input ProjectRepoProvisionInput) (ProjectRepoProvisionResult, error) {
	repoPath := strings.TrimSpace(input.RepoPath)
	if repoPath == "" {
		return ProjectRepoProvisionResult{}, fmt.Errorf("repo_path is required for local_path")
	}
	notifyProvisionProgress(input.Progress, "resolve_local_path", "using submitted local repository path")
	return ProjectRepoProvisionResult{
		RepoPath: filepath.Clean(repoPath),
	}, nil
}

func (p *projectRepoProvisioner) provisionLocalNew(ctx context.Context, input ProjectRepoProvisionInput) (ProjectRepoProvisionResult, error) {
	slug := normalizeProjectSlug(input.Slug)
	if slug == "" {
		return ProjectRepoProvisionResult{}, fmt.Errorf("slug is required for local_new")
	}

	reposRoot, err := p.resolveReposRoot()
	if err != nil {
		return ProjectRepoProvisionResult{}, err
	}
	notifyProvisionProgress(input.Progress, "ensure_repo_root", "ensuring repository root directory")
	if err := os.MkdirAll(reposRoot, 0o755); err != nil {
		return ProjectRepoProvisionResult{}, fmt.Errorf("create repos root: %w", err)
	}

	repoPath := filepath.Join(reposRoot, slug)
	notifyProvisionProgress(input.Progress, "create_directory", "creating local repository directory")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		return ProjectRepoProvisionResult{}, fmt.Errorf("create local repository directory: %w", err)
	}

	notifyProvisionProgress(input.Progress, "git_init", "initializing git repository")
	if err := p.gitRunner.Run(ctx, "init", repoPath); err != nil {
		return ProjectRepoProvisionResult{}, err
	}

	return ProjectRepoProvisionResult{
		RepoPath: repoPath,
	}, nil
}

func (p *projectRepoProvisioner) provisionGitHubClone(ctx context.Context, input ProjectRepoProvisionInput) (ProjectRepoProvisionResult, error) {
	owner := strings.TrimSpace(input.GitHubOwner)
	repo := strings.TrimSpace(input.GitHubRepo)
	ref := strings.TrimSpace(input.GitHubRef)

	if owner == "" {
		return ProjectRepoProvisionResult{}, fmt.Errorf("github.owner is required for github_clone")
	}
	if repo == "" {
		return ProjectRepoProvisionResult{}, fmt.Errorf("github.repo is required for github_clone")
	}
	if !isValidGitHubNamePart(owner) {
		return ProjectRepoProvisionResult{}, fmt.Errorf("github.owner contains unsupported characters")
	}
	if !isValidGitHubNamePart(repo) {
		return ProjectRepoProvisionResult{}, fmt.Errorf("github.repo contains unsupported characters")
	}

	reposRoot, err := p.resolveReposRoot()
	if err != nil {
		return ProjectRepoProvisionResult{}, err
	}
	notifyProvisionProgress(input.Progress, "ensure_repo_root", "ensuring repository root directory")
	if err := os.MkdirAll(reposRoot, 0o755); err != nil {
		return ProjectRepoProvisionResult{}, fmt.Errorf("create repos root: %w", err)
	}

	targetPath := filepath.Join(reposRoot, owner+"__"+repo)
	remoteURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	if pathExists(targetPath) {
		notifyProvisionProgress(input.Progress, "update_repository", "updating existing repository from GitHub")
		if err := p.gitRunner.Run(ctx, "-C", targetPath, "fetch", "--all", "--prune"); err != nil {
			return ProjectRepoProvisionResult{}, err
		}
	} else {
		notifyProvisionProgress(input.Progress, "clone_repository", "cloning repository from GitHub")
		args := []string{"clone"}
		if ref != "" {
			args = append(args, "--branch", ref)
		}
		args = append(args, remoteURL, targetPath)
		if err := p.gitRunner.Run(ctx, args...); err != nil {
			return ProjectRepoProvisionResult{}, err
		}
	}

	if ref != "" {
		notifyProvisionProgress(input.Progress, "checkout_ref", "checking out requested ref")
		if err := p.gitRunner.Run(ctx, "-C", targetPath, "checkout", ref); err != nil {
			return ProjectRepoProvisionResult{}, err
		}
	}

	return ProjectRepoProvisionResult{
		RepoPath:    targetPath,
		GitHubOwner: owner,
		GitHubRepo:  repo,
	}, nil
}

func (p *projectRepoProvisioner) resolveReposRoot() (string, error) {
	if p.reposRoot != "" {
		return filepath.Clean(p.reposRoot), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for repos root: %w", err)
	}
	return filepath.Join(homeDir, ".ai-workflow", "repos"), nil
}

func normalizeProjectSlug(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return ""
	}
	normalized = reProjectSlug.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-_")
	return normalized
}

func isValidGitHubNamePart(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	return reGitHubNameCharacters.MatchString(trimmed)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func notifyProvisionProgress(progress func(step, message string), step, message string) {
	if progress == nil {
		return
	}
	progress(strings.TrimSpace(step), strings.TrimSpace(message))
}
