package outbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeReviewerWorkflow(t *testing.T, backendListenLabels []string) string {
	t.Helper()

	backendLabels := `["to:backend"]`
	if len(backendListenLabels) > 0 {
		quoted := make([]string, 0, len(backendListenLabels))
		for _, label := range backendListenLabels {
			quoted = append(quoted, `"`+strings.TrimSpace(label)+`"`)
		}
		backendLabels = "[" + strings.Join(quoted, ", ") + "]"
	}

	content := `
version = 2

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[roles]
enabled = ["backend", "reviewer"]

[repos]
main = "."

[role_repo]
backend = "main"
reviewer = "main"

[groups.backend]
role = "backend"
max_concurrent = 2
mode = "owner"
writeback = "full"
listen_labels = ` + backendLabels + `

[groups.reviewer]
role = "reviewer"
max_concurrent = 1
mode = "subscriber"
writeback = "comment-only"
listen_labels = ["to:reviewer", "state:review"]

[executors.backend]
program = "go"
args = ["test", "./..."]
timeout_seconds = 30

[executors.reviewer]
program = "go"
args = ["test", "./..."]
timeout_seconds = 30
`

	path := filepath.Join(t.TempDir(), "workflow.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}
	return path
}

func claimIssueAsIntegrator(t *testing.T, svc *Service, ctx context.Context, issueRef string) {
	t.Helper()

	if err := svc.ClaimIssue(ctx, ClaimIssueInput{
		IssueRef: issueRef,
		Assignee: "lead-integrator",
		Actor:    "lead-integrator",
		Comment:  "Action: claim\nStatus: doing",
	}); err != nil {
		t.Fatalf("ClaimIssue() error = %v", err)
	}
}

func moveIssueToReviewStateAsIntegrator(t *testing.T, svc *Service, ctx context.Context, issueRef string) {
	t.Helper()

	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-integrator",
		State:    "review",
		Body:     "Action: update\nStatus: review",
	}); err != nil {
		t.Fatalf("CommentIssue(review) error = %v", err)
	}
}

func TestLeadSyncOnceReviewerSkipsStateReviewWithoutToReviewer(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeReviewerWorkflow(t, nil)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "review state issue",
		Body:   "body",
		Labels: []string{"to:backend", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	claimIssueAsIntegrator(t, svc, ctx, issueRef)
	moveIssueToReviewStateAsIntegrator(t, svc, ctx, issueRef)

	before, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(before) error = %v", err)
	}

	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		t.Fatalf("worker should not be invoked, got issue=%s run=%s", input.IssueRef, input.RunID)
		return nil
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "reviewer",
		Assignee:     "lead-reviewer",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Processed != 0 || result.Spawned != 0 || result.Blocked != 0 {
		t.Fatalf("result = %+v", result)
	}

	after, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(after) error = %v", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("events len = %d, want %d", len(after.Events), len(before.Events))
	}
}

func TestLeadSyncOnceReviewerSkipsToReviewerWithoutStateReview(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeReviewerWorkflow(t, nil)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "to reviewer issue",
		Body:   "body",
		Labels: []string{"to:reviewer", "state:todo"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	claimIssueAsIntegrator(t, svc, ctx, issueRef)

	before, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(before) error = %v", err)
	}

	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		t.Fatalf("worker should not be invoked, got issue=%s run=%s", input.IssueRef, input.RunID)
		return nil
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "reviewer",
		Assignee:     "lead-reviewer",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Processed != 0 || result.Spawned != 0 || result.Blocked != 0 {
		t.Fatalf("result = %+v", result)
	}

	after, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(after) error = %v", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("events len = %d, want %d", len(after.Events), len(before.Events))
	}
}

func TestLeadSyncOnceReviewerSubscriberCommentOnlyDoesNotChangeAssigneeOrState(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeReviewerWorkflow(t, nil)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "reviewer subscriber issue",
		Body:   "body",
		Labels: []string{"to:reviewer", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	claimIssueAsIntegrator(t, svc, ctx, issueRef)
	moveIssueToReviewStateAsIntegrator(t, svc, ctx, issueRef)

	activeIssueRef := ""
	activeRunID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		activeIssueRef = input.IssueRef
		activeRunID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return successWorkResult(activeIssueRef, activeRunID), nil
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "reviewer",
		Assignee:     "lead-reviewer",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Processed != 1 || result.Spawned != 1 || result.Blocked != 0 {
		t.Fatalf("result = %+v", result)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if got.Assignee != "lead-integrator" {
		t.Fatalf("Assignee = %q, want lead-integrator", got.Assignee)
	}
	if !contains(got.Labels, "state:review") {
		t.Fatalf("labels = %v", got.Labels)
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "Role: reviewer") || !strings.Contains(last, "review:approved") {
		t.Fatalf("last event body = %s", last)
	}
}

func TestLeadSyncOnceReviewerChangesRequestedRoutesToBackend(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeReviewerWorkflow(t, nil)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "changes requested issue",
		Body:   "body",
		Labels: []string{"to:backend", "to:reviewer", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	claimIssueAsIntegrator(t, svc, ctx, issueRef)
	moveIssueToReviewStateAsIntegrator(t, svc, ctx, issueRef)

	activeIssueRef := ""
	activeRunID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		activeIssueRef = input.IssueRef
		activeRunID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		result := successWorkResult(activeIssueRef, activeRunID)
		result.ResultCode = "review_changes_requested"
		result.Tests.Result = "fail"
		return result, nil
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "reviewer",
		Assignee:     "lead-reviewer",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Candidates != 1 || result.Processed != 1 || result.Spawned != 1 || result.Blocked != 1 {
		t.Fatalf("result = %+v", result)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:review") {
		t.Fatalf("labels = %v", got.Labels)
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "review requested changes") || !strings.Contains(last, "@backend address review changes and rerun") {
		t.Fatalf("last event body = %s", last)
	}
}

func TestLeadSyncOnceReviewerChangesRequestedRoutesToFrontend(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeReviewerWorkflow(t, nil)
	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "changes requested frontend issue",
		Body:   "body",
		Labels: []string{"to:frontend", "to:reviewer", "state:review"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	claimIssueAsIntegrator(t, svc, ctx, issueRef)
	moveIssueToReviewStateAsIntegrator(t, svc, ctx, issueRef)

	activeIssueRef := ""
	activeRunID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		activeIssueRef = input.IssueRef
		activeRunID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		result := successWorkResult(activeIssueRef, activeRunID)
		result.ResultCode = "review_changes_requested"
		result.Tests.Result = "fail"
		return result, nil
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "reviewer",
		Assignee:     "lead-reviewer",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Candidates != 1 || result.Processed != 1 || result.Spawned != 1 || result.Blocked != 1 {
		t.Fatalf("result = %+v", result)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "@frontend address review changes and rerun") {
		t.Fatalf("last event body = %s", last)
	}
}

func TestLeadSyncOnceBackendRoutingKeepsANDSemantics(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeReviewerWorkflow(t, []string{"to:backend", "state:review"})
	if _, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  "backend route and issue",
		Body:   "body",
		Labels: []string{"to:backend"},
	}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Candidates != 0 {
		t.Fatalf("Candidates = %d, want 0", result.Candidates)
	}
}
