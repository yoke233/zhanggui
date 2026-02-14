package outbox

import (
	"context"
	"errors"
	"fmt"

	"zhanggui/internal/errs"
	"zhanggui/internal/ports"
)

// ListIssues returns issue summaries with labels for queue views.
func (s *Service) ListIssues(ctx context.Context, includeClosed bool, assignee string) ([]IssueListItem, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(err, "check context")
	}
	if s.repo == nil {
		return nil, errors.New("outbox repository is required")
	}

	issues, err := s.repo.ListIssues(ctx, ports.OutboxIssueFilter{
		IncludeClosed: includeClosed,
		Assignee:      assignee,
	})
	if err != nil {
		return nil, err
	}

	items := make([]IssueListItem, 0, len(issues))
	for _, issue := range issues {
		labels, err := s.repo.ListIssueLabels(ctx, issue.IssueID)
		if err != nil {
			return nil, err
		}

		items = append(items, IssueListItem{
			IssueRef:  formatIssueRef(issue.IssueID),
			Title:     issue.Title,
			Assignee:  derefString(issue.Assignee),
			IsClosed:  issue.IsClosed,
			CreatedAt: issue.CreatedAt,
			UpdatedAt: issue.UpdatedAt,
			Labels:    labels,
		})
	}

	return items, nil
}

// GetIssue returns issue detail with timeline events.
func (s *Service) GetIssue(ctx context.Context, issueRef string) (IssueDetail, error) {
	if ctx == nil {
		return IssueDetail{}, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return IssueDetail{}, errs.Wrap(err, "check context")
	}
	if s.repo == nil {
		return IssueDetail{}, errors.New("outbox repository is required")
	}

	issueID, err := parseIssueRef(issueRef)
	if err != nil {
		return IssueDetail{}, err
	}

	issue, err := s.repo.GetIssue(ctx, issueID)
	if err != nil {
		if errors.Is(err, ports.ErrIssueNotFound) {
			return IssueDetail{}, fmt.Errorf("issue %s not found", issueRef)
		}
		return IssueDetail{}, err
	}

	labels, err := s.repo.ListIssueLabels(ctx, issueID)
	if err != nil {
		return IssueDetail{}, err
	}

	events, err := s.repo.ListIssueEvents(ctx, issueID)
	if err != nil {
		return IssueDetail{}, err
	}

	outEvents := make([]EventItem, 0, len(events))
	for _, event := range events {
		outEvents = append(outEvents, EventItem{
			EventID:   event.EventID,
			Actor:     event.Actor,
			CreatedAt: event.CreatedAt,
			Body:      event.Body,
		})
	}

	return IssueDetail{
		IssueRef:  formatIssueRef(issue.IssueID),
		Title:     issue.Title,
		Body:      issue.Body,
		Assignee:  derefString(issue.Assignee),
		IsClosed:  issue.IsClosed,
		CreatedAt: issue.CreatedAt,
		UpdatedAt: issue.UpdatedAt,
		ClosedAt:  derefString(issue.ClosedAt),
		Labels:    labels,
		Events:    outEvents,
	}, nil
}
