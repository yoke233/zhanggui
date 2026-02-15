package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zhanggui/internal/errs"
	"zhanggui/internal/ports"
)

// UnclaimIssue clears assignee and moves issue back to state:todo.
func (s *Service) UnclaimIssue(ctx context.Context, input UnclaimIssueInput) error {
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

	actor := strings.TrimSpace(input.Actor)
	if actor == "" {
		return errActorRequired
	}
	comment := strings.TrimSpace(input.Comment)
	if comment == "" {
		comment = "Action: unclaim\nStatus: todo"
	}

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

		if err := s.repo.SetIssueAssignee(txCtx, issueID, "", now); err != nil {
			return err
		}
		if err := setStateLabelTx(txCtx, s.repo, issueID, "state:todo"); err != nil {
			return err
		}

		normalizedBody := normalizeCommentBodyWithAction(input.IssueRef, actor, "unclaim", "todo", comment)
		if err := validateOptionalResultCodeInCommentBody(normalizedBody); err != nil {
			return err
		}
		if err := appendEventTx(txCtx, s.repo, issueID, actor, normalizedBody, now); err != nil {
			return err
		}
		if err := s.repo.UpdateIssueUpdatedAt(txCtx, issueID, now); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	s.setCacheBestEffort(ctx, cacheIssueStatusKey(input.IssueRef), "state:todo")
	s.setCacheBestEffort(ctx, cacheIssueAssigneeKey(input.IssueRef), "")
	return nil
}
