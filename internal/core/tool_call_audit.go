package core

import (
	"context"
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

// ToolCallAuditStore persists structured tool call audit summaries.
type ToolCallAuditStore interface {
	CreateToolCallAudit(ctx context.Context, audit *ToolCallAudit) (int64, error)
	GetToolCallAudit(ctx context.Context, id int64) (*ToolCallAudit, error)
	GetToolCallAuditByToolCallID(ctx context.Context, runID int64, toolCallID string) (*ToolCallAudit, error)
	ListToolCallAuditsByRun(ctx context.Context, runID int64) ([]*ToolCallAudit, error)
	UpdateToolCallAudit(ctx context.Context, audit *ToolCallAudit) error
}
