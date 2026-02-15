package outbox

import (
	"context"
	"errors"
	"strings"

	"zhanggui/internal/errs"
)

// GetActiveRunID returns active run_id for an issue under a role lead.
func (s *Service) GetActiveRunID(ctx context.Context, role string, issueRef string) (string, bool, error) {
	if ctx == nil {
		return "", false, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", false, errs.Wrap(err, "check context")
	}
	if s.cache == nil {
		return "", false, nil
	}

	normalizedRole := strings.TrimSpace(role)
	if normalizedRole == "" {
		normalizedRole = "backend"
	}

	key := leadActiveRunKey(normalizedRole, strings.TrimSpace(issueRef))
	value, found, err := s.cache.Get(ctx, key)
	if err != nil {
		return "", false, err
	}
	value = strings.TrimSpace(value)
	if !found || value == "" {
		return "", false, nil
	}
	return value, true, nil
}
