package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/observability"
)

func cloneStringMapForEngine(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func RunEventData(traceID string, issueNumber int, op string, extra map[string]string) map[string]string {
	data := make(map[string]string, len(extra)+2)
	for k, v := range extra {
		data[k] = v
	}
	if issueNumber > 0 {
		data["issue_number"] = strconv.Itoa(issueNumber)
	}
	if strings.TrimSpace(op) != "" {
		data["op"] = strings.TrimSpace(op)
	}
	return observability.EventDataWithTrace(data, traceID)
}

func issueNumberFromRun(p *core.Run) int {
	if p == nil {
		return 0
	}
	if p.Config != nil {
		for _, key := range []string{"issue_number", "github_issue_number"} {
			if n := parseIssueNumberConfigValue(p.Config[key]); n > 0 {
				return n
			}
		}
	}
	if p.Artifacts != nil {
		for _, key := range []string{"issue_number", "github_issue_number"} {
			if n := parseIssueNumberConfigValue(p.Artifacts[key]); n > 0 {
				return n
			}
		}
	}
	return 0
}

func prNumberFromRunData(p *core.Run) int {
	if p == nil {
		return 0
	}
	if p.Config != nil {
		for _, key := range []string{"pr_number", "github_pr_number"} {
			if n := parseIssueNumberConfigValue(p.Config[key]); n > 0 {
				return n
			}
		}
	}
	if p.Artifacts != nil {
		for _, key := range []string{"pr_number", "github_pr_number"} {
			if n := parseIssueNumberConfigValue(p.Artifacts[key]); n > 0 {
				return n
			}
		}
	}
	return 0
}

func mergeConflictHintFromConfig(config map[string]any) string {
	if len(config) == 0 {
		return ""
	}
	raw, ok := config["merge_conflict_hint"]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func parseIssueNumberConfigValue(raw any) int {
	switch v := raw.(type) {
	case int:
		if v > 0 {
			return v
		}
	case int32:
		if v > 0 {
			return int(v)
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func RunTraceID(p *core.Run) string {
	if p == nil || p.Config == nil {
		return ""
	}
	traceID, _ := p.Config["trace_id"].(string)
	return strings.TrimSpace(traceID)
}
