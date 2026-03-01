package core

import "context"

// ReviewGate performs TaskPlan review and returns review decisions.
type ReviewGate interface {
	Plugin
	Submit(ctx context.Context, plan *TaskPlan) (reviewID string, err error)
	Check(ctx context.Context, reviewID string) (*ReviewResult, error)
	Cancel(ctx context.Context, reviewID string) error
}

const (
	ReviewStatusPending          = "pending"
	ReviewStatusApproved         = "approved"
	ReviewStatusRejected         = "rejected"
	ReviewStatusChangesRequested = "changes_requested"
	ReviewStatusCancelled        = "cancelled"
)

const (
	ReviewDecisionPending   = "pending"
	ReviewDecisionApprove   = "approve"
	ReviewDecisionReject    = "reject"
	ReviewDecisionFix       = "fix"
	ReviewDecisionCancelled = "cancelled"
)

type ReviewResult struct {
	Status   string          `json:"status"`
	Verdicts []ReviewVerdict `json:"verdicts"`
	Decision string          `json:"decision"`
	Revised  *TaskPlan       `json:"revised,omitempty"`
	Comments []string        `json:"comments,omitempty"`
}

type ReviewVerdict struct {
	Reviewer string        `json:"reviewer"`
	Status   string        `json:"status"`
	Issues   []ReviewIssue `json:"issues"`
	Score    int           `json:"score"`
}

type ReviewIssue struct {
	Severity    string `json:"severity"`
	TaskID      string `json:"task_id"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}
