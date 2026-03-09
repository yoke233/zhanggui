package teamleader

import (
	"context"
	"errors"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

type gateStubDemandReviewer struct {
	verdict core.ReviewVerdict
	err     error
}

func (s *gateStubDemandReviewer) Review(_ context.Context, _ *core.Issue) (core.ReviewVerdict, error) {
	return s.verdict, s.err
}

func TestAutoGateRunner_Pass(t *testing.T) {
	t.Parallel()

	runner := &AutoGateRunner{
		Reviewer: &gateStubDemandReviewer{
			verdict: core.ReviewVerdict{Status: "pass", Score: 90, Summary: "looks good"},
		},
	}
	issue := &core.Issue{ID: "issue-1", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "lint", Type: core.GateTypeAuto}

	check, err := runner.Check(context.Background(), issue, gate, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Status != core.GateStatusPassed {
		t.Errorf("expected passed, got %q", check.Status)
	}
	if check.Reason != "looks good" {
		t.Errorf("expected reason 'looks good', got %q", check.Reason)
	}
	if check.CheckedBy != "auto" {
		t.Errorf("expected checked_by 'auto', got %q", check.CheckedBy)
	}
	if check.GateName != "lint" {
		t.Errorf("expected gate_name 'lint', got %q", check.GateName)
	}
	if check.IssueID != "issue-1" {
		t.Errorf("expected issue_id 'issue-1', got %q", check.IssueID)
	}
}

func TestAutoGateRunner_Fail(t *testing.T) {
	t.Parallel()

	runner := &AutoGateRunner{
		Reviewer: &gateStubDemandReviewer{
			verdict: core.ReviewVerdict{
				Status: "issues_found", Score: 40,
				Issues:  []core.ReviewIssue{{Description: "missing tests"}},
				Summary: "found issues",
			},
		},
	}
	issue := &core.Issue{ID: "issue-2", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "lint", Type: core.GateTypeAuto}

	check, err := runner.Check(context.Background(), issue, gate, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Status != core.GateStatusFailed {
		t.Errorf("expected failed, got %q", check.Status)
	}
}

func TestAutoGateRunner_ReviewerError(t *testing.T) {
	t.Parallel()

	runner := &AutoGateRunner{
		Reviewer: &gateStubDemandReviewer{
			err: errors.New("agent unavailable"),
		},
	}
	issue := &core.Issue{ID: "issue-err", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "lint", Type: core.GateTypeAuto}

	check, err := runner.Check(context.Background(), issue, gate, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Status != core.GateStatusFailed {
		t.Errorf("expected failed on error, got %q", check.Status)
	}
	if check.Reason == "" {
		t.Error("expected non-empty reason on error")
	}
}

func TestAutoGateRunner_NilReviewer(t *testing.T) {
	t.Parallel()

	runner := &AutoGateRunner{Reviewer: nil}
	issue := &core.Issue{ID: "issue-nil", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "lint", Type: core.GateTypeAuto}

	_, err := runner.Check(context.Background(), issue, gate, 1)
	if err == nil {
		t.Fatal("expected error for nil reviewer")
	}
}

func TestOwnerReviewRunner_Pending(t *testing.T) {
	t.Parallel()

	runner := &OwnerReviewRunner{}
	issue := &core.Issue{ID: "issue-3", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "owner", Type: core.GateTypeOwnerReview}

	check, err := runner.Check(context.Background(), issue, gate, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Status != core.GateStatusPending {
		t.Errorf("expected pending, got %q", check.Status)
	}
	if check.CheckedBy != "human" {
		t.Errorf("expected checked_by 'human', got %q", check.CheckedBy)
	}
	if check.GateName != "owner" {
		t.Errorf("expected gate_name 'owner', got %q", check.GateName)
	}
}

func TestPeerReviewRunner_Pending(t *testing.T) {
	t.Parallel()

	runner := &PeerReviewRunner{}
	issue := &core.Issue{ID: "issue-4", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "peer", Type: core.GateTypePeerReview}

	check, err := runner.Check(context.Background(), issue, gate, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Status != core.GateStatusPending {
		t.Errorf("expected pending, got %q", check.Status)
	}
	if check.Reason != "awaiting peer review" {
		t.Errorf("expected peer review reason, got %q", check.Reason)
	}
}

func TestVoteGateRunner_Pending(t *testing.T) {
	t.Parallel()

	runner := &VoteGateRunner{}
	issue := &core.Issue{ID: "issue-5", Title: "test", Template: "standard"}
	gate := core.Gate{Name: "vote", Type: core.GateTypeVote}

	check, err := runner.Check(context.Background(), issue, gate, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Status != core.GateStatusPending {
		t.Errorf("expected pending, got %q", check.Status)
	}
	if check.Reason != "awaiting vote" {
		t.Errorf("expected vote reason, got %q", check.Reason)
	}
}
