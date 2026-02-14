package outbox

import (
	"context"
	"fmt"
	"strings"

	domainoutbox "zhanggui/internal/domain/outbox"
	"zhanggui/internal/ports"
)

func isTaskIssue(title string, labels []string) bool {
	return domainoutbox.IsTaskIssue(title, labels)
}

// hasTaskIssueSections checks the minimum task template anchors used in Phase-1.
func hasTaskIssueSections(body string) bool {
	return domainoutbox.HasTaskIssueSections(body)
}

func requiresWorkStartValidation(state string) bool {
	return domainoutbox.RequiresWorkStartValidation(state)
}

// ensureWorkPreconditionsTx enforces start/advance gates for workflow states.
func ensureWorkPreconditionsTx(ctx context.Context, repo ports.OutboxRepository, issue ports.OutboxIssue, targetState string) ([]string, error) {
	dependencies := domainoutbox.ParseDependsOnRefs(issue.Body)
	unresolved, err := unresolvedDependenciesTx(ctx, repo, dependencies)
	if err != nil {
		return nil, err
	}
	hasNeedsHuman, err := hasIssueLabelTx(ctx, repo, issue.IssueID, "needs-human")
	if err != nil {
		return nil, err
	}

	return domainoutbox.EvaluateWorkPreconditions(domainoutbox.WorkPreconditions{
		TargetState:            targetState,
		Assignee:               derefString(issue.Assignee),
		HasNeedsHuman:          hasNeedsHuman,
		UnresolvedDependencies: unresolved,
	})
}

func parseDependsOnRefs(body string) []string {
	return domainoutbox.ParseDependsOnRefs(body)
}

func appendBlockedEventTx(ctx context.Context, repo ports.OutboxRepository, issueID uint64, actor string, blockedBy []string, reason string, createdAt string) error {
	lines := []string{
		"Action: blocked",
		"Status: blocked",
		"",
		"Summary:",
		"- " + reason,
		"",
		"BlockedBy:",
	}

	if len(blockedBy) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, dep := range blockedBy {
			lines = append(lines, "- "+dep)
		}
	}

	return appendEventTx(ctx, repo, issueID, actor, strings.Join(lines, "\n"), createdAt)
}

// normalizeCommentBody upgrades free text into a parseable structured comment.
func normalizeCommentBody(issueRef string, actor string, state string, body string) string {
	if domainoutbox.IsStructuredCommentBody(body) {
		return body
	}

	status := "doing"
	if normalized, err := normalizeStateLabel(state); err == nil && normalized != "" {
		status = strings.TrimPrefix(normalized, "state:")
	}

	summary := domainoutbox.FirstNonEmptyLine(body)
	if summary == "" {
		summary = "update"
	}

	return fmt.Sprintf(
		"Role: %s\nRepo: main\nIssueRef: %s\nRunId: none\nSpecRef: none\nContractsRef: none\nAction: update\nStatus: %s\nReadUpTo: none\nTrigger: manual:%s\n\nSummary:\n- %s\n\nChanges:\n- PR: none\n- Commit: none\n\nTests:\n- Command: none\n- Result: n/a\n- Evidence: none\n\nBlockedBy:\n- none\n\nOpenQuestions:\n- none\n\nNext:\n- @integrator review update\n",
		actor,
		issueRef,
		status,
		nowUTCString(),
		summary,
	)
}

func hasCloseEvidenceTx(ctx context.Context, repo ports.OutboxRepository, issueID uint64) (bool, error) {
	events, err := repo.ListIssueEvents(ctx, issueID)
	if err != nil {
		return false, err
	}

	for _, event := range events {
		if hasCloseEvidenceFromBody(event.Body) {
			return true, nil
		}
	}
	return false, nil
}

func hasCloseEvidenceFromBody(body string) bool {
	return domainoutbox.HasCloseEvidenceFromBody(body)
}
