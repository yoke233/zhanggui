package core

import (
	"fmt"
	"time"
)

// RunStatus represents the lifecycle state of a run.
type RunStatus string

const (
	StatusCreated       RunStatus = "created"
	StatusRunning       RunStatus = "running"
	StatusWaitingReview RunStatus = "waiting_review"
	StatusDone          RunStatus = "done"
	StatusFailed        RunStatus = "failed"
	StatusTimeout       RunStatus = "timeout"
)

// validTransitions encodes the state machine: from -> set of legal targets.
var validTransitions = map[RunStatus]map[RunStatus]bool{
	StatusCreated: {
		StatusRunning: true,
	},
	StatusRunning: {
		StatusWaitingReview: true,
		StatusDone:          true,
		StatusFailed:        true,
		StatusTimeout:       true,
	},
	StatusWaitingReview: {
		StatusRunning: true,
		StatusDone:    true,
		StatusFailed:  true,
		StatusTimeout: true,
	},
	StatusFailed: {
		StatusRunning: true,
	},
	StatusTimeout: {
		StatusRunning: true,
	},
}

// ValidateTransition checks whether a status change is legal.
// Returns nil on success or an error describing why the transition is invalid.
func ValidateTransition(from, to RunStatus) error {
	targets, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("no transitions allowed from %q", from)
	}
	if !targets[to] {
		return fmt.Errorf("invalid transition: %q -> %q", from, to)
	}
	return nil
}

// Run is the central aggregate representing one workflow execution.
type Run struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Template    string    `json:"template"`
	Status      RunStatus `json:"status"`

	CurrentStage StageID           `json:"current_stage"`
	Stages       []StageConfig     `json:"stages"`
	Artifacts    map[string]string `json:"artifacts"`
	Config       map[string]any    `json:"config"`
	BranchName   string            `json:"branch_name"`
	WorktreePath string            `json:"worktree_path"`
	IssueID      string            `json:"issue_id"`
	ErrorMessage string            `json:"error_message,omitempty"`

	MaxTotalRetries int    `json:"max_total_retries"`
	TotalRetries    int    `json:"total_retries"`
	RunCount        int    `json:"run_count,omitempty"`
	LastErrorType   string `json:"last_error_type,omitempty"`

	QueuedAt        time.Time `json:"queued_at,omitempty"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
