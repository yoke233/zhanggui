package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zhanggui/internal/errs"
	"zhanggui/internal/ports"
)

// ClaimIssue sets assignee as the claim source of truth and moves the issue to state:doing.
func (s *Service) ClaimIssue(ctx context.Context, input ClaimIssueInput) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "check context")
	}
	if s.repo == nil {
		return errors.New("outbox repository is required")
	}
	if s.uow == nil {
		return errors.New("outbox unit of work is required")
	}

	issueID, err := parseIssueRef(input.IssueRef)
	if err != nil {
		return err
	}

	assignee := strings.TrimSpace(input.Assignee)
	if assignee == "" {
		return errors.New("assignee is required")
	}

	actor := strings.TrimSpace(input.Actor)
	if actor == "" {
		actor = assignee
	}

	comment := strings.TrimSpace(input.Comment)
	now := nowUTCString()

	if err := s.uow.WithTx(ctx, func(txCtx context.Context) error {
		issue, err := s.repo.GetIssue(txCtx, issueID)
		if err != nil {
			if errors.Is(err, ports.ErrIssueNotFound) {
				return fmt.Errorf("issue %s not found", input.IssueRef)
			}
			return err
		}
		if issue.IsClosed {
			return fmt.Errorf("issue %s is closed", input.IssueRef)
		}

		if err := s.repo.SetIssueAssignee(txCtx, issueID, assignee, now); err != nil {
			return err
		}

		if err := setStateLabelTx(txCtx, s.repo, issueID, "state:doing"); err != nil {
			return err
		}

		if comment != "" {
			if err := appendEventTx(txCtx, s.repo, issueID, actor, comment, now); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	s.setCacheBestEffort(ctx, cacheIssueStatusKey(input.IssueRef), "state:doing")
	s.setCacheBestEffort(ctx, cacheIssueAssigneeKey(input.IssueRef), assignee)
	return nil
}
