package outbox

import (
	"context"
	"testing"
)

func TestLeadRunIssueOnceSpawnsWorker(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "lead run issue once", "body", []string{"to:backend", "state:todo"})

	runID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return successWorkResult(issueRef, runID), nil
	}

	result, err := svc.LeadRunIssueOnce(ctx, LeadRunIssueInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		IssueRef:     issueRef,
		WorkflowFile: workflowPath,
	})
	if err != nil {
		t.Fatalf("LeadRunIssueOnce() error = %v", err)
	}
	if !result.Processed || !result.Spawned || result.Blocked {
		t.Fatalf("result = %+v, want processed+spawned and not blocked", result)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:review") {
		t.Fatalf("labels = %v, want contains state:review", got.Labels)
	}
}

func TestLeadRunIssueOnceForceSpawnBypassesStateSkip(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "lead run force spawn", "body", []string{"to:backend", "state:todo"})
	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "blocked",
		Body:     "manual blocked",
	}); err != nil {
		t.Fatalf("CommentIssue(blocked) error = %v", err)
	}

	runID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return successWorkResult(issueRef, runID), nil
	}

	normalResult, err := svc.LeadRunIssueOnce(ctx, LeadRunIssueInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		IssueRef:     issueRef,
		WorkflowFile: workflowPath,
	})
	if err != nil {
		t.Fatalf("LeadRunIssueOnce(normal) error = %v", err)
	}
	if normalResult.Processed || normalResult.Spawned || normalResult.Blocked {
		t.Fatalf("normal result = %+v, want skipped", normalResult)
	}

	forceResult, err := svc.LeadRunIssueOnce(ctx, LeadRunIssueInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		IssueRef:     issueRef,
		WorkflowFile: workflowPath,
		ForceSpawn:   true,
	})
	if err != nil {
		t.Fatalf("LeadRunIssueOnce(force) error = %v", err)
	}
	if !forceResult.Processed || !forceResult.Spawned || forceResult.Blocked {
		t.Fatalf("force result = %+v, want processed+spawned and not blocked", forceResult)
	}
}
