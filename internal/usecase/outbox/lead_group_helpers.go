package outbox

import "strings"

func normalizeGroupMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "owner"
	}
	return normalized
}

func normalizeGroupWriteback(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "full"
	}
	return normalized
}

func isSubscriberMode(mode string) bool {
	return normalizeGroupMode(mode) == "subscriber"
}

func isCommentOnlyWriteback(mode string) bool {
	return normalizeGroupWriteback(mode) == "comment-only"
}

func shouldIndexVerdictLabels(role string, groupMode string, writebackMode string) bool {
	if strings.TrimSpace(role) != "integrator" {
		return false
	}
	if isSubscriberMode(groupMode) {
		return false
	}
	return !isCommentOnlyWriteback(writebackMode)
}

func shouldAddNeedsHumanForResultCode(code string) bool {
	switch strings.TrimSpace(code) {
	case "manual_intervention", "output_unparseable", "permission_denied":
		return true
	default:
		return false
	}
}

func applyVerdictMarker(role string, status string, resultCode string, summary string) string {
	normalizedSummary := strings.TrimSpace(summary)
	normalizedRole := strings.TrimSpace(role)
	normalizedStatus := strings.TrimSpace(status)
	normalizedCode := strings.TrimSpace(resultCode)

	switch normalizedRole {
	case "reviewer":
		marker := "review:approved"
		if normalizedStatus == "blocked" || normalizedCode == "review_changes_requested" {
			marker = "review:changes_requested"
		}
		if strings.HasPrefix(strings.ToLower(normalizedSummary), "review:") {
			return normalizedSummary
		}
		if normalizedSummary == "" {
			return marker
		}
		return marker + "; " + normalizedSummary
	case "qa":
		marker := "qa:pass"
		if normalizedStatus == "blocked" {
			marker = "qa:fail"
		}
		if strings.HasPrefix(strings.ToLower(normalizedSummary), "qa:") {
			return normalizedSummary
		}
		if normalizedSummary == "" {
			return marker
		}
		return marker + "; " + normalizedSummary
	default:
		return normalizedSummary
	}
}
