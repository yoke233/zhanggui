package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zhanggui/internal/ports"
)

// LeadRunIssueOnce runs lead orchestration against one issue.
// It is used by operator console actions such as spawn/switch worker.
func (s *Service) LeadRunIssueOnce(ctx context.Context, input LeadRunIssueInput) (LeadRunIssueResult, error) {
	if ctx == nil {
		return LeadRunIssueResult{}, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return LeadRunIssueResult{}, err
	}
	if s.repo == nil {
		return LeadRunIssueResult{}, errors.New("outbox repository is required")
	}
	if s.cache == nil {
		return LeadRunIssueResult{}, errors.New("cache is required")
	}

	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = "backend"
	}
	assignee := strings.TrimSpace(input.Assignee)
	if assignee == "" {
		assignee = "lead-" + role
	}

	profile, err := loadWorkflowProfile(input.WorkflowFile)
	if err != nil {
		return LeadRunIssueResult{}, err
	}
	if !isRoleEnabled(profile, role) {
		return LeadRunIssueResult{}, fmt.Errorf("role %s is not enabled in workflow", role)
	}
	if strings.TrimSpace(profile.Outbox.Backend) != "sqlite" {
		return LeadRunIssueResult{}, fmt.Errorf("lead only supports sqlite backend, got %q", profile.Outbox.Backend)
	}
	if _, ok := findGroupByRole(profile, role); !ok {
		return LeadRunIssueResult{}, fmt.Errorf("group config is required for role %s", role)
	}

	issueID, err := parseIssueRef(input.IssueRef)
	if err != nil {
		return LeadRunIssueResult{}, err
	}
	issue, err := s.repo.GetIssue(ctx, issueID)
	if err != nil {
		if errors.Is(err, ports.ErrIssueNotFound) {
			return LeadRunIssueResult{}, fmt.Errorf("issue %s not found", input.IssueRef)
		}
		return LeadRunIssueResult{}, err
	}

	outcome, err := s.processLeadIssue(ctx, leadIssueProcessInput{
		Role:            role,
		Assignee:        assignee,
		Issue:           issue,
		WorkflowFile:    input.WorkflowFile,
		ConfigFile:      input.ConfigFile,
		ExecutablePath:  input.ExecutablePath,
		Profile:         profile,
		CursorAfter:     0,
		AllowSpawn:      true,
		IgnoreStateSkip: input.ForceSpawn,
	})
	if err != nil {
		return LeadRunIssueResult{}, err
	}

	return LeadRunIssueResult{
		Processed: outcome.processed,
		Blocked:   outcome.blocked,
		Spawned:   outcome.spawned,
	}, nil
}
