package outbox

import (
	"context"
	"testing"
)

func TestCodexPipeline_EndToEnd_ReviewAndTestLoopThenMergeReady(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()
	issueRef := createLeadClaimedIssue(t, svc, ctx, "pipeline e2e review and test loops", "body", []string{"to:backend", "state:doing"})

	fake := &fakeCodexRunner{
		results: []CodexRunOutput{
			{Status: "pass", Summary: "coding-1", Commit: "git:c1"},
			{Status: "fail", Summary: "review fail-1", ResultCode: "review_changes_requested", Evidence: "review://r1"},
			{Status: "pass", Summary: "coding-2", Commit: "git:c2"},
			{Status: "pass", Summary: "review pass-2", Evidence: "review://r2"},
			{Status: "fail", Summary: "tests fail-2", ResultCode: "ci_failed", Evidence: "test://t2"},
			{Status: "pass", Summary: "coding-3", Commit: "git:c3"},
			{Status: "pass", Summary: "review pass-3", Evidence: "review://r3"},
			{Status: "pass", Summary: "tests pass-3", Evidence: "test://t3"},
		},
	}
	svc.codexRunner = fake

	out, err := svc.RunCodexPipeline(ctx, RunCodexPipelineInput{
		IssueRef:       issueRef,
		ProjectDir:     ".",
		PromptFile:     "mailbox/issue.md",
		CodingRole:     "backend",
		MaxReviewRound: 3,
		MaxTestRound:   3,
	})
	if err != nil {
		t.Fatalf("RunCodexPipeline() error = %v", err)
	}
	if !out.ReadyToMerge {
		t.Fatalf("ReadyToMerge = false, want true: %+v", out)
	}
	if out.Rounds != 3 {
		t.Fatalf("Rounds = %d, want 3", out.Rounds)
	}

	wantModes := []CodexRunMode{
		CodexRunCoding,
		CodexRunReview,
		CodexRunCoding,
		CodexRunReview,
		CodexRunTest,
		CodexRunCoding,
		CodexRunReview,
		CodexRunTest,
	}
	if len(fake.calls) != len(wantModes) {
		t.Fatalf("runner calls = %d, want %d", len(fake.calls), len(wantModes))
	}
	for i, want := range wantModes {
		if fake.calls[i].Mode != want {
			t.Fatalf("call[%d].Mode = %q, want %q", i, fake.calls[i].Mode, want)
		}
	}

	ok, reason, err := svc.CanMergeIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("CanMergeIssue() error = %v", err)
	}
	if !ok {
		t.Fatalf("CanMergeIssue() ok = false, reason=%q", reason)
	}

	issue, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(issue.Labels, "review:approved") {
		t.Fatalf("labels = %v, want review:approved", issue.Labels)
	}
	if !contains(issue.Labels, "qa:pass") {
		t.Fatalf("labels = %v, want qa:pass", issue.Labels)
	}

	qualityEvents, err := svc.ListQualityEvents(ctx, issueRef, 20)
	if err != nil {
		t.Fatalf("ListQualityEvents() error = %v", err)
	}
	if len(qualityEvents) != 5 {
		t.Fatalf("quality events len = %d, want 5", len(qualityEvents))
	}
}
