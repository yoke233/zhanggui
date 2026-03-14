package core

import (
	"encoding/json"
	"time"
)

// ToolCallAudit stores the structured summary for one tool call during an execution.
type ToolCallAudit struct {
	ID             int64      `json:"id"`
	WorkItemID     int64      `json:"work_item_id"`
	ActionID       int64      `json:"action_id"`
	RunID          int64      `json:"run_id"`
	SessionID      string     `json:"session_id,omitempty"`
	ToolCallID     string     `json:"tool_call_id"`
	ToolName       string     `json:"tool_name,omitempty"`
	Status         string     `json:"status"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	DurationMs     int64      `json:"duration_ms,omitempty"`
	ExitCode       *int       `json:"exit_code,omitempty"`
	InputDigest    string     `json:"input_digest,omitempty"`
	OutputDigest   string     `json:"output_digest,omitempty"`
	StdoutDigest   string     `json:"stdout_digest,omitempty"`
	StderrDigest   string     `json:"stderr_digest,omitempty"`
	InputPreview   string     `json:"input_preview,omitempty"`
	OutputPreview  string     `json:"output_preview,omitempty"`
	StdoutPreview  string     `json:"stdout_preview,omitempty"`
	StderrPreview  string     `json:"stderr_preview,omitempty"`
	RedactionLevel string     `json:"redaction_level,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ToolCallAuditFilter constrains tool call audit queries.
type ToolCallAuditFilter struct {
	RunID      *int64
	ToolCallID string
}

// ToEvent converts a ToolCallAudit into an Event with category="tool_audit".
func (a *ToolCallAudit) ToEvent() *Event {
	data := map[string]any{
		"session_id":      a.SessionID,
		"tool_call_id":    a.ToolCallID,
		"tool_name":       a.ToolName,
		"status":          a.Status,
		"duration_ms":     a.DurationMs,
		"input_digest":    a.InputDigest,
		"output_digest":   a.OutputDigest,
		"stdout_digest":   a.StdoutDigest,
		"stderr_digest":   a.StderrDigest,
		"input_preview":   a.InputPreview,
		"output_preview":  a.OutputPreview,
		"stdout_preview":  a.StdoutPreview,
		"stderr_preview":  a.StderrPreview,
		"redaction_level": a.RedactionLevel,
	}
	if a.StartedAt != nil {
		data["started_at"] = a.StartedAt.Format(time.RFC3339Nano)
	}
	if a.FinishedAt != nil {
		data["finished_at"] = a.FinishedAt.Format(time.RFC3339Nano)
	}
	if a.ExitCode != nil {
		data["exit_code"] = *a.ExitCode
	}

	ts := a.CreatedAt
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	return &Event{
		ID:         a.ID,
		Type:       "tool_call_audit",
		Category:   EventCategoryToolAudit,
		WorkItemID: a.WorkItemID,
		ActionID:   a.ActionID,
		RunID:      a.RunID,
		Data:       data,
		Timestamp:  ts,
	}
}

// ToolCallAuditFromEvent deserializes a ToolCallAudit from an Event with category="tool_audit".
func ToolCallAuditFromEvent(e *Event) *ToolCallAudit {
	if e == nil {
		return nil
	}
	a := &ToolCallAudit{
		ID:         e.ID,
		WorkItemID: e.WorkItemID,
		ActionID:   e.ActionID,
		RunID:      e.RunID,
		CreatedAt:  e.Timestamp,
	}

	if v, ok := e.Data["session_id"].(string); ok {
		a.SessionID = v
	}
	if v, ok := e.Data["tool_call_id"].(string); ok {
		a.ToolCallID = v
	}
	if v, ok := e.Data["tool_name"].(string); ok {
		a.ToolName = v
	}
	if v, ok := e.Data["status"].(string); ok {
		a.Status = v
	}
	if v, ok := e.Data["duration_ms"].(float64); ok {
		a.DurationMs = int64(v)
	} else if v, ok := e.Data["duration_ms"].(json.Number); ok {
		if i, err := v.Int64(); err == nil {
			a.DurationMs = i
		}
	}
	if v, ok := e.Data["exit_code"].(float64); ok {
		ec := int(v)
		a.ExitCode = &ec
	} else if v, ok := e.Data["exit_code"].(json.Number); ok {
		if i, err := v.Int64(); err == nil {
			ec := int(i)
			a.ExitCode = &ec
		}
	}
	if v, ok := e.Data["input_digest"].(string); ok {
		a.InputDigest = v
	}
	if v, ok := e.Data["output_digest"].(string); ok {
		a.OutputDigest = v
	}
	if v, ok := e.Data["stdout_digest"].(string); ok {
		a.StdoutDigest = v
	}
	if v, ok := e.Data["stderr_digest"].(string); ok {
		a.StderrDigest = v
	}
	if v, ok := e.Data["input_preview"].(string); ok {
		a.InputPreview = v
	}
	if v, ok := e.Data["output_preview"].(string); ok {
		a.OutputPreview = v
	}
	if v, ok := e.Data["stdout_preview"].(string); ok {
		a.StdoutPreview = v
	}
	if v, ok := e.Data["stderr_preview"].(string); ok {
		a.StderrPreview = v
	}
	if v, ok := e.Data["redaction_level"].(string); ok {
		a.RedactionLevel = v
	}
	if v, ok := e.Data["started_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			a.StartedAt = &t
		}
	}
	if v, ok := e.Data["finished_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			a.FinishedAt = &t
		}
	}

	return a
}
