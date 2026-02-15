package outbox

import (
	"context"
	"strings"
	"testing"
)

func TestAddIssueLabelsAddsLabelsAndReplacesState(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "label add issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:todo"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	if err := svc.AddIssueLabels(ctx, AddIssueLabelsInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		Labels:   []string{" needs-human ", "state:doing"},
	}); err != nil {
		t.Fatalf("AddIssueLabels() error = %v", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "needs-human") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if !contains(got.Labels, "state:doing") || contains(got.Labels, "state:todo") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if len(got.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(got.Events))
	}
	if !strings.Contains(got.Events[0].Body, "Action: label-add") {
		t.Fatalf("event body = %s", got.Events[0].Body)
	}
}

func TestAddIssueLabelsRejectsMultipleStateLabels(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "multi state label issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:todo"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	if err := svc.AddIssueLabels(ctx, AddIssueLabelsInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		Labels:   []string{"state:doing", "state:blocked"},
	}); err == nil {
		t.Fatalf("AddIssueLabels() expected error")
	}
}

func TestRemoveIssueLabelsRemovesLabels(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "label remove issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:todo", "needs-human"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	if err := svc.RemoveIssueLabels(ctx, RemoveIssueLabelsInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		Labels:   []string{"needs-human", "state:todo"},
	}); err != nil {
		t.Fatalf("RemoveIssueLabels() error = %v", err)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if contains(got.Labels, "needs-human") || contains(got.Labels, "state:todo") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if len(got.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(got.Events))
	}
	if !strings.Contains(got.Events[0].Body, "Action: label-remove") {
		t.Fatalf("event body = %s", got.Events[0].Body)
	}
}
