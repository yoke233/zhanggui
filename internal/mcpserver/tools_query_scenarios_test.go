package mcpserver_test

import (
	"encoding/json"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
)

// Scenario 1: TeamLeader diagnoses failed runs
// Flow: query_runs(conclusion=failure) → query_run_detail → query_run_events(event_type=stage_failed)
func TestScenario_DiagnoseFailedRuns(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r-ok", "p1", core.StatusCompleted, core.ConclusionSuccess)
	seedRun(t, store, "r-fail", "p1", core.StatusCompleted, core.ConclusionFailure)
	// Add events to the failed run
	store.SaveRunEvent(core.RunEvent{RunID: "r-fail", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: "implement"})
	store.SaveRunEvent(core.RunEvent{RunID: "r-fail", ProjectID: "p1", EventType: string(core.EventStageFailed), Stage: "implement", Error: "compilation error"})
	store.SaveRunEvent(core.RunEvent{RunID: "r-fail", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: "test"})
	store.SaveRunEvent(core.RunEvent{RunID: "r-fail", ProjectID: "p1", EventType: string(core.EventStageFailed), Stage: "test", Error: "test timeout"})

	session := setupTestClient(t, store)

	// Step 1: Find failed runs
	res := callTool(t, session, "query_runs", map[string]any{
		"project_id": "p1",
		"conclusion": "failure",
	})
	var failedRuns []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &failedRuns); err != nil {
		t.Fatal(err)
	}
	if len(failedRuns) != 1 || failedRuns[0].ID != "r-fail" {
		t.Fatalf("expected 1 failed run (r-fail), got %d runs", len(failedRuns))
	}

	// Step 2: Get run detail
	res = callTool(t, session, "query_run_detail", map[string]any{"run_id": "r-fail"})
	var detail mcpserver.RunDetail
	if err := json.Unmarshal([]byte(resultText(t, res)), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Run.Conclusion != core.ConclusionFailure {
		t.Errorf("expected failure conclusion, got %s", detail.Run.Conclusion)
	}

	// Step 3: Get only failed events
	res = callTool(t, session, "query_run_events", map[string]any{
		"run_id":     "r-fail",
		"event_type": string(core.EventStageFailed),
	})
	var failedEvents []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &failedEvents); err != nil {
		t.Fatal(err)
	}
	if len(failedEvents) != 2 {
		t.Fatalf("expected 2 stage_failed events, got %d", len(failedEvents))
	}
	for _, e := range failedEvents {
		if e.Error == "" {
			t.Error("expected non-empty error in failed event")
		}
	}
}

// Scenario 2: Track an issue through its execution pipeline
// Flow: query_issue_detail → query_runs(issue_id) → query_run_events(stage)
func TestScenario_TrackIssueExecution(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Add auth", "feature", core.IssueStateOpen, core.IssueStatusExecuting)
	// Run associated with this issue
	store.SaveRun(&core.Run{
		ID: "r1", ProjectID: "p1", Name: "run-i1", Template: "default",
		Status: core.StatusInProgress, IssueID: "i1",
	})
	store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: "implement"})
	store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageComplete), Stage: "implement"})
	store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: "review"})
	store.SaveCheckpoint(&core.Checkpoint{
		RunID: "r1", StageName: core.StageImplement, Status: core.CheckpointSuccess,
	})

	session := setupTestClient(t, store)

	// Step 1: Get issue detail
	res := callTool(t, session, "query_issue_detail", map[string]any{"issue_id": "i1"})
	var issueDetail mcpserver.IssueDetail
	if err := json.Unmarshal([]byte(resultText(t, res)), &issueDetail); err != nil {
		t.Fatal(err)
	}
	if issueDetail.Issue.Status != core.IssueStatusExecuting {
		t.Errorf("expected running, got %s", issueDetail.Issue.Status)
	}

	// Step 2: Find runs for this issue
	res = callTool(t, session, "query_runs", map[string]any{
		"project_id": "p1",
		"issue_id":   "i1",
	})
	var runs []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "r1" {
		t.Fatalf("expected 1 run r1, got %d", len(runs))
	}

	// Step 3: Get implement stage events
	res = callTool(t, session, "query_run_events", map[string]any{
		"run_id": "r1",
		"stage":  "implement",
	})
	var events []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 implement events, got %d", len(events))
	}

	// Step 4: Verify checkpoint
	res = callTool(t, session, "query_run_detail", map[string]any{"run_id": "r1"})
	var runDetail mcpserver.RunDetail
	if err := json.Unmarshal([]byte(resultText(t, res)), &runDetail); err != nil {
		t.Fatal(err)
	}
	if len(runDetail.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(runDetail.Checkpoints))
	}
	if runDetail.Checkpoints[0].Status != core.CheckpointSuccess {
		t.Errorf("expected success checkpoint, got %s", runDetail.Checkpoints[0].Status)
	}
}

// Scenario 3: Project health overview
// Flow: query_project_stats → query_issues(state=open) → query_runs(status=in_progress)
func TestScenario_ProjectHealthOverview(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	seedIssue(t, store, "i2", "p1", "Bug2", "bugfix", core.IssueStateClosed, core.IssueStatusDone)
	seedRun(t, store, "r1", "p1", core.StatusCompleted, core.ConclusionSuccess)
	seedRun(t, store, "r2", "p1", core.StatusCompleted, core.ConclusionFailure)
	seedRun(t, store, "r3", "p1", core.StatusInProgress, "")

	session := setupTestClient(t, store)

	// Step 1: Overview stats
	res := callTool(t, session, "query_project_stats", map[string]any{"project_id": "p1"})
	var stats mcpserver.ProjectStats
	if err := json.Unmarshal([]byte(resultText(t, res)), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.TotalIssues != 2 {
		t.Errorf("total issues: want 2, got %d", stats.TotalIssues)
	}
	if stats.OpenIssues != 1 {
		t.Errorf("open issues: want 1, got %d", stats.OpenIssues)
	}
	if stats.TotalRuns != 3 {
		t.Errorf("total runs: want 3, got %d", stats.TotalRuns)
	}
	if stats.CompletedRuns != 2 {
		t.Errorf("completed runs: want 2, got %d", stats.CompletedRuns)
	}
	if stats.SuccessRate != 0.5 {
		t.Errorf("success rate: want 0.5, got %f", stats.SuccessRate)
	}

	// Step 2: Drill into open issues
	res = callTool(t, session, "query_issues", map[string]any{
		"project_id": "p1",
		"state":      "open",
	})
	var openIssues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &openIssues); err != nil {
		t.Fatal(err)
	}
	if len(openIssues) != 1 {
		t.Fatalf("expected 1 open issue, got %d", len(openIssues))
	}

	// Step 3: Check active runs
	res = callTool(t, session, "query_runs", map[string]any{
		"project_id": "p1",
		"status":     "in_progress",
	})
	var activeRuns []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &activeRuns); err != nil {
		t.Fatal(err)
	}
	if len(activeRuns) != 1 {
		t.Fatalf("expected 1 active run, got %d", len(activeRuns))
	}
}

// Scenario 4: Filter issues by chat session to find what was discussed
func TestScenario_FilterIssuesBySession(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	store.CreateChatSession(&core.ChatSession{ID: "sess-a", ProjectID: "p1"})
	store.CreateChatSession(&core.ChatSession{ID: "sess-b", ProjectID: "p1"})
	store.CreateIssue(&core.Issue{
		ID: "i1", ProjectID: "p1", Title: "From session A", Template: "bugfix",
		State: core.IssueStateOpen, Status: core.IssueStatusReady, SessionID: "sess-a",
	})
	store.CreateIssue(&core.Issue{
		ID: "i2", ProjectID: "p1", Title: "From session B", Template: "feature",
		State: core.IssueStateOpen, Status: core.IssueStatusReady, SessionID: "sess-b",
	})
	store.CreateIssue(&core.Issue{
		ID: "i3", ProjectID: "p1", Title: "Also session A", Template: "bugfix",
		State: core.IssueStateClosed, Status: core.IssueStatusDone, SessionID: "sess-a",
	})

	session := setupTestClient(t, store)

	// Default state=open: only open issues are returned.
	res := callTool(t, session, "query_issues", map[string]any{
		"project_id": "p1",
		"session_id": "sess-a",
	})
	var openIssues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &openIssues); err != nil {
		t.Fatal(err)
	}
	if len(openIssues) != 1 {
		t.Fatalf("expected 1 open issue from sess-a, got %d", len(openIssues))
	}

	// state=all returns both open and closed.
	res = callTool(t, session, "query_issues", map[string]any{
		"project_id": "p1",
		"session_id": "sess-a",
		"state":      "all",
	})
	var allIssues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &allIssues); err != nil {
		t.Fatal(err)
	}
	if len(allIssues) != 2 {
		t.Fatalf("expected 2 issues from sess-a with state=all, got %d", len(allIssues))
	}
	for _, iss := range allIssues {
		if iss.SessionID != "sess-a" {
			t.Errorf("expected session_id=sess-a, got %s", iss.SessionID)
		}
	}
}

// Scenario 5: Filter events by type and stage, then limit results
func TestScenario_FilterEventsByTypeAndStage(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusCompleted, core.ConclusionSuccess)
	// Simulate a multi-stage pipeline with various events
	stages := []string{"implement", "review", "test"}
	for _, s := range stages {
		store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: s})
		store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventAgentOutput), Stage: s})
		store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageComplete), Stage: s})
	}

	session := setupTestClient(t, store)

	// Filter by event_type only
	res := callTool(t, session, "query_run_events", map[string]any{
		"run_id":     "r1",
		"event_type": string(core.EventStageStart),
	})
	var starts []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &starts); err != nil {
		t.Fatal(err)
	}
	if len(starts) != 3 {
		t.Fatalf("expected 3 stage_start events, got %d", len(starts))
	}

	// Filter by stage only
	res = callTool(t, session, "query_run_events", map[string]any{
		"run_id": "r1",
		"stage":  "review",
	})
	var reviewEvents []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &reviewEvents); err != nil {
		t.Fatal(err)
	}
	if len(reviewEvents) != 3 {
		t.Fatalf("expected 3 review events, got %d", len(reviewEvents))
	}

	// Combined: event_type + stage
	res = callTool(t, session, "query_run_events", map[string]any{
		"run_id":     "r1",
		"event_type": string(core.EventAgentOutput),
		"stage":      "test",
	})
	var testOutput []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &testOutput); err != nil {
		t.Fatal(err)
	}
	if len(testOutput) != 1 {
		t.Fatalf("expected 1 agent_output+test event, got %d", len(testOutput))
	}

	// Limit: get only first 2 of all events
	res = callTool(t, session, "query_run_events", map[string]any{
		"run_id": "r1",
		"limit":  2,
	})
	var limited []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &limited); err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected 2 events with limit, got %d", len(limited))
	}
}
