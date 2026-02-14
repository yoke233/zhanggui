package outbox

import (
	"context"
	"errors"
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

func appendBlockedEventTx(ctx context.Context, repo ports.OutboxRepository, issueID uint64, actor string, blockedBy []string, reason string, resultCode string, createdAt string) error {
	normalizedCode, err := normalizeOptionalResultCode(resultCode)
	if err != nil {
		return err
	}
	if normalizedCode == "none" {
		return errors.New("blocked event requires result_code")
	}

	body := renderStructuredComment(structuredComment{
		Role:          actor,
		Repo:          "main",
		IssueRef:      formatIssueRef(issueID),
		RunID:         "none",
		SpecRef:       "none",
		ContractsRef:  "none",
		Action:        "blocked",
		Status:        "blocked",
		ResultCode:    normalizedCode,
		ReadUpTo:      "none",
		Trigger:       "manual:" + createdAt,
		Summary:       []string{reason},
		ChangesPR:     "none",
		ChangesCommit: "none",
		TestsCommand:  "none",
		TestsResult:   "n/a",
		TestsEvidence: "none",
		BlockedBy:     blockedBy,
		OpenQuestions: []string{"none"},
		Next:          []string{"@lead resolve blocker"},
	})

	return appendEventTx(ctx, repo, issueID, actor, body, createdAt)
}

// normalizeCommentBody upgrades free text into a parseable structured comment.
func normalizeCommentBody(issueRef string, actor string, state string, body string) string {
	return normalizeCommentBodyWithAction(issueRef, actor, "update", state, body)
}

type structuredComment struct {
	Role          string
	Repo          string
	IssueRef      string
	RunID         string
	SpecRef       string
	ContractsRef  string
	Action        string
	Status        string
	ResultCode    string
	ReadUpTo      string
	Trigger       string
	Summary       []string
	ChangesPR     string
	ChangesCommit string
	TestsCommand  string
	TestsResult   string
	TestsEvidence string
	BlockedBy     []string
	OpenQuestions []string
	Next          []string
}

func renderStructuredComment(input structuredComment) string {
	ensureBulletList := func(lines []string, fallback string) []string {
		out := make([]string, 0, len(lines))
		for _, raw := range lines {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			out = append(out, line)
		}
		if len(out) == 0 {
			out = append(out, fallback)
		}
		return out
	}

	summary := ensureBulletList(input.Summary, "update")
	blockedBy := ensureBulletList(input.BlockedBy, "none")
	openQuestions := ensureBulletList(input.OpenQuestions, "none")
	next := ensureBulletList(input.Next, "@integrator review update")

	resultCode := strings.TrimSpace(input.ResultCode)
	if resultCode == "" {
		resultCode = "none"
	}

	return fmt.Sprintf(
		"Role: %s\nRepo: %s\nIssueRef: %s\nRunId: %s\nSpecRef: %s\nContractsRef: %s\nAction: %s\nStatus: %s\nResultCode: %s\nReadUpTo: %s\nTrigger: %s\n\nSummary:\n%s\n\nChanges:\n- PR: %s\n- Commit: %s\n\nTests:\n- Command: %s\n- Result: %s\n- Evidence: %s\n\nBlockedBy:\n%s\n\nOpenQuestions:\n%s\n\nNext:\n%s\n",
		strings.TrimSpace(input.Role),
		strings.TrimSpace(input.Repo),
		strings.TrimSpace(input.IssueRef),
		strings.TrimSpace(input.RunID),
		strings.TrimSpace(input.SpecRef),
		strings.TrimSpace(input.ContractsRef),
		strings.TrimSpace(input.Action),
		strings.TrimSpace(input.Status),
		resultCode,
		strings.TrimSpace(input.ReadUpTo),
		strings.TrimSpace(input.Trigger),
		renderBulletLines(summary),
		strings.TrimSpace(input.ChangesPR),
		strings.TrimSpace(input.ChangesCommit),
		strings.TrimSpace(input.TestsCommand),
		strings.TrimSpace(input.TestsResult),
		strings.TrimSpace(input.TestsEvidence),
		renderBulletLines(blockedBy),
		renderBulletLines(openQuestions),
		renderBulletLines(next),
	)
}

func renderBulletLines(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func normalizeCommentBodyWithAction(issueRef string, actor string, action string, state string, body string) string {
	if domainoutbox.IsStructuredCommentBody(body) {
		return body
	}

	status := "doing"
	if normalized, err := normalizeStateLabel(state); err == nil && normalized != "" {
		status = strings.TrimPrefix(normalized, "state:")
	}

	summary := domainoutbox.FirstNonEmptyLine(body)
	if summary == "" {
		switch strings.TrimSpace(action) {
		case "claim":
			summary = "claim"
		case "blocked":
			summary = "blocked"
		case "done":
			summary = "done"
		default:
			summary = "update"
		}
	}

	return renderStructuredComment(structuredComment{
		Role:          actor,
		Repo:          "main",
		IssueRef:      issueRef,
		RunID:         "none",
		SpecRef:       "none",
		ContractsRef:  "none",
		Action:        action,
		Status:        status,
		ResultCode:    "none",
		ReadUpTo:      "none",
		Trigger:       "manual:" + nowUTCString(),
		Summary:       []string{summary},
		ChangesPR:     "none",
		ChangesCommit: "none",
		TestsCommand:  "none",
		TestsResult:   "n/a",
		TestsEvidence: "none",
		BlockedBy:     []string{"none"},
		OpenQuestions: []string{"none"},
		Next:          []string{"@integrator review update"},
	})
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

func normalizeOptionalResultCode(code string) (string, error) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" || strings.EqualFold(trimmed, "none") {
		return "none", nil
	}
	if err := domainoutbox.ValidateResultCode(trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

func validateOptionalResultCodeInCommentBody(body string) error {
	if strings.TrimSpace(body) == "" {
		return nil
	}

	lines := strings.Split(body, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			break
		}

		lower := strings.ToLower(line)
		if !strings.HasPrefix(lower, "resultcode:") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid ResultCode header line: %q", raw)
		}
		value := strings.TrimSpace(parts[1])
		_, err := normalizeOptionalResultCode(value)
		return err
	}

	return nil
}
