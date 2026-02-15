package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zhanggui/internal/errs"
	"zhanggui/internal/ports"
)

type AddIssueLabelsInput struct {
	IssueRef string
	Actor    string
	Labels   []string
}

type RemoveIssueLabelsInput struct {
	IssueRef string
	Actor    string
	Labels   []string
}

func (s *Service) AddIssueLabels(ctx context.Context, input AddIssueLabelsInput) error {
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

	labels := normalizeLabels(input.Labels)
	if len(labels) == 0 {
		return errors.New("labels are required")
	}

	stateLabel := ""
	otherLabels := make([]string, 0, len(labels))
	for _, label := range labels {
		if strings.HasPrefix(label, "state:") {
			if stateLabel != "" && stateLabel != label {
				return errors.New("multiple state labels are not allowed")
			}
			stateLabel = label
			continue
		}
		otherLabels = append(otherLabels, label)
	}

	now := nowUTCString()
	var resolvedStatus string
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

		for _, label := range otherLabels {
			if err := s.repo.AddIssueLabel(txCtx, issueID, label); err != nil {
				return err
			}
		}
		if err := setStateLabelTx(txCtx, s.repo, issueID, stateLabel); err != nil {
			return err
		}

		if err := s.repo.UpdateIssueUpdatedAt(txCtx, issueID, now); err != nil {
			return err
		}

		updatedLabels, err := s.repo.ListIssueLabels(txCtx, issueID)
		if err != nil {
			return err
		}
		status := strings.TrimPrefix(currentStateLabel(updatedLabels), "state:")
		if status == "" {
			status = "doing"
		}
		resolvedStatus = status

		summary := "labels added: " + strings.Join(labels, ", ")
		body := buildStructuredComment(StructuredCommentInput{
			Role:         actor,
			IssueRef:     input.IssueRef,
			RunID:        "none",
			Action:       "label-add",
			Status:       status,
			ResultCode:   "none",
			ReadUpTo:     "none",
			Trigger:      "manual:label-add:" + now,
			Summary:      summary,
			BlockedBy:    []string{"none"},
			OpenQuestion: "none",
			Next:         "@lead continue",
		})

		if err := appendEventTx(txCtx, s.repo, issueID, actor, body, now); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if strings.HasPrefix(stateLabel, "state:") && resolvedStatus != "" {
		s.setCacheBestEffort(ctx, cacheIssueStatusKey(input.IssueRef), "state:"+resolvedStatus)
	}
	return nil
}

func (s *Service) RemoveIssueLabels(ctx context.Context, input RemoveIssueLabelsInput) error {
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

	labels := normalizeLabels(input.Labels)
	if len(labels) == 0 {
		return errors.New("labels are required")
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

		for _, label := range labels {
			if err := s.repo.RemoveIssueLabel(txCtx, issueID, label); err != nil {
				return err
			}
		}

		if err := s.repo.UpdateIssueUpdatedAt(txCtx, issueID, now); err != nil {
			return err
		}

		updatedLabels, err := s.repo.ListIssueLabels(txCtx, issueID)
		if err != nil {
			return err
		}
		status := strings.TrimPrefix(currentStateLabel(updatedLabels), "state:")
		if status == "" {
			status = "doing"
		}

		summary := "labels removed: " + strings.Join(labels, ", ")
		body := buildStructuredComment(StructuredCommentInput{
			Role:         actor,
			IssueRef:     input.IssueRef,
			RunID:        "none",
			Action:       "label-remove",
			Status:       status,
			ResultCode:   "none",
			ReadUpTo:     "none",
			Trigger:      "manual:label-remove:" + now,
			Summary:      summary,
			BlockedBy:    []string{"none"},
			OpenQuestion: "none",
			Next:         "@lead continue",
		})

		if err := appendEventTx(txCtx, s.repo, issueID, actor, body, now); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}
