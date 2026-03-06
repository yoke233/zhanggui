package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	"github.com/yoke233/ai-workflow/internal/eventbus"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

// e2eStageResults provides ordered results for testStageFunc.
type e2eStageResults struct {
	mu      sync.Mutex
	results []error
	calls   int
}

func (r *e2eStageResults) next() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := r.calls
	r.calls++
	if idx < len(r.results) {
		return r.results[idx]
	}
	return nil
}

// ---------------------------------------------------------------------------
// fake types (scoped to this test file)
// ---------------------------------------------------------------------------

type e2eFakeWorkspace struct{}

func (e2eFakeWorkspace) Name() string                    { return "fake-workspace" }
func (e2eFakeWorkspace) Init(context.Context) error      { return nil }
func (e2eFakeWorkspace) Close() error                    { return nil }
func (e2eFakeWorkspace) Setup(_ context.Context, req core.WorkspaceSetupRequest) (core.WorkspaceSetupResult, error) {
	return core.WorkspaceSetupResult{
		BranchName:   "ai-flow/" + req.RunID,
		WorktreePath: req.RepoPath,
		BaseBranch:   "main",
	}, nil
}
func (e2eFakeWorkspace) Cleanup(_ context.Context, _ core.WorkspaceCleanupRequest) error {
	return nil
}

// depSchedulerIssueAdapter mirrors cmd/ai-flow/commands.go pattern.
type e2eDepSchedulerIssueAdapter struct {
	scheduler *teamleader.DepScheduler
}

func (a *e2eDepSchedulerIssueAdapter) Start(ctx context.Context) error {
	return a.scheduler.Start(ctx)
}
func (a *e2eDepSchedulerIssueAdapter) Stop(ctx context.Context) error {
	return a.scheduler.Stop(ctx)
}
func (a *e2eDepSchedulerIssueAdapter) RecoverExecutingIssues(ctx context.Context) error {
	return a.scheduler.RecoverExecutingIssues(ctx, "")
}
func (a *e2eDepSchedulerIssueAdapter) StartIssue(ctx context.Context, issue *core.Issue) error {
	return a.scheduler.ScheduleIssues(ctx, []*core.Issue{issue})
}

// fakeReviewSubmitter satisfies managerIssueReviewSubmitter (no-op).
type e2eFakeReviewSubmitter struct{}

func (e2eFakeReviewSubmitter) Submit(_ context.Context, _ []*core.Issue) error { return nil }

// ---------------------------------------------------------------------------
// stack setup
// ---------------------------------------------------------------------------

type e2aStack struct {
	ts       *httptest.Server
	store    *storesqlite.SQLiteStore
	executor *engine.Executor
	bus      *eventbus.MemoryBus
}

func setupE2AA2AStack(t *testing.T, stageResults *e2eStageResults) e2aStack {
	t.Helper()

	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}

	project := &core.Project{
		ID:       "proj-e2e",
		Name:     "e2e-proj",
		RepoPath: t.TempDir(),
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatal(err)
	}

	bus := eventbus.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	exec := engine.NewExecutor(store, bus, logger)
	exec.TestSetStageFunc(func(ctx context.Context, runID string, stage core.StageID, agentName, prompt string) error {
		return stageResults.next()
	})
	exec.SetWorkspace(e2eFakeWorkspace{})
	exec.SetRoleResolver(acpclient.NewRoleResolver(
		[]acpclient.AgentProfile{{
			ID: "codex",
			CapabilitiesMax: acpclient.ClientCapabilities{
				FSRead: true, FSWrite: true, Terminal: true,
			},
		}},
		[]acpclient.RoleProfile{
			{ID: "worker", AgentID: "codex", Capabilities: acpclient.ClientCapabilities{FSRead: true, FSWrite: true, Terminal: true}},
			{ID: "reviewer", AgentID: "codex", Capabilities: acpclient.ClientCapabilities{FSRead: true, FSWrite: true, Terminal: true}},
		},
	))

	// Override template to simplify stages: setup, implement, review, cleanup.
	// Save original and restore after test.
	origTemplate := engine.Templates["standard"]
	engine.Templates["standard"] = []core.StageID{
		core.StageSetup, core.StageImplement, core.StageReview, core.StageCleanup,
	}
	t.Cleanup(func() { engine.Templates["standard"] = origTemplate })

	depScheduler := teamleader.NewDepScheduler(
		store, bus,
		func(ctx context.Context, runID string) error {
			ok, markErr := store.TryMarkRunInProgress(runID, core.StatusQueued)
			if markErr != nil {
				return markErr
			}
			if !ok {
				return fmt.Errorf("run %s not markable", runID)
			}
			return exec.RunScheduled(ctx, runID)
		},
		nil, // no tracker
		1,   // max concurrent
	)
	depScheduler.SetStageRoles(map[string]string{
		"implement": "worker",
		"review":    "reviewer",
	})

	adapter := &e2eDepSchedulerIssueAdapter{scheduler: depScheduler}
	manager, err := teamleader.NewManager(store, nil, e2eFakeReviewSubmitter{}, adapter)
	if err != nil {
		t.Fatal(err)
	}

	// Start the scheduler event loop so RunDone events update issue status.
	if err := depScheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Simulate merge completion: when issue enters merging state, publish EventIssueMerged.
	mergeSub, subErr := bus.Subscribe()
	if subErr != nil {
		t.Fatal(subErr)
	}
	mergeCtx, mergeCancel := context.WithCancel(context.Background())
	go func() {
		defer mergeSub.Unsubscribe()
		for {
			select {
			case <-mergeCtx.Done():
				return
			case evt, ok := <-mergeSub.C:
				if !ok {
					return
				}
				if evt.Type == core.EventIssueMerging {
					bus.Publish(context.Background(), core.Event{
						Type:      core.EventIssueMerged,
						IssueID:   evt.IssueID,
						RunID:     evt.RunID,
						ProjectID: evt.ProjectID,
						Timestamp: time.Now(),
					})
				}
			}
		}
	}()
	t.Cleanup(func() { mergeCancel() })

	bridge, err := teamleader.NewA2ABridge(store, manager)
	if err != nil {
		t.Fatal(err)
	}

	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "test-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
		Store:      store,
		RunExec:    exec,
	})
	ts := httptest.NewServer(srv.Handler())

	t.Cleanup(func() {
		ts.Close()
		_ = adapter.Stop(context.Background())
		store.Close()
	})

	return e2aStack{
		ts:       ts,
		store:    store,
		executor: exec,
		bus:      bus,
	}
}

// ---------------------------------------------------------------------------
// polling helpers
// ---------------------------------------------------------------------------

func pollA2ATaskState(t *testing.T, baseURL, token, taskID string, wantState string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	reqBody := fmt.Sprintf(`{
		"jsonrpc":"2.0","id":"poll-1","method":"tasks/get",
		"params":{"id":%q,"metadata":{"project_id":"proj-e2e"}}
	}`, taskID)

	for time.Now().Before(deadline) {
		payload := mustDoA2ARPCRequest(t, baseURL, reqBody, token)
		result, ok := payload["result"].(map[string]any)
		if !ok {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		status, ok := result["status"].(map[string]any)
		if !ok {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		state, _ := status["state"].(string)
		if state == wantState {
			return result
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for A2A task %s to reach state %q", taskID, wantState)
	return nil
}

func pollRunStatus(t *testing.T, store core.Store, issueID string, wantStatus core.RunStatus, timeout time.Duration) *core.Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		issue, err := store.GetIssue(issueID)
		if err != nil || strings.TrimSpace(issue.RunID) == "" {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		run, err := store.GetRun(issue.RunID)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if run.Status == wantStatus {
			return run
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for run (issue %s) to reach status %q", issueID, wantStatus)
	return nil
}

func sendA2AMessage(t *testing.T, baseURL, token, message string) (taskID string) {
	t.Helper()
	reqBody := fmt.Sprintf(`{
		"jsonrpc":"2.0","id":"send-1","method":"message/send",
		"params":{
			"message":{
				"messageId":"m-1","role":"user",
				"parts":[{"kind":"text","text":%s}]
			},
			"metadata":{"project_id":"proj-e2e"}
		}
	}`, mustMarshalString(message))

	payload := mustDoA2ARPCRequest(t, baseURL, reqBody, token)
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload)
	}
	id, _ := result["id"].(string)
	if id == "" {
		t.Fatalf("expected non-empty task id in result, got %#v", result)
	}
	return id
}

func mustMarshalString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func getIssueIDFromTask(t *testing.T, taskID string) string {
	// task ID == issue ID in A2ABridge
	return taskID
}

// ---------------------------------------------------------------------------
// Test 1: Review success → Run completed → A2A completed
// ---------------------------------------------------------------------------

func TestE2E_A2A_ReviewSuccess_RunCompleted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// All agent stages succeed.
	stack := setupE2AA2AStack(t, &e2eStageResults{})

	// Send A2A message.
	taskID := sendA2AMessage(t, stack.ts.URL, "test-token", "implement feature X")

	// Poll until A2A task reaches "completed".
	result := pollA2ATaskState(t, stack.ts.URL, "test-token", taskID, string(teamleader.A2ATaskStateCompleted), 15*time.Second)

	// Verify A2A task state.
	status := result["status"].(map[string]any)
	if status["state"] != string(teamleader.A2ATaskStateCompleted) {
		t.Fatalf("expected A2A state completed, got %v", status["state"])
	}

	// Verify run state.
	issueID := getIssueIDFromTask(t, taskID)
	issue, err := stack.store.GetIssue(issueID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if issue.Status != core.IssueStatusDone {
		t.Fatalf("expected issue status done, got %s", issue.Status)
	}
	if strings.TrimSpace(issue.RunID) == "" {
		t.Fatal("expected issue to have a run ID")
	}

	run, err := stack.store.GetRun(issue.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != core.StatusCompleted {
		t.Fatalf("expected run status completed, got %s", run.Status)
	}
	if run.Conclusion != core.ConclusionSuccess {
		t.Fatalf("expected run conclusion success, got %s", run.Conclusion)
	}

	// Verify agent stages have success checkpoints.
	checkpoints, err := stack.store.GetCheckpoints(run.ID)
	if err != nil {
		t.Fatalf("get checkpoints: %v", err)
	}
	successByStage := map[core.StageID]bool{}
	for _, cp := range checkpoints {
		if cp.Status == core.CheckpointSuccess {
			successByStage[cp.StageName] = true
		}
	}
	for _, wantStage := range []core.StageID{core.StageImplement, core.StageReview} {
		if !successByStage[wantStage] {
			t.Fatalf("expected success checkpoint for stage %s", wantStage)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 2: Review failure → action_required → reject → verify state
// ---------------------------------------------------------------------------

func TestE2E_A2A_ReviewReject_ActionRequired(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// implement succeeds, review fails (triggers human escalation via OnFailure=human).
	// setup/cleanup have no agent runtime calls.
	// Agent stages in order: implement(1st call), review(2nd call).
	stack := setupE2AA2AStack(t, &e2eStageResults{
		results: []error{
			nil,                            // implement succeeds
			errors.New("review-rejection"), // review fails
		},
	})

	// Send A2A message.
	taskID := sendA2AMessage(t, stack.ts.URL, "test-token", "implement feature Y")
	issueID := getIssueIDFromTask(t, taskID)

	// Wait for run to enter action_required.
	run := pollRunStatus(t, stack.store, issueID, core.StatusActionRequired, 15*time.Second)
	if run.CurrentStage != core.StageReview {
		t.Fatalf("expected current stage review, got %s", run.CurrentStage)
	}

	// Apply reject action on implement stage.
	rejectErr := stack.executor.ApplyAction(context.Background(), core.RunAction{
		RunID:   run.ID,
		Type:    core.ActionReject,
		Stage:   core.StageImplement,
		Message: "代码质量不达标",
	})
	if rejectErr != nil {
		t.Fatalf("apply reject: %v", rejectErr)
	}

	// Verify run state after reject.
	run, err := stack.store.GetRun(run.ID)
	if err != nil {
		t.Fatalf("get run after reject: %v", err)
	}
	if run.Status != core.StatusActionRequired {
		t.Fatalf("expected run status action_required after reject, got %s", run.Status)
	}
	if run.ErrorMessage != "代码质量不达标" {
		t.Fatalf("expected error message '代码质量不达标', got %q", run.ErrorMessage)
	}

	// Verify implement checkpoint was invalidated.
	checkpoints, err := stack.store.GetCheckpoints(run.ID)
	if err != nil {
		t.Fatalf("get checkpoints: %v", err)
	}
	foundInvalidated := false
	for _, cp := range checkpoints {
		if cp.StageName == core.StageImplement && cp.Status == core.CheckpointInvalidated {
			foundInvalidated = true
		}
	}
	if !foundInvalidated {
		t.Fatal("expected implement checkpoint to be invalidated after reject")
	}

	// Verify A2A task is still "working" (issue is executing, not done).
	a2aReqBody := fmt.Sprintf(`{
		"jsonrpc":"2.0","id":"check-1","method":"tasks/get",
		"params":{"id":%q,"metadata":{"project_id":"proj-e2e"}}
	}`, taskID)
	payload := mustDoA2ARPCRequest(t, stack.ts.URL, a2aReqBody, "test-token")
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result, got %#v", payload)
	}
	status, ok := result["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected status, got %#v", result)
	}
	state, _ := status["state"].(string)
	// Issue is still executing (run is action_required but not failed/done),
	// so A2A state should be "working".
	if state != string(teamleader.A2ATaskStateWorking) {
		t.Fatalf("expected A2A task state working, got %q", state)
	}
}
