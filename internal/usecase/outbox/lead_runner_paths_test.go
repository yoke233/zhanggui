package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func createLeadClaimedIssue(t *testing.T, svc *Service, ctx context.Context, title string, body string, labels []string) string {
	t.Helper()

	issueRef, err := svc.CreateIssue(ctx, CreateIssueInput{
		Title:  title,
		Body:   body,
		Labels: labels,
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
	return issueRef
}

func successWorkResult(issueRef string, runID string) WorkResultEnvelope {
	return WorkResultEnvelope{
		IssueRef: issueRef,
		RunID:    runID,
		Changes: WorkResultChanges{
			PR:     "none",
			Commit: "git:abc123",
		},
		Tests: WorkResultTests{
			Command:  "go test ./...",
			Result:   "pass",
			Evidence: "none",
		},
	}
}

func TestLeadSyncOnceStaleRunSkipsWriteback(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "stale run issue", "body", []string{"to:backend", "state:todo"})

	before, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(before) error = %v", err)
	}

	runID := ""
	svc.workerInvoker = func(ctx context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return cache.Set(ctx, leadActiveRunKey("backend", issueRef), "other-run", 0)
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return WorkResultEnvelope{
			IssueRef: issueRef,
			RunID:    runID,
		}, nil
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
	if result.Processed != 1 {
		t.Fatalf("Processed = %d, want 1", result.Processed)
	}
	if result.Spawned != 1 {
		t.Fatalf("Spawned = %d, want 1", result.Spawned)
	}
	if result.Blocked != 0 {
		t.Fatalf("Blocked = %d, want 0", result.Blocked)
	}

	after, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(after) error = %v", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("events len = %d, want %d", len(after.Events), len(before.Events))
	}
}

func TestLeadSyncOnceEchoMismatchWritesBlocked(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "echo mismatch issue", "body", []string{"to:backend", "state:todo"})

	runID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return WorkResultEnvelope{
			IssueRef: "local#999",
			RunID:    runID,
		}, nil
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
	if result.Processed != 1 || result.Spawned != 1 || result.Blocked != 1 {
		t.Fatalf("result = %+v", result)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if !contains(got.Labels, "needs-human") {
		t.Fatalf("labels = %v", got.Labels)
	}
	found := false
	for _, evt := range got.Events {
		if strings.Contains(evt.Body, "work result echo validation failed") && strings.Contains(evt.Body, "work-result-echo") {
			found = true
			break
		}
	}
	if !found {
		last := got.Events[len(got.Events)-1].Body
		t.Fatalf("blocked comment not found; last event body = %s", last)
	}
}

func TestLeadSyncOnceWorkResultLoaderErrorAddsNeedsHuman(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "work result loader error issue", "body", []string{"to:backend", "state:todo"})

	runID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return WorkResultEnvelope{}, errors.New("boom")
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
	if result.Processed != 1 || result.Spawned != 1 || result.Blocked != 1 {
		t.Fatalf("result = %+v", result)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if !contains(got.Labels, "needs-human") {
		t.Fatalf("labels = %v", got.Labels)
	}
	found := false
	for _, evt := range got.Events {
		if strings.Contains(evt.Body, "worker result is missing or invalid") &&
			strings.Contains(evt.Body, "ResultCode: output_unparseable") &&
			strings.Contains(evt.Body, "workrun:"+runID) {
			found = true
			break
		}
	}
	if !found {
		last := got.Events[len(got.Events)-1].Body
		t.Fatalf("blocked comment not found; last event body = %s", last)
	}
}

func TestLeadSyncOnceMissingEvidenceWritesBlocked(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "missing evidence issue", "body", []string{"to:backend", "state:todo"})

	runID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return WorkResultEnvelope{
			IssueRef: issueRef,
			RunID:    runID,
			Changes: WorkResultChanges{
				PR:     "none",
				Commit: "none",
			},
			Tests: WorkResultTests{
				Command:  "go test ./...",
				Result:   "pass",
				Evidence: "none",
			},
		}, nil
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
	if result.Processed != 1 || result.Spawned != 1 || result.Blocked != 1 {
		t.Fatalf("result = %+v", result)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if !contains(got.Labels, "needs-human") {
		t.Fatalf("labels = %v", got.Labels)
	}
	found := false
	for _, evt := range got.Events {
		if strings.Contains(evt.Body, "missing required evidence") {
			found = true
			break
		}
	}
	if !found {
		last := got.Events[len(got.Events)-1].Body
		t.Fatalf("blocked comment not found; last event body = %s", last)
	}
}

func TestLeadSyncOnceMissingEvidenceTakesPrecedenceOverWorkerResultCode(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "missing evidence precedence issue", "body", []string{"to:backend", "state:todo"})

	runID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return WorkResultEnvelope{
			IssueRef:   issueRef,
			RunID:      runID,
			ResultCode: "test_failed",
			Changes: WorkResultChanges{
				PR:     "none",
				Commit: "none",
			},
			Tests: WorkResultTests{
				Command:  "go test ./...",
				Result:   "fail",
				Evidence: "none",
			},
		}, nil
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
	if result.Processed != 1 || result.Spawned != 1 || result.Blocked != 1 {
		t.Fatalf("result = %+v", result)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	if !contains(got.Labels, "needs-human") {
		t.Fatalf("labels = %v", got.Labels)
	}
	found := ""
	for _, evt := range got.Events {
		if strings.Contains(evt.Body, "missing required evidence") {
			found = evt.Body
			break
		}
	}
	if found == "" {
		last := got.Events[len(got.Events)-1].Body
		t.Fatalf("evidence-missing comment not found; last event body = %s", last)
	}
	if !strings.Contains(found, "ResultCode: manual_intervention") {
		t.Fatalf("expected manual_intervention result code, body=%s", found)
	}
	if strings.Contains(found, "worker reported result_code") {
		t.Fatalf("evidence-missing path should not be overridden by worker result_code, body=%s", found)
	}
}

func TestLeadSyncOnceResultCodeWritesBlocked(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "result code issue", "body", []string{"to:backend", "state:todo"})

	runID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return WorkResultEnvelope{
			IssueRef:   issueRef,
			RunID:      runID,
			ResultCode: "test_failed",
			Changes: WorkResultChanges{
				PR:     "none",
				Commit: "git:abc123",
			},
			Tests: WorkResultTests{
				Command:  "go test ./...",
				Result:   "fail",
				Evidence: "none",
			},
		}, nil
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
	if result.Processed != 1 || result.Spawned != 1 || result.Blocked != 1 {
		t.Fatalf("result = %+v", result)
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "worker reported result_code: test_failed") {
		t.Fatalf("last event body = %s", last)
	}
}

func TestLeadSyncOnceEventBatchOneCursorProgress(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "event batch issue", "body", []string{"to:backend", "state:todo"})
	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "doing",
		Body:     "Action: update\nStatus: doing",
	}); err != nil {
		t.Fatalf("CommentIssue(1) error = %v", err)
	}
	if err := svc.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    "lead-backend",
		State:    "doing",
		Body:     "Action: update\nStatus: doing",
	}); err != nil {
		t.Fatalf("CommentIssue(2) error = %v", err)
	}

	runID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return successWorkResult(issueRef, runID), nil
	}

	run1, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   1,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce(run1) error = %v", err)
	}
	if run1.Processed != 1 || run1.Spawned != 1 {
		t.Fatalf("run1 = %+v", run1)
	}
	if run1.CursorAfter != run1.CursorBefore+1 {
		t.Fatalf("run1 cursor should advance by 1, before=%d after=%d", run1.CursorBefore, run1.CursorAfter)
	}

	run2, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   1,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce(run2) error = %v", err)
	}
	if run2.CursorBefore != run1.CursorAfter || run2.CursorAfter != run2.CursorBefore+1 {
		t.Fatalf("run2 cursor mismatch: run1 after=%d run2 before=%d run2 after=%d", run1.CursorAfter, run2.CursorBefore, run2.CursorAfter)
	}

	run3, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   1,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce(run3) error = %v", err)
	}
	if run3.CursorBefore != run2.CursorAfter || run3.CursorAfter != run3.CursorBefore+1 {
		t.Fatalf("run3 cursor mismatch: run2 after=%d run3 before=%d run3 after=%d", run2.CursorAfter, run3.CursorBefore, run3.CursorAfter)
	}

	run4, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   1,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce(run4) error = %v", err)
	}
	if run4.CursorBefore != run3.CursorAfter || run4.CursorAfter != run4.CursorBefore+1 {
		t.Fatalf("run4 cursor mismatch: run3 after=%d run4 before=%d run4 after=%d", run3.CursorAfter, run4.CursorBefore, run4.CursorAfter)
	}

	run5, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   1,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce(run5) error = %v", err)
	}
	if run5.CursorBefore != run4.CursorAfter || run5.CursorAfter != run4.CursorAfter {
		t.Fatalf("run5 cursor should stay, run4 after=%d run5 before=%d run5 after=%d", run4.CursorAfter, run5.CursorBefore, run5.CursorAfter)
	}
	if cache.data[leadCursorKey("backend")] != "4" {
		t.Fatalf("cursor cache = %q, want 4", cache.data[leadCursorKey("backend")])
	}
}

func TestLeadSyncOnceDedupCandidatesByIssueID(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	_ = createLeadClaimedIssue(t, svc, ctx, "dedup issue", "body", []string{"to:backend", "state:todo", "needs-human"})

	result, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	})
	if err != nil {
		t.Fatalf("LeadSyncOnce() error = %v", err)
	}
	if result.Candidates != 1 {
		t.Fatalf("Candidates = %d, want 1", result.Candidates)
	}
	if result.Processed != 1 || result.Blocked != 1 || result.Spawned != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestLeadSyncOnceInvalidCachedCursorReturnsError(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflow(t)
	cache.data[leadCursorKey("backend")] = "invalid-cursor"

	if _, err := svc.LeadSyncOnce(ctx, LeadSyncInput{
		Role:         "backend",
		Assignee:     "lead-backend",
		WorkflowFile: workflowPath,
		EventBatch:   100,
	}); err == nil {
		t.Fatalf("LeadSyncOnce() expected error for invalid cached cursor")
	}
}
