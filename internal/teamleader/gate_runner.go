package teamleader

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// AutoGateRunner evaluates an auto gate using the existing DemandReviewer.
type AutoGateRunner struct {
	Reviewer DemandReviewer
}

func (r *AutoGateRunner) Check(ctx context.Context, issue *core.Issue, gate core.Gate, attempt int) (*core.GateCheck, error) {
	if r.Reviewer == nil {
		return nil, fmt.Errorf("auto gate runner: reviewer is nil")
	}

	verdict, err := r.Reviewer.Review(ctx, cloneIssueForReview(issue))
	if err != nil {
		return &core.GateCheck{
			ID:        core.NewGateCheckID(),
			IssueID:   issue.ID,
			GateName:  gate.Name,
			GateType:  gate.Type,
			Attempt:   attempt,
			Status:    core.GateStatusFailed,
			Reason:    fmt.Sprintf("review error: %v", err),
			CheckedBy: "auto",
			CreatedAt: time.Now(),
		}, nil
	}

	status := core.GateStatusPassed
	if verdictNeedsFix(verdict) {
		status = core.GateStatusFailed
	}

	return &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issue.ID,
		GateName:  gate.Name,
		GateType:  gate.Type,
		Attempt:   attempt,
		Status:    status,
		Reason:    verdict.Summary,
		CheckedBy: "auto",
		CreatedAt: time.Now(),
	}, nil
}

// OwnerReviewRunner creates a pending gate check that waits for human resolution.
type OwnerReviewRunner struct{}

func (r *OwnerReviewRunner) Check(_ context.Context, issue *core.Issue, gate core.Gate, attempt int) (*core.GateCheck, error) {
	return &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issue.ID,
		GateName:  gate.Name,
		GateType:  gate.Type,
		Attempt:   attempt,
		Status:    core.GateStatusPending,
		Reason:    "awaiting owner review",
		CheckedBy: "human",
		CreatedAt: time.Now(),
	}, nil
}

// PeerReviewRunner creates a pending gate check that waits for peer review.
type PeerReviewRunner struct{}

func (r *PeerReviewRunner) Check(_ context.Context, issue *core.Issue, gate core.Gate, attempt int) (*core.GateCheck, error) {
	return &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issue.ID,
		GateName:  gate.Name,
		GateType:  gate.Type,
		Attempt:   attempt,
		Status:    core.GateStatusPending,
		Reason:    "awaiting peer review",
		CheckedBy: "human",
		CreatedAt: time.Now(),
	}, nil
}

// VoteGateRunner creates a pending gate check that waits for a human vote.
type VoteGateRunner struct{}

func (r *VoteGateRunner) Check(_ context.Context, issue *core.Issue, gate core.Gate, attempt int) (*core.GateCheck, error) {
	return &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issue.ID,
		GateName:  gate.Name,
		GateType:  gate.Type,
		Attempt:   attempt,
		Status:    core.GateStatusPending,
		Reason:    "awaiting vote",
		CheckedBy: "human",
		CreatedAt: time.Now(),
	}, nil
}
