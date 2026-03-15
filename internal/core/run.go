package core

import "time"

// RunStatus represents the lifecycle state of a Run.
type RunStatus string

const (
	RunCreated   RunStatus = "created"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

// ErrorKind classifies the nature of a run failure.
type ErrorKind string

const (
	ErrKindTransient ErrorKind = "transient" // retry is worthwhile
	ErrKindPermanent ErrorKind = "permanent" // no point retrying
	ErrKindNeedHelp  ErrorKind = "need_help" // requires human/lead intervention
)

// Run is a single attempt to execute an Action. An Action may have multiple Runs (retries).
type Run struct {
	ID               int64          `json:"id"`
	ActionID         int64          `json:"action_id"`
	WorkItemID       int64          `json:"work_item_id"`
	Status           RunStatus      `json:"status"`
	AgentID          string         `json:"agent_id,omitempty"`
	AgentContextID   *int64         `json:"agent_context_id,omitempty"`
	BriefingSnapshot string         `json:"briefing_snapshot,omitempty"` // briefing at run time
	Input            map[string]any `json:"input,omitempty"`
	Output           map[string]any `json:"output,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	ErrorKind        ErrorKind      `json:"error_kind,omitempty"`
	Attempt          int            `json:"attempt"`
	StartedAt        *time.Time     `json:"started_at,omitempty"`
	FinishedAt       *time.Time     `json:"finished_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	// Inline result fields (formerly in Deliverable/artifacts table).
	ResultMarkdown string         `json:"result_markdown,omitempty"`
	ResultMetadata map[string]any `json:"result_metadata,omitempty"`
	// ResultAssets is kept for compatibility with legacy SQLite result_assets data.
	ResultAssets []Asset `json:"result_assets,omitempty"`
}

// HasResult returns true if this Run produced a non-empty result.
func (r *Run) HasResult() bool {
	return r.ResultMarkdown != ""
}
