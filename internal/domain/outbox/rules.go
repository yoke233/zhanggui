package outbox

import "strings"

func IsTaskIssue(title string, labels []string) bool {
	for _, label := range labels {
		if strings.EqualFold(strings.TrimSpace(label), "kind:task") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(title), "[kind:task]")
}

func HasTaskIssueSections(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "## goal") && strings.Contains(lower, "## acceptance criteria")
}

func ParseDependsOnRefs(body string) []string {
	lines := strings.Split(body, "\n")
	inDependsOn := false
	out := make([]string, 0, 4)
	seen := make(map[string]struct{})

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lowerLine := strings.ToLower(line)

		if strings.HasPrefix(lowerLine, "- dependson:") || strings.HasPrefix(lowerLine, "dependson:") {
			inDependsOn = true
			idx := strings.Index(line, ":")
			if idx >= 0 {
				addDependencyTokens(line[idx+1:], seen, &out)
			}
			continue
		}

		if !inDependsOn {
			continue
		}

		if strings.HasPrefix(lowerLine, "- blockedby:") ||
			strings.HasPrefix(lowerLine, "blockedby:") ||
			strings.HasPrefix(line, "## ") {
			break
		}

		if strings.HasPrefix(line, "- ") {
			addDependencyTokens(strings.TrimSpace(strings.TrimPrefix(line, "- ")), seen, &out)
			continue
		}

		if strings.Contains(line, ":") && !strings.HasPrefix(strings.ToLower(line), "http") {
			break
		}

		addDependencyTokens(line, seen, &out)
	}

	return out
}

func HasCloseEvidenceFromBody(body string) bool {
	lines := strings.Split(body, "\n")
	inChanges := false
	inTests := false
	hasChanges := false
	hasTestsResult := false

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		lowerLine := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lowerLine, "changes:"):
			inChanges = true
			inTests = false
			continue
		case strings.HasPrefix(lowerLine, "tests:"):
			inChanges = false
			inTests = true
			continue
		case strings.HasSuffix(lowerLine, ":") && !strings.HasPrefix(lowerLine, "- "):
			inChanges = false
			inTests = false
			continue
		}

		if inChanges {
			if strings.HasPrefix(lowerLine, "- pr:") {
				value := strings.TrimSpace(line[len("- PR:"):])
				if !IsNoneLike(value) {
					hasChanges = true
				}
			}
			if strings.HasPrefix(lowerLine, "- commit:") {
				value := strings.TrimSpace(line[len("- Commit:"):])
				if !IsNoneLike(value) {
					hasChanges = true
				}
			}
		}

		if inTests && strings.HasPrefix(lowerLine, "- result:") {
			value := strings.TrimSpace(line[len("- Result:"):])
			if value != "" {
				hasTestsResult = true
			}
		}
	}

	return hasChanges && hasTestsResult
}

func IsStructuredCommentBody(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "issueref:") &&
		strings.Contains(lower, "changes:") &&
		strings.Contains(lower, "tests:") &&
		strings.Contains(lower, "next:")
}

func FirstNonEmptyLine(body string) string {
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		return line
	}
	return ""
}

func IsNoneLike(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	return trimmed == "" || trimmed == "none" || trimmed == "n/a"
}

func addDependencyTokens(raw string, seen map[string]struct{}, out *[]string) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	parts := strings.Split(raw, ",")
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		token = strings.TrimSuffix(token, ".")
		if IsNoneLike(token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		*out = append(*out, token)
	}
}
