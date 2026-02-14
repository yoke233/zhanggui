package outbox

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestLeadSyncOnceBlockedByNeedsHuman(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "needs human lead block",
		Body:   "body",
		Labels: []string{"to:backend", "state:todo", "needs-human"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
		Comment:  "Action: claim\nStatus: doing",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:           "backend",
		Assignee:       "lead-backend",
		WorkflowFile:   workflowPath,
		ExecutablePath: "definitely-not-existing-executable",
		EventBatch:     100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Processed != 1 {
		t.Fatalf("Processed = %d, want 1", result.Processed)
	}
	if result.Blocked != 1 {
		t.Fatalf("Blocked = %d, want 1", result.Blocked)
	}
	if result.Spawned != 0 {
		t.Fatalf("Spawned = %d, want 0", result.Spawned)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if len(got.Events) == 0 {
		t.Fatalf("events should not be empty")
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "manual:needs-human") || !strings.Contains(last, "needs-human") {
		t.Fatalf("last event body = %s", last)
	}
}

func TestLeadSyncOnceBlockedByDependsOn(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	depRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "dependency",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue(dependency) error = %v", err)
	}
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title: "depends-on lead block",
		Body:  "## Dependencies\n- DependsOn:\n  - " + depRef + "\n- BlockedBy:\n  - none\n",
		Labels: []string{
			"to:backend",
			"state:todo",
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue(main) error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
		Comment:  "Action: claim\nStatus: doing",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:           "backend",
		Assignee:       "lead-backend",
		WorkflowFile:   workflowPath,
		ExecutablePath: "definitely-not-existing-executable",
		EventBatch:     100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Processed != 1 {
		t.Fatalf("Processed = %d, want 1", result.Processed)
	}
	if result.Blocked != 1 {
		t.Fatalf("Blocked = %d, want 1", result.Blocked)
	}
	if result.Spawned != 0 {
		t.Fatalf("Spawned = %d, want 0", result.Spawned)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if len(got.Events) == 0 {
		t.Fatalf("events should not be empty")
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "manual:depends-on") || !strings.Contains(last, depRef) {
		t.Fatalf("last event body = %s", last)
	}
}

func TestLeadSyncOnceSkipsAutoflowOff(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "autoflow off issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:todo", "autoflow:off"},
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

	before, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(before) error = %v", err)
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:           "backend",
		Assignee:       "lead-backend",
		WorkflowFile:   workflowPath,
		ExecutablePath: "definitely-not-existing-executable",
		EventBatch:     100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Processed != 0 {
		t.Fatalf("Processed = %d, want 0", result.Processed)
	}
	if result.Blocked != 0 {
		t.Fatalf("Blocked = %d, want 0", result.Blocked)
	}
	if result.Spawned != 0 {
		t.Fatalf("Spawned = %d, want 0", result.Spawned)
	}

	after, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(after) error = %v", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("events len = %d, want %d", len(after.Events), len(before.Events))
	}
}

func TestLeadSyncOncePersistsCursor(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "cursor issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:todo"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-backend",
		Actor:    "lead-backend",
		Comment:  "Action: claim\nStatus: doing",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}
	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "doing",
		Body:     "Action: update\nStatus: doing",
	}); err != nil {
		t.Fatalf("CommentIssue() error = %v", err)
	}

	first, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:           "backend",
		Assignee:       "lead-backend",
		WorkflowFile:   workflowPath,
		ExecutablePath: "definitely-not-existing-executable",
		EventBatch:     100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce(first) error = %v", err)
	}
	if first.CursorAfter <= first.CursorBefore {
		t.Fatalf("cursor should move forward, before=%d after=%d", first.CursorBefore, first.CursorAfter)
	}

	cursorKey := leadCursorKey("backend")
	if cache.data[cursorKey] != strconv.FormatUint(first.CursorAfter, 10) {
		t.Fatalf("cursor cache = %q, want %d", cache.data[cursorKey], first.CursorAfter)
	}

	second, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:           "backend",
		Assignee:       "lead-backend",
		WorkflowFile:   workflowPath,
		ExecutablePath: "definitely-not-existing-executable",
		EventBatch:     100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce(second) error = %v", err)
	}
	if second.CursorBefore != first.CursorAfter {
		t.Fatalf("second cursor before = %d, want %d", second.CursorBefore, first.CursorAfter)
	}
	if second.CursorAfter < second.CursorBefore {
		t.Fatalf("second cursor should not move backward, before=%d after=%d", second.CursorBefore, second.CursorAfter)
	}
	if cache.data[cursorKey] != strconv.FormatUint(second.CursorAfter, 10) {
		t.Fatalf("cursor cache = %q, want %d", cache.data[cursorKey], second.CursorAfter)
	}
}

func TestShouldSkipLeadSpawnByState(t *testing.T) {
	testCases := []struct {
		name   string
		labels []string
		want   bool
	}{
		{name: "blocked", labels: []string{"state:blocked"}, want: true},
		{name: "review", labels: []string{"state:review"}, want: true},
		{name: "done", labels: []string{"state:done"}, want: true},
		{name: "doing", labels: []string{"state:doing"}, want: false},
		{name: "todo", labels: []string{"state:todo"}, want: false},
		{name: "no state", labels: []string{"to:backend"}, want: false},
		{name: "with space", labels: []string{"  state:review  "}, want: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := shouldSkipLeadSpawnByState(testCase.labels)
			if got != testCase.want {
				t.Fatalf("shouldSkipLeadSpawnByState() = %v, want %v", got, testCase.want)
			}
		})
	}
}
