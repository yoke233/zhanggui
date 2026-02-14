package outbox

import (
	"strings"
	"time"

	domainoutbox "zhanggui/internal/domain/outbox"
)

func parseIssueRef(issueRef string) (uint64, error) {
	return domainoutbox.ParseIssueRef(issueRef)
}

func formatIssueRef(issueID uint64) string {
	return domainoutbox.FormatIssueRef(issueID)
}

func nowUTCString() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func normalizeLabels(in []string) []string {
	if len(in) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		label := strings.TrimSpace(raw)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func normalizeStateLabel(state string) (string, error) {
	return domainoutbox.NormalizeStateLabel(state)
}

func derefString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func cacheIssueStatusKey(issueRef string) string {
	return "issue_status:" + issueRef
}

func cacheIssueAssigneeKey(issueRef string) string {
	return "issue_assignee:" + issueRef
}
