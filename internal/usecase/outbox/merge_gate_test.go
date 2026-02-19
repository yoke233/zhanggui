package outbox

import (
	"context"
	"testing"
)

func TestMergeGate_RequiresReviewApprovedAndQAPass(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()
	issueRef := createLeadClaimedIssue(t, svc, ctx, "merge gate", "body", []string{"to:backend", "state:review"})

	ok, reason, err := svc.CanMergeIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("CanMergeIssue() error = %v", err)
	}
	if ok {
		t.Fatalf("CanMergeIssue() ok = true, want false")
	}
	if reason != "missing review:approved" {
		t.Fatalf("CanMergeIssue() reason = %q, want missing review:approved", reason)
	}

	if err := svc.AddIssueLabels(ctx, AddIssueLabelsInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		Labels:   []string{"review:approved"},
	}); err != nil {
		t.Fatalf("AddIssueLabels(review:approved) error = %v", err)
	}

	ok, reason, err = svc.CanMergeIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("CanMergeIssue() error = %v", err)
	}
	if ok {
		t.Fatalf("CanMergeIssue() ok = true, want false after review approval only")
	}
	if reason != "missing qa:pass" {
		t.Fatalf("CanMergeIssue() reason = %q, want missing qa:pass", reason)
	}

	if err := svc.AddIssueLabels(ctx, AddIssueLabelsInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		Labels:   []string{"qa:pass"},
	}); err != nil {
		t.Fatalf("AddIssueLabels(qa:pass) error = %v", err)
	}

	ok, reason, err = svc.CanMergeIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("CanMergeIssue() error = %v", err)
	}
	if !ok {
		t.Fatalf("CanMergeIssue() ok = false, want true")
	}
	if reason != "ready" {
		t.Fatalf("CanMergeIssue() reason = %q, want ready", reason)
	}

	blockedIssueRef := createLeadClaimedIssue(t, svc, ctx, "merge gate blocked", "body", []string{
		"to:backend", "state:review", "review:approved", "qa:pass", "needs-human",
	})

	ok, reason, err = svc.CanMergeIssue(ctx, blockedIssueRef)
	if err != nil {
		t.Fatalf("CanMergeIssue(blocked) error = %v", err)
	}
	if ok {
		t.Fatalf("CanMergeIssue(blocked) ok = true, want false")
	}
	if reason != "needs-human present" {
		t.Fatalf("CanMergeIssue(blocked) reason = %q, want needs-human present", reason)
	}
}
