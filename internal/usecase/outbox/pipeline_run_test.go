package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeCodexRunner struct {
	results []CodexRunOutput
	calls   []CodexRunInput
}

func (f *fakeCodexRunner) Run(ctx context.Context, input CodexRunInput) (CodexRunOutput, error) {
	if err := ctx.Err(); err != nil {
		return CodexRunOutput{}, err
	}

	f.calls = append(f.calls, input)
	idx := len(f.calls) - 1
	if idx >= len(f.results) {
		return CodexRunOutput{}, errors.New("unexpected codex runner call")
	}
	return f.results[idx], nil
}

func TestRunCodexPipeline_ReviewFailThenFixThenTestPass(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()
	issueRef := createLeadClaimedIssue(t, svc, ctx, "pipeline review fail then pass", "body", []string{"to:backend", "state:doing"})

	fake := &fakeCodexRunner{
		results: []CodexRunOutput{
			{Status: "pass", Summary: "coding-1", Commit: "git:c1"},
			{Status: "fail", Summary: "review found issues", ResultCode: "review_changes_requested", Evidence: "review://r1"},
			{Status: "pass", Summary: "coding-2", Commit: "git:c2"},
			{Status: "pass", Summary: "review approved", Evidence: "review://r2"},
			{Status: "pass", Summary: "tests passed", Evidence: "test://t2"},
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
		t.Fatalf("expected ReadyToMerge=true, got %+v", out)
	}
	if out.Rounds != 2 {
		t.Fatalf("Rounds = %d, want 2", out.Rounds)
	}
	if out.LastResultCode != "none" {
		t.Fatalf("LastResultCode = %q, want none", out.LastResultCode)
	}

	if len(fake.calls) != 5 {
		t.Fatalf("runner calls = %d, want 5", len(fake.calls))
	}
	wantModes := []CodexRunMode{CodexRunCoding, CodexRunReview, CodexRunCoding, CodexRunReview, CodexRunTest}
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
	if len(qualityEvents) != 3 {
		t.Fatalf("quality events len = %d, want 3", len(qualityEvents))
	}
}

func TestRunCodexPipeline_ReviewFailExceedsMaxRoundManualIntervention(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()
	issueRef := createLeadClaimedIssue(t, svc, ctx, "pipeline max review round", "body", []string{"to:backend", "state:doing"})

	fake := &fakeCodexRunner{
		results: []CodexRunOutput{
			{Status: "pass", Summary: "coding-1", Commit: "git:c1"},
			{Status: "fail", Summary: "review fail-1", ResultCode: "review_changes_requested", Evidence: "review://r1"},
			{Status: "pass", Summary: "coding-2", Commit: "git:c2"},
			{Status: "fail", Summary: "review fail-2", ResultCode: "review_changes_requested", Evidence: "review://r2"},
		},
	}
	svc.codexRunner = fake

	out, err := svc.RunCodexPipeline(ctx, RunCodexPipelineInput{
		IssueRef:       issueRef,
		ProjectDir:     ".",
		PromptFile:     "mailbox/issue.md",
		CodingRole:     "backend",
		MaxReviewRound: 1,
		MaxTestRound:   3,
	})
	if err != nil {
		t.Fatalf("RunCodexPipeline() error = %v", err)
	}
	if out.ReadyToMerge {
		t.Fatalf("ReadyToMerge = true, want false")
	}
	if out.LastResultCode != "manual_intervention" {
		t.Fatalf("LastResultCode = %q, want manual_intervention", out.LastResultCode)
	}
	if !strings.Contains(out.LastResult, "review") {
		t.Fatalf("LastResult = %q, want contains review", out.LastResult)
	}

	issue, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(issue.Labels, "needs-human") {
		t.Fatalf("labels = %v, want needs-human", issue.Labels)
	}
	if !contains(issue.Labels, "state:blocked") {
		t.Fatalf("labels = %v, want state:blocked", issue.Labels)
	}
}

func TestRunCodexPipeline_ReRunPersistsDistinctQualityEvents(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()
	issueRef := createLeadClaimedIssue(t, svc, ctx, "pipeline rerun quality audit", "body", []string{"to:backend", "state:doing"})

	runPassPipeline := func(seq string) {
		svc.codexRunner = &fakeCodexRunner{
			results: []CodexRunOutput{
				{Status: "pass", Summary: "coding-" + seq, Commit: "git:c" + seq},
				{Status: "pass", Summary: "review-" + seq, Evidence: "review://" + seq},
				{Status: "pass", Summary: "tests-" + seq, Evidence: "test://" + seq},
			},
		}
		out, err := svc.RunCodexPipeline(ctx, RunCodexPipelineInput{
			IssueRef:       issueRef,
			ProjectDir:     ".",
			PromptFile:     "mailbox/issue.md",
			CodingRole:     "backend",
			MaxReviewRound: 3,
			MaxTestRound:   3,
		})
		if err != nil {
			t.Fatalf("RunCodexPipeline() seq=%s error = %v", seq, err)
		}
		if !out.ReadyToMerge {
			t.Fatalf("RunCodexPipeline() seq=%s ReadyToMerge = false", seq)
		}
	}

	runPassPipeline("1")
	runPassPipeline("2")

	qualityEvents, err := svc.ListQualityEvents(ctx, issueRef, 20)
	if err != nil {
		t.Fatalf("ListQualityEvents() error = %v", err)
	}
	if len(qualityEvents) != 4 {
		t.Fatalf("quality events len = %d, want 4", len(qualityEvents))
	}

	keys := make(map[string]struct{}, len(qualityEvents))
	for _, item := range qualityEvents {
		if _, duplicated := keys[item.IdempotencyKey]; duplicated {
			t.Fatalf("duplicate idempotency key detected: %s", item.IdempotencyKey)
		}
		keys[item.IdempotencyKey] = struct{}{}
	}
}
