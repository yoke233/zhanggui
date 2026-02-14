package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zhanggui/internal/errs"
	"zhanggui/internal/ports"
)

// CommentIssue appends a structured timeline event and optionally updates workflow state.
func (s *Service) CommentIssue(ctx context.Context, input CommentIssueInput) error {
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

	body := strings.TrimSpace(input.Body)
	if body == "" {
		return errBodyRequired
	}

	state, err := normalizeStateLabel(input.State)
	if err != nil {
		return err
	}

	now := nowUTCString()
	var blockedErr error
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

		if requiresWorkStartValidation(state) {
			blockedBy, condErr := ensureWorkPreconditionsTx(txCtx, s.repo, issue, state)
			if condErr != nil {
				if errors.Is(condErr, errNeedsHuman) || errors.Is(condErr, errDependsUnresolved) {
					if err := setStateLabelTx(txCtx, s.repo, issueID, "state:blocked"); err != nil {
						return err
					}
					if err := appendBlockedEventTx(txCtx, s.repo, issueID, actor, blockedBy, condErr.Error(), now); err != nil {
						return err
					}
					// Persist blocked state/event for auditability, then reject the operation outside tx.
					blockedErr = condErr
					return nil
				}
				return condErr
			}
		}

		normalizedBody := normalizeCommentBody(input.IssueRef, actor, state, body)
		if err := appendEventTx(txCtx, s.repo, issueID, actor, normalizedBody, now); err != nil {
			return err
		}

		if err := s.repo.UpdateIssueUpdatedAt(txCtx, issueID, now); err != nil {
			return err
		}

		if state != "" {
			if err := setStateLabelTx(txCtx, s.repo, issueID, state); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}
	if blockedErr != nil {
		return blockedErr
	}

	if state != "" {
		s.setCacheBestEffort(ctx, cacheIssueStatusKey(input.IssueRef), state)
	}
	return nil
}
