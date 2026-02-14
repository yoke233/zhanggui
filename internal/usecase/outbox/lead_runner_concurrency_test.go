package outbox

import (
	"context"
	"fmt"
	"testing"
)

func writeLeadWorkflowWithMaxConcurrent(t *testing.T, maxConcurrent int) string {
	t.Helper()

	content := fmt.Sprintf(`
version = 1

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[roles]
enabled = ["backend"]

[repos]
main = "."

[role_repo]
backend = "main"

[groups.backend]
role = "backend"
max_concurrent = %d
listen_labels = ["to:backend"]
`, maxConcurrent)
	return writeLeadWorkflowFile(t, content)
}

func TestLeadSyncOnceRespectsMaxConcurrentLimit(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeLeadWorkflowWithMaxConcurrent(t, 1)
	issueRef1 := createLeadClaimedIssue(t, svc, ctx, "limit issue 1", "body", []string{"to:backend", "state:todo"})
	issueRef2 := createLeadClaimedIssue(t, svc, ctx, "limit issue 2", "body", []string{"to:backend", "state:todo"})

	activeIssueRef := ""
	activeRunID := ""
	workerCalls := 0
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		workerCalls++
		activeIssueRef = input.IssueRef
		activeRunID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return successWorkResult(activeIssueRef, activeRunID), nil
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
	if workerCalls != 1 {
		t.Fatalf("worker calls = %d, want 1", workerCalls)
	}
	if result.Candidates != 2 || result.Spawned != 1 || result.Processed != 1 || result.Skipped != 1 {
		t.Fatalf("result = %+v", result)
	}

	issue1, err := svc.GetIssue(ctx, issueRef1)
	if err != nil {
		t.Fatalf("GetIssue(issue1) error = %v", err)
	}
	issue2, err := svc.GetIssue(ctx, issueRef2)
	if err != nil {
		t.Fatalf("GetIssue(issue2) error = %v", err)
	}
	if !contains(issue1.Labels, "state:review") {
		t.Fatalf("issue1 labels = %v", issue1.Labels)
	}
	if !contains(issue2.Labels, "state:doing") {
		t.Fatalf("issue2 labels = %v", issue2.Labels)
	}
}

func TestLeadSyncOnceMaxConcurrentZeroDefaultsToOne(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeLeadWorkflowWithMaxConcurrent(t, 0)
	_ = createLeadClaimedIssue(t, svc, ctx, "default-one issue 1", "body", []string{"to:backend", "state:todo"})
	_ = createLeadClaimedIssue(t, svc, ctx, "default-one issue 2", "body", []string{"to:backend", "state:todo"})

	activeIssueRef := ""
	activeRunID := ""
	workerCalls := 0
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		workerCalls++
		activeIssueRef = input.IssueRef
		activeRunID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return successWorkResult(activeIssueRef, activeRunID), nil
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
	if workerCalls != 1 {
		t.Fatalf("worker calls = %d, want 1", workerCalls)
	}
	if result.Spawned != 1 {
		t.Fatalf("Spawned = %d, want 1", result.Spawned)
	}
}

func TestLeadSyncOnceMaxConcurrentAllowsMultiple(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeLeadWorkflowWithMaxConcurrent(t, 2)
	issueRef1 := createLeadClaimedIssue(t, svc, ctx, "multi issue 1", "body", []string{"to:backend", "state:todo"})
	issueRef2 := createLeadClaimedIssue(t, svc, ctx, "multi issue 2", "body", []string{"to:backend", "state:todo"})

	activeIssueRef := ""
	activeRunID := ""
	workerCalls := 0
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		workerCalls++
		activeIssueRef = input.IssueRef
		activeRunID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return successWorkResult(activeIssueRef, activeRunID), nil
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
	if workerCalls != 2 {
		t.Fatalf("worker calls = %d, want 2", workerCalls)
	}
	if result.Spawned != 2 || result.Processed != 2 || result.Skipped != 0 {
		t.Fatalf("result = %+v", result)
	}

	issue1, err := svc.GetIssue(ctx, issueRef1)
	if err != nil {
		t.Fatalf("GetIssue(issue1) error = %v", err)
	}
	issue2, err := svc.GetIssue(ctx, issueRef2)
	if err != nil {
		t.Fatalf("GetIssue(issue2) error = %v", err)
	}
	if !contains(issue1.Labels, "state:review") || !contains(issue2.Labels, "state:review") {
		t.Fatalf("labels issue1=%v issue2=%v", issue1.Labels, issue2.Labels)
	}
}

func TestMaxConcurrentForTick(t *testing.T) {
	testCases := []struct {
		name  string
		input int
		want  int
	}{
		{name: "positive", input: 3, want: 3},
		{name: "zero defaults one", input: 0, want: 1},
		{name: "negative defaults one", input: -1, want: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := maxConcurrentForTick(testCase.input)
			if got != testCase.want {
				t.Fatalf("maxConcurrentForTick(%d) = %d, want %d", testCase.input, got, testCase.want)
			}
		})
	}
}
