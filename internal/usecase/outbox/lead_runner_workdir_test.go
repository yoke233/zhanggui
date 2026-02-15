package outbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubWorkdirManager struct {
	preparePath  string
	prepareErr   error
	cleanupErr   error
	prepareCalls int
	cleanupCalls int
	lastRunID    string
}

func (m *stubWorkdirManager) Prepare(_ context.Context, _ string, _ string, runID string) (string, error) {
	m.prepareCalls++
	m.lastRunID = runID
	if m.prepareErr != nil {
		return "", m.prepareErr
	}
	return m.preparePath, nil
}

func (m *stubWorkdirManager) Cleanup(_ context.Context, _ string, _ string, runID string, _ string) error {
	m.cleanupCalls++
	m.lastRunID = runID
	return m.cleanupErr
}

func writeTestWorkflowWithWorkdir(t *testing.T) string {
	t.Helper()

	content := `
version = 2

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[workdir]
enabled = true
backend = "git-worktree"
root = "/.worktrees/runs"
cleanup = "immediate"
roles = ["backend"]

[roles]
enabled = ["backend"]

[repos]
main = "."

[role_repo]
backend = "main"

[groups.backend]
role = "backend"
max_concurrent = 4
mode = "owner"
writeback = "full"
listen_labels = ["to:backend"]

[executors.backend]
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

func TestLeadSyncOnceWorkdirPrepareFailureBlocksWithoutSpawningWorker(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflowWithWorkdir(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "workdir prepare fail", "body", []string{"to:backend", "state:todo"})

	workdirStub := &stubWorkdirManager{
		prepareErr: errors.New("prepare failed"),
	}
	svc.workdirFactory = func(_ workflowWorkdirConfig, _ string, _ string) (workdirManager, error) {
		return workdirStub, nil
	}

	workerCalled := false
	svc.workerInvoker = func(_ context.Context, _ invokeWorkerInput) error {
		workerCalled = true
		return nil
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
	if result.Processed != 1 || result.Blocked != 1 || result.Spawned != 0 {
		t.Fatalf("result = %+v", result)
	}
	if workerCalled {
		t.Fatalf("worker should not be invoked when workdir prepare fails")
	}

	got, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if !contains(got.Labels, "state:blocked") {
		t.Fatalf("labels = %v", got.Labels)
	}
	last := got.Events[len(got.Events)-1].Body
	if !strings.Contains(last, "workdir prepare failed") || !strings.Contains(last, "workdir-prepare") {
		t.Fatalf("last event body = %s", last)
	}
}

func TestLeadSyncOnceWorkdirCleanupFailureBlocksAndKeepsEvidence(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflowWithWorkdir(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "workdir cleanup fail", "body", []string{"to:backend", "state:todo"})

	runID := ""
	svc.workerInvoker = func(_ context.Context, input invokeWorkerInput) error {
		runID = input.RunID
		return nil
	}
	svc.workResultLoader = func(_ string) (WorkResultEnvelope, error) {
		return successWorkResult(issueRef, runID), nil
	}

	workdirPath := filepath.Join(filepath.Dir(workflowPath), ".worktrees", "runs", "backend", "local_1", "run")
	workdirStub := &stubWorkdirManager{
		preparePath: workdirPath,
		cleanupErr:  errors.New("dirty workdir"),
	}
	svc.workdirFactory = func(_ workflowWorkdirConfig, _ string, _ string) (workdirManager, error) {
		return workdirStub, nil
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
	if result.Processed != 1 || result.Blocked != 1 || result.Spawned != 1 {
		t.Fatalf("result = %+v", result)
	}

	contextPackDir := leadContextPackDir(issueRef, runID)
	order, err := loadWorkOrder(filepath.Join(contextPackDir, "work_order.json"))
	if err != nil {
		t.Fatalf("loadWorkOrder() error = %v", err)
	}
	if order.RepoDir != workdirPath {
		t.Fatalf("work order repo dir = %q, want %q", order.RepoDir, workdirPath)
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
		if strings.Contains(evt.Body, "workdir cleanup failed") {
			found = evt.Body
			break
		}
	}
	if found == "" {
		last := got.Events[len(got.Events)-1].Body
		t.Fatalf("cleanup failure comment not found; last event body = %s", last)
	}
	if !strings.Contains(found, "workdir cleanup failed") || !strings.Contains(found, "workdir-cleanup") {
		t.Fatalf("cleanup failure comment body = %s", found)
	}
	if !strings.Contains(found, "git:abc123") {
		t.Fatalf("cleanup failure comment should keep evidence, body=%s", found)
	}
}

func TestLeadSyncOnceWorkerExecutionCleanupFailureAddsNeedsHuman(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflowWithWorkdir(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "worker execution cleanup fail", "body", []string{"to:backend", "state:todo"})

	workdirPath := filepath.Join(filepath.Dir(workflowPath), ".worktrees", "runs", "backend", "local_1", "run")
	workdirStub := &stubWorkdirManager{
		preparePath: workdirPath,
		cleanupErr:  errors.New("dirty workdir"),
	}
	svc.workdirFactory = func(_ workflowWorkdirConfig, _ string, _ string) (workdirManager, error) {
		return workdirStub, nil
	}

	svc.workerInvoker = func(_ context.Context, _ invokeWorkerInput) error {
		return errors.New("executor failed")
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
	if result.Processed != 1 || result.Blocked != 1 || result.Spawned != 1 {
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
		if strings.Contains(evt.Body, "worker execution failed") {
			found = evt.Body
			break
		}
	}
	if found == "" {
		last := got.Events[len(got.Events)-1].Body
		t.Fatalf("worker execution failure comment not found; last event body = %s", last)
	}
	if !strings.Contains(found, "worker execution failed") || !strings.Contains(found, "workdir cleanup failed") {
		t.Fatalf("worker execution failure comment body = %s", found)
	}
}

func TestLeadSyncOnceStaleRunStillCleansUpWorkdir(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	workflowPath := writeTestWorkflowWithWorkdir(t)
	issueRef := createLeadClaimedIssue(t, svc, ctx, "workdir stale cleanup", "body", []string{"to:backend", "state:todo"})

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

	workdirStub := &stubWorkdirManager{
		preparePath: filepath.Join(filepath.Dir(workflowPath), ".worktrees", "runs", "backend", "local_1", "run"),
	}
	svc.workdirFactory = func(_ workflowWorkdirConfig, _ string, _ string) (workdirManager, error) {
		return workdirStub, nil
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
	if result.Processed != 1 || result.Spawned != 1 || result.Blocked != 0 {
		t.Fatalf("result = %+v", result)
	}
	if workdirStub.cleanupCalls != 1 {
		t.Fatalf("cleanupCalls = %d, want 1", workdirStub.cleanupCalls)
	}

	after, err := svc.GetIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("GetIssue(after) error = %v", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("events len = %d, want %d", len(after.Events), len(before.Events))
	}
}
