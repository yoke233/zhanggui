package outbox

import (
	"context"
	"errors"
	"strings"

	"zhanggui/internal/ports"
)

func appendEventTx(ctx context.Context, repo ports.OutboxRepository, issueID uint64, actor string, body string, createdAt string) error {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return errActorRequired
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return errBodyRequired
	}

	return repo.AppendEvent(ctx, ports.OutboxEventCreate{
		IssueID:   issueID,
		Actor:     actor,
		Body:      body,
		CreatedAt: createdAt,
	})
}

func setStateLabelTx(ctx context.Context, repo ports.OutboxRepository, issueID uint64, stateLabel string) error {
	stateLabel = strings.TrimSpace(stateLabel)
	if stateLabel == "" {
		return nil
	}

	normalized, err := normalizeStateLabel(stateLabel)
	if err != nil {
		return err
	}
	return repo.ReplaceStateLabel(ctx, issueID, normalized)
}

func hasIssueLabelTx(ctx context.Context, repo ports.OutboxRepository, issueID uint64, label string) (bool, error) {
	return repo.HasIssueLabel(ctx, issueID, label)
}

// unresolvedDependenciesTx resolves local dependencies and returns unresolved refs.
// Non-local refs are treated as external and skipped in local-first mode.
func unresolvedDependenciesTx(ctx context.Context, repo ports.OutboxRepository, dependencies []string) ([]string, error) {
	if len(dependencies) == 0 {
		return nil, nil
	}

	unresolved := make([]string, 0, len(dependencies))
	for _, dep := range dependencies {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}

		// Phase-1 local-first: only local#<id> can be verified automatically.
		if !strings.HasPrefix(dep, "local#") {
			continue
		}

		depIssueID, err := parseIssueRef(dep)
		if err != nil {
			unresolved = append(unresolved, dep)
			continue
		}

		depIssue, err := repo.GetIssue(ctx, depIssueID)
		if err != nil {
			if errors.Is(err, ports.ErrIssueNotFound) {
				unresolved = append(unresolved, dep)
				continue
			}
			return nil, err
		}
		if !depIssue.IsClosed {
			unresolved = append(unresolved, dep)
		}
	}
	return unresolved, nil
}
