package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zhanggui/internal/errs"
)

type CleanupWorkdirInput struct {
	WorkflowFile string
	Role         string
	IssueRef     string
	RunID        string
}

type CleanupWorkdirResult struct {
	Workdir string
}

// CleanupWorkdir removes a previously prepared workdir (git worktree) by (role, issue_ref, run_id).
// It forces cleanup execution regardless of workflow workdir.cleanup policy (manual/immediate).
func (s *Service) CleanupWorkdir(ctx context.Context, input CleanupWorkdirInput) (CleanupWorkdirResult, error) {
	if ctx == nil {
		return CleanupWorkdirResult{}, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return CleanupWorkdirResult{}, errs.Wrap(err, "check context")
	}

	workflowFile := strings.TrimSpace(input.WorkflowFile)
	if workflowFile == "" {
		workflowFile = "workflow.toml"
	}

	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = "backend"
	}

	issueRef := strings.TrimSpace(input.IssueRef)
	if issueRef == "" {
		return CleanupWorkdirResult{}, errors.New("issue ref is required")
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return CleanupWorkdirResult{}, errors.New("run id is required")
	}

	profile, err := loadWorkflowProfile(workflowFile)
	if err != nil {
		return CleanupWorkdirResult{}, err
	}
	workdirCfg := profile.Workdir
	if !shouldUseWorkdir(workdirCfg, role) {
		return CleanupWorkdirResult{}, fmt.Errorf("workdir is not enabled for role %s", role)
	}

	repoDir, err := resolveRoleRepoDir(profile, workflowFile, role)
	if err != nil {
		return CleanupWorkdirResult{}, err
	}

	// Force actual cleanup execution.
	workdirCfg.Cleanup = "immediate"

	mgr, err := newGitWorktreeManager(workdirCfg, workflowFile, repoDir)
	if err != nil {
		return CleanupWorkdirResult{}, err
	}

	workdirPath, err := mgr.workdirPath(role, issueRef, runID)
	if err != nil {
		return CleanupWorkdirResult{}, err
	}

	if err := mgr.Cleanup(ctx, role, issueRef, runID, workdirPath); err != nil {
		return CleanupWorkdirResult{}, errs.Wrap(err, "cleanup workdir")
	}
	return CleanupWorkdirResult{Workdir: workdirPath}, nil
}
