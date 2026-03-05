package core

import (
	"fmt"
	"time"
)

// RunStatus represents the lifecycle state of a run (GitHub Actions model).
type RunStatus string

const (
	StatusQueued         RunStatus = "queued"
	StatusInProgress     RunStatus = "in_progress"
	StatusCompleted      RunStatus = "completed"
	StatusActionRequired RunStatus = "action_required"
)

// RunConclusion represents the outcome of a completed run.
type RunConclusion string

const (
	ConclusionSuccess   RunConclusion = "success"
	ConclusionFailure   RunConclusion = "failure"
	ConclusionTimedOut  RunConclusion = "timed_out"
	ConclusionCancelled RunConclusion = "cancelled"
)

// validTransitions encodes the state machine: from -> set of legal targets.
var validTransitions = map[RunStatus]map[RunStatus]bool{
	StatusQueued: {
		StatusInProgress: true,
		StatusCompleted:  true, // abort
	},
	StatusInProgress: {
		StatusCompleted:      true,
		StatusActionRequired: true,
		StatusQueued:         true, // re-enqueue
	},
	StatusActionRequired: {
		StatusInProgress: true,
		StatusCompleted:  true,
		StatusQueued:     true, // re-enqueue
	},
	StatusCompleted: {
		StatusInProgress: true, // retry from failure
	},
}

// ValidateTransition checks whether a status change is legal.
// Returns nil on success or an error describing why the transition is invalid.
func ValidateTransition(from, to RunStatus) error {
	if from == to {
		return nil // idempotent
	}
	targets, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("no transitions allowed from %q", from)
	}
	if !targets[to] {
		return fmt.Errorf("invalid transition: %q -> %q", from, to)
	}
	return nil
}

// TransitionStatus validates and applies a status change on the Run.
// Idempotent transitions (from == to) are always allowed.
func (r *Run) TransitionStatus(to RunStatus) error {
	if err := ValidateTransition(r.Status, to); err != nil {
		return err
	}
	r.Status = to
	r.UpdatedAt = time.Now()
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

	Conclusion   RunConclusion     `json:"conclusion,omitempty"`
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
