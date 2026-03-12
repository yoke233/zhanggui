package core

import "time"

// ExecutionStatus represents the lifecycle state of an Execution.
type ExecutionStatus string

const (
	ExecCreated   ExecutionStatus = "created"
	ExecRunning   ExecutionStatus = "running"
	ExecSucceeded ExecutionStatus = "succeeded"
	ExecFailed    ExecutionStatus = "failed"
	ExecCancelled ExecutionStatus = "cancelled"
)

// ErrorKind classifies the nature of an execution failure.
type ErrorKind string

const (
	ErrKindTransient ErrorKind = "transient" // retry is worthwhile
	ErrKindPermanent ErrorKind = "permanent" // no point retrying
	ErrKindNeedHelp  ErrorKind = "need_help" // requires human/lead intervention
)

// Execution is a single attempt to run a Step. A Step may have multiple Executions (retries).
type Execution struct {
	ID               int64           `json:"id"`
	StepID           int64           `json:"step_id"`
	IssueID          int64           `json:"issue_id"`
	Status           ExecutionStatus `json:"status"`
	AgentID          string          `json:"agent_id,omitempty"`
	AgentContextID   *int64          `json:"agent_context_id,omitempty"`
	BriefingSnapshot string          `json:"briefing_snapshot,omitempty"` // briefing at execution time
	ArtifactID       *int64          `json:"artifact_id,omitempty"`       // points to Artifact
	Input            map[string]any  `json:"input,omitempty"`
	Output           map[string]any  `json:"output,omitempty"`
	ErrorMessage     string          `json:"error_message,omitempty"`
	ErrorKind        ErrorKind       `json:"error_kind,omitempty"`
	Attempt          int             `json:"attempt"`
	StartedAt        *time.Time      `json:"started_at,omitempty"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}
