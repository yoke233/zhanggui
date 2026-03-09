package core

import (
	"fmt"
	"strings"
	"time"
)

// TaskStepAction represents an action recorded in a TaskStep.
type TaskStepAction string

// Issue state-transition actions.
const (
	StepCreated            TaskStepAction = "created"
	StepSubmittedForReview TaskStepAction = "submitted_for_review"
	StepReviewApproved     TaskStepAction = "review_approved"
	StepReviewRejected     TaskStepAction = "review_rejected"
	StepQueued             TaskStepAction = "queued"
	StepReady              TaskStepAction = "ready"
	StepExecutionStarted   TaskStepAction = "execution_started"
	StepMergeStarted       TaskStepAction = "merge_started"
	StepCompleted          TaskStepAction = "completed"
	StepMergeCompleted     TaskStepAction = "merge_completed"
	StepFailed             TaskStepAction = "failed"
	StepAbandoned          TaskStepAction = "abandoned"
	StepDecomposeStarted   TaskStepAction = "decompose_started"
	StepDecomposed         TaskStepAction = "decomposed"
	StepSuperseded         TaskStepAction = "superseded"
)

// Run-level actions (do not change Issue.Status).
const (
	StepRunCreated     TaskStepAction = "run_created"
	StepRunStarted     TaskStepAction = "run_started"
	StepStageStarted   TaskStepAction = "stage_started"
	StepStageCompleted TaskStepAction = "stage_completed"
	StepStageFailed    TaskStepAction = "stage_failed"
	StepRunCompleted   TaskStepAction = "run_completed"
	StepRunFailed      TaskStepAction = "run_failed"
)

// Gate-level actions (do not change Issue.Status).
const (
	StepGateCheck  TaskStepAction = "gate_check"
	StepGatePassed TaskStepAction = "gate_passed"
	StepGateFailed TaskStepAction = "gate_failed"
)

var actionToStatus = map[TaskStepAction]IssueStatus{
	StepCreated:            IssueStatusDraft,
	StepSubmittedForReview: IssueStatusReviewing,
	StepReviewApproved:     IssueStatusQueued,
	StepReviewRejected:     IssueStatusDraft,
	StepQueued:             IssueStatusQueued,
	StepReady:              IssueStatusReady,
	StepExecutionStarted:   IssueStatusExecuting,
	StepMergeStarted:       IssueStatusMerging,
	StepCompleted:          IssueStatusDone,
	StepMergeCompleted:     IssueStatusDone,
	StepFailed:             IssueStatusFailed,
	StepAbandoned:          IssueStatusAbandoned,
	StepDecomposeStarted:   IssueStatusDecomposing,
	StepDecomposed:         IssueStatusDecomposed,
	StepSuperseded:         IssueStatusSuperseded,
}

var validTaskStepActions = map[TaskStepAction]struct{}{
	StepCreated: {}, StepSubmittedForReview: {}, StepReviewApproved: {},
	StepReviewRejected: {}, StepQueued: {}, StepReady: {},
	StepExecutionStarted: {}, StepMergeStarted: {}, StepCompleted: {}, StepMergeCompleted: {},
	StepFailed: {}, StepAbandoned: {}, StepDecomposeStarted: {},
	StepDecomposed: {}, StepSuperseded: {},
	StepRunCreated: {}, StepRunStarted: {}, StepStageStarted: {},
	StepStageCompleted: {}, StepStageFailed: {}, StepRunCompleted: {}, StepRunFailed: {},
	StepGateCheck: {}, StepGatePassed: {}, StepGateFailed: {},
}

// DeriveIssueStatus returns the IssueStatus this action implies.
// Returns ("", false) for run-level actions that don't change Issue status.
func (a TaskStepAction) DeriveIssueStatus() (IssueStatus, bool) {
	status, ok := actionToStatus[a]
	return status, ok
}

// TaskStep records a single business fact in the issue lifecycle.
type TaskStep struct {
	ID        string         `json:"id"`
	IssueID   string         `json:"issue_id"`
	RunID     string         `json:"run_id,omitempty"`
	AgentID   string         `json:"agent_id,omitempty"`
	Action    TaskStepAction `json:"action"`
	StageID   StageID        `json:"stage_id,omitempty"`
	Input     string         `json:"input,omitempty"`
	Output    string         `json:"output,omitempty"`
	Note      string         `json:"note,omitempty"`
	RefID     string         `json:"ref_id,omitempty"`
	RefType   string         `json:"ref_type,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// Validate checks required TaskStep fields.
func (s TaskStep) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("task step ID is required")
	}
	if strings.TrimSpace(s.IssueID) == "" {
		return fmt.Errorf("task step issue_id is required")
	}
	if _, ok := validTaskStepActions[s.Action]; !ok {
		return fmt.Errorf("invalid task step action %q", s.Action)
	}
	return nil
}

// NewTaskStepID generates an ID in format: step-YYYYMMDD-HHMMSS-xxxxxxxx.
func NewTaskStepID() string {
	return fmt.Sprintf("step-%s-%s", time.Now().Format("20060102-150405"), randomHex(4))
}
