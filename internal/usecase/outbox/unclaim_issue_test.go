package outbox

import (
	"context"
	"strings"
	"testing"
)

func TestUnclaimIssueClearsAssigneeAndState(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "unclaim issue",
		Body:   "body",
		Labels: []string{"to:backend"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	if err := svc.UnclaimIssue(ctx, UnclaimIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		Comment:  "console unclaim",
	}); err != nil {
		t.Fatalf("UnclaimIssue() error = %v", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if strings.TrimSpace(got.Assignee) != "" {
		t.Fatalf("assignee = %q, want empty", got.Assignee)
	}
	if !contains(got.Labels, "state:todo") {
		t.Fatalf("labels = %v, want contains state:todo", got.Labels)
	}
	if len(got.Events) < 2 {
		t.Fatalf("events len = %d, want >= 2", len(got.Events))
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "Action: unclaim") {
		t.Fatalf("last event missing Action: unclaim, body=%s", last)
	}
	if !strings.Contains(last, "Status: todo") {
		t.Fatalf("last event missing Status: todo, body=%s", last)
	}

	if gotStatus := cache.data[cacheIssueStatusKey(issueRef)]; gotStatus != "state:todo" {
		t.Fatalf("cache status = %q, want state:todo", gotStatus)
	}
	if gotAssignee, ok := cache.data[cacheIssueAssigneeKey(issueRef)]; !ok || gotAssignee != "" {
		t.Fatalf("cache assignee = %q, want empty string", gotAssignee)
	}
}

func TestUnclaimIssueRequiresActor(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "unclaim actor required",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	err = svc.UnclaimIssue(ctx, UnclaimIssueInput{
		IssueRef: issueRef,
		Actor:    "",
	})
	if err == nil {
		t.Fatalf("UnclaimIssue() expected actor required error")
	}
	if !strings.Contains(err.Error(), "actor is required") {
		t.Fatalf("UnclaimIssue() error = %v, want actor required", err)
	}
}

func TestUnclaimIssueRejectsInvalidResultCode(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "unclaim invalid result code",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	err = svc.UnclaimIssue(ctx, UnclaimIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		Comment:  "Role: lead-backend\nIssueRef: " + issueRef + "\nRunId: none\nAction: unclaim\nStatus: todo\nResultCode: not_allowed\n\nSummary:\n- invalid\n\nChanges:\n- PR: none\n- Commit: none\n\nTests:\n- Command: none\n- Result: n/a\n- Evidence: none\n\nNext:\n- @lead check",
	})
	if err == nil {
		t.Fatalf("UnclaimIssue() expected invalid result code error")
	}
	if !strings.Contains(err.Error(), "invalid result code") {
		t.Fatalf("UnclaimIssue() error = %v, want invalid result code", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if strings.TrimSpace(got.Assignee) != "lead-backend" {
		t.Fatalf("assignee = %q, want lead-backend", got.Assignee)
	}
	if !contains(got.Labels, "state:doing") {
		t.Fatalf("labels = %v, want contains state:doing", got.Labels)
	}
	if len(got.Events) != 1 {
		t.Fatalf("events len = %d, want 1 (only claim event)", len(got.Events))
	}
}
