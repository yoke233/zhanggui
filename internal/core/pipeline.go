package core

import (
	"fmt"
	"time"
)

// PipelineStatus represents the lifecycle state of a pipeline.
type PipelineStatus string

const (
	StatusCreated      PipelineStatus = "created"
	StatusRunning      PipelineStatus = "running"
	StatusWaitingHuman PipelineStatus = "waiting_human"
	StatusPaused       PipelineStatus = "paused"
	StatusDone         PipelineStatus = "done"
	StatusFailed       PipelineStatus = "failed"
	StatusAborted      PipelineStatus = "aborted"
)

// validTransitions encodes the state machine: from → set of legal targets.
var validTransitions = map[PipelineStatus]map[PipelineStatus]bool{
	StatusCreated: {
		StatusRunning: true,
		StatusAborted: true,
	},
	StatusRunning: {
		StatusWaitingHuman: true,
		StatusPaused:       true,
		StatusFailed:       true,
		StatusDone:         true,
	},
	StatusPaused: {
		StatusRunning: true,
		StatusAborted: true,
	},
	StatusWaitingHuman: {
		StatusRunning: true,
		StatusAborted: true,
	},
	StatusFailed: {
		StatusRunning: true,
	},
}

// ValidateTransition checks whether a status change is legal.
// Returns nil on success or an error describing why the transition is invalid.
func ValidateTransition(from, to PipelineStatus) error {
	targets, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("no transitions allowed from %q", from)
	}
	if !targets[to] {
		return fmt.Errorf("invalid transition: %q -> %q", from, to)
	}
	return nil
}

// Pipeline is the central aggregate representing one workflow execution.
type Pipeline struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Template    string            `json:"template"`
	Status      PipelineStatus    `json:"status"`

	CurrentStage StageID          `json:"current_stage,omitempty"`
	Stages       []StageConfig    `json:"stages"`
	Artifacts    map[string]string `json:"artifacts,omitempty"`
	Config       map[string]any    `json:"config,omitempty"`

	BranchName   string `json:"branch_name,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	MaxTotalRetries int `json:"max_total_retries"`
	TotalRetries    int `json:"total_retries"`

	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
