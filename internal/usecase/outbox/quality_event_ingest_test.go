package outbox

import (
	"context"
	"strings"
	"testing"
)

func TestIngestQualityEventReviewChangesRequestedWritesStructuredComment(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "quality review issue",
		Body:   "body",
		Labels: []string{"to:backend", "to:reviewer", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-integrator",
		Actor:    "lead-integrator",
		Comment:  "claim by integrator",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}
	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-integrator",
		State:    "review",
		Body:     "back to review",
	}); err != nil {
		t.Fatalf("CommentIssue(review) error = %v", err)
	}

	result, err := svc.IngestQualityEvent(ctx, IngestQualityEventInput{
		IssueRef:         issueRef,
		Source:           "github",
		ExternalEventID:  "pr#1/review#3",
		Category:         "review",
		Result:           "changes_requested",
		Actor:            "quality-bot",
		Summary:          "blocking issue found",
		Evidence:         []string{"https://github.com/org/repo/pull/1#pullrequestreview-3"},
		ProvidedEventKey: "",
	})
	if err != nil {
		t.Fatalf("IngestQualityEvent() error = %v", err)
	}
	if result.Duplicate {
		t.Fatalf("IngestQualityEvent() duplicate = true, want false")
	}
	if result.Marker != "review:changes_requested" {
		t.Fatalf("marker = %q", result.Marker)
	}
	if result.RoutedRole != "backend" {
		t.Fatalf("routed role = %q, want backend", result.RoutedRole)
	}
	if !result.CommentWritten {
		t.Fatalf("comment should be written")
	}

	issue, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if issue.Assignee != "lead-integrator" {
		t.Fatalf("assignee = %q, want lead-integrator", issue.Assignee)
	}
	if !contains(issue.Labels, "state:review") {
		t.Fatalf("labels = %v, want contains state:review", issue.Labels)
	}
	last := issue.Events[len(issue.Events)-1].Body
	if !strings.Contains(last, "review:changes_requested") {
		t.Fatalf("last event should contain review marker, got: %s", last)
	}
	if !strings.Contains(last, "ResultCode: review_changes_requested") {
		t.Fatalf("last event should contain result_code, got: %s", last)
	}
	if !strings.Contains(last, "@backend address quality failure and rerun") {
		t.Fatalf("last event should contain backend route, got: %s", last)
	}
}

func TestIngestQualityEventDeduplicatesByEventKey(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "quality dedup issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	first, err := svc.IngestQualityEvent(ctx, IngestQualityEventInput{
		IssueRef:         issueRef,
		Source:           "github",
		ExternalEventID:  "check#100",
		Category:         "ci",
		Result:           "fail",
		Actor:            "quality-bot",
		Summary:          "unit tests failed",
		Evidence:         []string{"https://ci.example/build/100"},
		ProvidedEventKey: "evt:check:100",
	})
	if err != nil {
		t.Fatalf("IngestQualityEvent(first) error = %v", err)
	}
	if first.Duplicate {
		t.Fatalf("first event should not be duplicate")
	}

	second, err := svc.IngestQualityEvent(ctx, IngestQualityEventInput{
		IssueRef:         issueRef,
		Source:           "github",
		ExternalEventID:  "check#100",
		Category:         "ci",
		Result:           "fail",
		Actor:            "quality-bot",
		Summary:          "same event replay",
		Evidence:         []string{"https://ci.example/build/100"},
		ProvidedEventKey: "evt:check:100",
	})
	if err != nil {
		t.Fatalf("IngestQualityEvent(second) error = %v", err)
	}
	if !second.Duplicate {
		t.Fatalf("second event should be duplicate")
	}
	if second.CommentWritten {
		t.Fatalf("duplicate event should not write comment")
	}

	issue, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if len(issue.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(issue.Events))
	}

	items, err := svc.ListQualityEvents(ctx, issueRef, 20)
	if err != nil {
		t.Fatalf("ListQualityEvents() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListQualityEvents() len = %d, want 1", len(items))
	}
}

func TestIngestQualityEventCIFailRequiresEvidence(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "ci fail issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	_, err = svc.IngestQualityEvent(ctx, IngestQualityEventInput{
		IssueRef:        issueRef,
		Source:          "github",
		ExternalEventID: "check#101",
		Category:        "ci",
		Result:          "fail",
		Actor:           "quality-bot",
	})
	if err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("IngestQualityEvent() error = %v, want evidence error", err)
	}
}

func TestIngestQualityEventCIPassRoutesToIntegrator(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "ci pass issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	result, err := svc.IngestQualityEvent(ctx, IngestQualityEventInput{
		IssueRef:        issueRef,
		Source:          "github",
		ExternalEventID: "check#102",
		Category:        "ci",
		Result:          "pass",
		Actor:           "quality-bot",
		Evidence:        []string{"https://ci.example/build/102"},
	})
	if err != nil {
		t.Fatalf("IngestQualityEvent() error = %v", err)
	}
	if result.Marker != "qa:pass" {
		t.Fatalf("marker = %q, want qa:pass", result.Marker)
	}
	if result.RoutedRole != "integrator" {
		t.Fatalf("routed role = %q, want integrator", result.RoutedRole)
	}

	issue, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	last := issue.Events[len(issue.Events)-1].Body
	if !strings.Contains(last, "qa:pass") {
		t.Fatalf("last event should contain qa:pass, got: %s", last)
	}
	if !strings.Contains(last, "@integrator review quality gate signals") {
		t.Fatalf("last event should route to integrator, got: %s", last)
	}
}

func TestIngestQualityEventInfersGitHubReviewFromPayload(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "github review payload issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	payload := `{
  "action": "submitted",
  "review": {
    "id": 3001,
    "state": "approved",
    "body": "looks good",
    "html_url": "https://github.com/org/repo/pull/10#pullrequestreview-3001",
    "user": { "login": "reviewer-a" }
  },
  "pull_request": {
    "number": 10,
    "html_url": "https://github.com/org/repo/pull/10"
  },
  "repository": {
    "full_name": "org/repo"
  },
  "sender": { "login": "reviewer-a" }
}`

	result, err := svc.IngestQualityEvent(ctx, IngestQualityEventInput{
		IssueRef: issueRef,
		Source:   "github",
		Payload:  payload,
	})
	if err != nil {
		t.Fatalf("IngestQualityEvent() error = %v", err)
	}
	if result.Marker != "review:approved" {
		t.Fatalf("marker = %q, want review:approved", result.Marker)
	}
	if result.RoutedRole != "integrator" {
		t.Fatalf("routed role = %q, want integrator", result.RoutedRole)
	}

	items, err := svc.ListQualityEvents(ctx, issueRef, 20)
	if err != nil {
		t.Fatalf("ListQualityEvents() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListQualityEvents() len = %d, want 1", len(items))
	}
	if items[0].ExternalEventID != "github:review:3001" {
		t.Fatalf("external_event_id = %q, want github:review:3001", items[0].ExternalEventID)
	}
	if len(items[0].Evidence) == 0 || items[0].Evidence[0] != "https://github.com/org/repo/pull/10#pullrequestreview-3001" {
		t.Fatalf("evidence = %v", items[0].Evidence)
	}
}

func TestIngestQualityEventInfersGitHubCheckRunFailureFromPayload(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "github check payload issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	payload := `{
  "action": "completed",
  "check_run": {
    "id": 9001,
    "name": "ci/test",
    "status": "completed",
    "conclusion": "failure",
    "html_url": "https://github.com/org/repo/runs/9001"
  },
  "repository": {
    "full_name": "org/repo"
  }
}`

	result, err := svc.IngestQualityEvent(ctx, IngestQualityEventInput{
		IssueRef: issueRef,
		Source:   "github",
		Payload:  payload,
	})
	if err != nil {
		t.Fatalf("IngestQualityEvent() error = %v", err)
	}
	if result.Marker != "qa:fail" {
		t.Fatalf("marker = %q, want qa:fail", result.Marker)
	}
	if result.RoutedRole != "backend" {
		t.Fatalf("routed role = %q, want backend", result.RoutedRole)
	}

	issue, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	last := issue.Events[len(issue.Events)-1].Body
	if !strings.Contains(last, "qa:fail") {
		t.Fatalf("last event should contain qa:fail, got: %s", last)
	}
	if !strings.Contains(last, "https://github.com/org/repo/runs/9001") {
		t.Fatalf("last event should contain check evidence url, got: %s", last)
	}
}
