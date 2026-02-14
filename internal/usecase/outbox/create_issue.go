package outbox

import (
	"context"
	"errors"
	"strings"

	"zhanggui/internal/errs"
	"zhanggui/internal/ports"
)

// CreateIssue creates a new issue, validates task template hard requirements, and sets initial cache status.
func (s *Service) CreateIssue(ctx context.Context, input CreateIssueInput) (string, error) {
	if ctx == nil {
		return "", errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", errs.Wrap(err, "check context")
	}
	if s.repo == nil {
		return "", errors.New("outbox repository is required")
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		return "", errors.New("title is required")
	}

	body := strings.TrimSpace(input.Body)
	if body == "" {
		return "", errors.New("body is required")
	}

	now := nowUTCString()
	labels := normalizeLabels(input.Labels)
	if isTaskIssue(title, labels) && !hasTaskIssueSections(body) {
		return "", errTaskIssueBody
	}

	if s.uow == nil {
		return "", errors.New("outbox unit of work is required")
	}

	var created ports.OutboxIssue
	if err := s.uow.WithTx(ctx, func(txCtx context.Context) error {
		var err error
		created, err = s.repo.CreateIssue(txCtx, ports.OutboxIssue{
			Title:     title,
			Body:      body,
			IsClosed:  false,
			CreatedAt: now,
			UpdatedAt: now,
		}, labels)
		return err
	}); err != nil {
		return "", err
	}

	issueRef := formatIssueRef(created.IssueID)
	s.setCacheBestEffort(ctx, cacheIssueStatusKey(issueRef), "open")
	return issueRef, nil
}
