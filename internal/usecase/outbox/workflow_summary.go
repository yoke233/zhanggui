package outbox

import (
	"context"
	"errors"
	"strings"

	"zhanggui/internal/errs"
)

func (s *Service) GetWorkflowSummary(ctx context.Context, workflowFile string) (WorkflowSummary, error) {
	if ctx == nil {
		return WorkflowSummary{}, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return WorkflowSummary{}, errs.Wrap(err, "check context")
	}

	profile, err := loadWorkflowProfile(workflowFile)
	if err != nil {
		return WorkflowSummary{}, err
	}

	seen := make(map[string]struct{}, len(profile.Roles.Enabled))
	enabledRoles := make([]string, 0, len(profile.Roles.Enabled))
	for _, role := range profile.Roles.Enabled {
		normalized := strings.TrimSpace(role)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		enabledRoles = append(enabledRoles, normalized)
	}

	return WorkflowSummary{
		EnabledRoles: enabledRoles,
	}, nil
}
