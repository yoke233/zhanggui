package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zhanggui/internal/errs"
	"zhanggui/internal/ports"
)

// CloseIssue closes an issue only when preconditions and close evidence are satisfied.
func (s *Service) CloseIssue(ctx context.Context, input CloseIssueInput) error {
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

	comment := strings.TrimSpace(input.Comment)
	actor := strings.TrimSpace(input.Actor)
	if comment != "" && actor == "" {
		return errActorRequired
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
			return nil
		}

		blockedBy, condErr := ensureWorkPreconditionsTx(txCtx, s.repo, issue, "state:done")
		if condErr != nil {
			if errors.Is(condErr, errNeedsHuman) || errors.Is(condErr, errDependsUnresolved) {
				if err := setStateLabelTx(txCtx, s.repo, issueID, "state:blocked"); err != nil {
					return err
				}
				if actor != "" {
					if err := appendBlockedEventTx(txCtx, s.repo, issueID, actor, blockedBy, condErr.Error(), now); err != nil {
						return err
					}
				}
				// Persist blocked state/event for auditability, then reject the operation outside tx.
				blockedErr = condErr
				return nil
			}
			return condErr
		}

		hasEvidence, err := hasCloseEvidenceTx(txCtx, s.repo, issueID)
		if err != nil {
			return err
		}
		if !hasEvidence && !hasCloseEvidenceFromBody(comment) {
			return errCloseEvidence
		}

		if err := s.repo.MarkIssueClosed(txCtx, issueID, now); err != nil {
			return err
		}
		if err := setStateLabelTx(txCtx, s.repo, issueID, "state:done"); err != nil {
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
	if blockedErr != nil {
		return blockedErr
	}

	s.setCacheBestEffort(ctx, cacheIssueStatusKey(input.IssueRef), "closed")
	return nil
}
