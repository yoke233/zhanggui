package agui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func asMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func validateID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("empty")
	}
	if !idRe.MatchString(id) {
		return fmt.Errorf("invalid: %q", id)
	}
	return nil
}

func stringifyContent(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func normalizeRunInput(raw map[string]any, req RunRequest) map[string]any {
	out := map[string]any{}
	for k, v := range raw {
		out[k] = v
	}

	if strings.TrimSpace(req.ThreadID) != "" {
		out["threadId"] = req.ThreadID
	}
	if strings.TrimSpace(req.RunID) != "" {
		out["runId"] = req.RunID
	}
	if strings.TrimSpace(req.Workflow) != "" {
		out["workflow"] = req.Workflow
	}

	if req.Resume != nil && strings.TrimSpace(req.Resume.InterruptID) != "" {
		resume := map[string]any{}
		for k, v := range req.Resume.Raw {
			resume[k] = v
		}
		resume["interruptId"] = req.Resume.InterruptID
		resume["payload"] = req.Resume.Payload
		out["resume"] = resume
	}

	return out
}
