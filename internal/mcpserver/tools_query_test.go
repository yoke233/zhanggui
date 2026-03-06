package mcpserver_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

// --- helpers ---

func setupTestStore(t *testing.T) core.Store {
	t.Helper()
	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func setupTestClient(t *testing.T, store core.Store) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := mcpserver.NewServer(mcpserver.Deps{Store: store}, mcpserver.Options{})
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return result
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("empty content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

func seedProject(t *testing.T, store core.Store, id, name string) {
	t.Helper()
	err := store.CreateProject(&core.Project{ID: id, Name: name, RepoPath: "/tmp/" + id})
	if err != nil {
		t.Fatal(err)
	}
}

func seedIssue(t *testing.T, store core.Store, id, projectID, title, template string, state core.IssueState, status core.IssueStatus) {
	t.Helper()
	err := store.CreateIssue(&core.Issue{
		ID:        id,
		ProjectID: projectID,
		Title:     title,
		Template:  template,
		State:     state,
		Status:    status,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func seedRun(t *testing.T, store core.Store, id, projectID string, status core.RunStatus, conclusion core.RunConclusion) {
	t.Helper()
	err := store.SaveRun(&core.Run{
		ID:         id,
		ProjectID:  projectID,
		Name:       "run-" + id,
		Template:   "default",
		Status:     status,
		Conclusion: conclusion,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// --- query_projects ---

func TestQueryProjects_ReturnsList(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedProject(t, store, "p2", "Beta")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_projects", nil)
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var projects []core.Project
	if err := json.Unmarshal([]byte(resultText(t, res)), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestQueryProjects_NameContainsFilter(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedProject(t, store, "p2", "Beta")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_projects", map[string]any{"name_contains": "alph"})
	var projects []core.Project
	if err := json.Unmarshal([]byte(resultText(t, res)), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].ID != "p1" {
		t.Errorf("expected p1, got %s", projects[0].ID)
	}
}

func TestQueryProjects_EmptyReturnsEmptyArray(t *testing.T) {
	store := setupTestStore(t)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_projects", nil)
	text := resultText(t, res)
	if text != "[]" {
		t.Errorf("expected [], got %s", text)
	}
}

// --- query_project_detail ---

func TestQueryProjectDetail_ReturnsProject(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_project_detail", map[string]any{"project_id": "p1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var p core.Project
	if err := json.Unmarshal([]byte(resultText(t, res)), &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Alpha" {
		t.Errorf("expected Alpha, got %s", p.Name)
	}
}

func TestQueryProjectDetail_NotFound(t *testing.T) {
	store := setupTestStore(t)
	session := setupTestClient(t, store)

	// Store returns (nil, error) for not-found. The handler propagates this as
	// a Go error, which the MCP SDK surfaces as an internal error result.
	res := callTool(t, session, "query_project_detail", map[string]any{"project_id": "nonexistent"})
	if !res.IsError {
		t.Fatal("expected IsError for nonexistent project")
	}
}

func TestQueryProjectDetail_MissingProjectID_EmptyStore(t *testing.T) {
	store := setupTestStore(t)
	session := setupTestClient(t, store)

	// No projects exist + no project_id/name → error from resolveProjectID.
	res := callTool(t, session, "query_project_detail", map[string]any{})
	if !res.IsError {
		t.Fatal("expected error for empty store with no project_id")
	}
}

// --- query_issues ---

func TestQueryIssues_ReturnsList(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	seedIssue(t, store, "i2", "p1", "Bug2", "bugfix", core.IssueStateClosed, core.IssueStatusDone)
	session := setupTestClient(t, store)

	// Default state is "open", so only i1 is returned.
	res := callTool(t, session, "query_issues", map[string]any{"project_id": "p1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var issues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 open issue, got %d", len(issues))
	}

	// Passing state=all returns both.
	res2 := callTool(t, session, "query_issues", map[string]any{"project_id": "p1", "state": "all"})
	var allIssues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res2)), &allIssues); err != nil {
		t.Fatal(err)
	}
	if len(allIssues) != 2 {
		t.Fatalf("expected 2 issues with state=all, got %d", len(allIssues))
	}
}

func TestQueryIssues_FilterByStatus(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	seedIssue(t, store, "i2", "p1", "Bug2", "bugfix", core.IssueStateClosed, core.IssueStatusDone)
	session := setupTestClient(t, store)

	// Must pass state=all to see closed issues when filtering by status=done.
	res := callTool(t, session, "query_issues", map[string]any{
		"project_id": "p1",
		"status":     "done",
		"state":      "all",
	})
	var issues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].ID != "i2" {
		t.Errorf("expected i2, got %s", issues[0].ID)
	}
}

func TestQueryIssues_FilterByState(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	seedIssue(t, store, "i2", "p1", "Bug2", "bugfix", core.IssueStateClosed, core.IssueStatusDone)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_issues", map[string]any{
		"project_id": "p1",
		"state":      "open",
	})
	var issues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].ID != "i1" {
		t.Errorf("expected i1, got %s", issues[0].ID)
	}
}

func TestQueryIssues_LimitOffset(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	seedIssue(t, store, "i2", "p1", "Bug2", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	seedIssue(t, store, "i3", "p1", "Bug3", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_issues", map[string]any{
		"project_id": "p1",
		"limit":      2,
		"offset":     1,
	})
	var issues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
}

func TestQueryIssues_EmptyReturnsEmptyArray(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_issues", map[string]any{"project_id": "p1"})
	text := resultText(t, res)
	if text != "[]" {
		t.Errorf("expected [], got %s", text)
	}
}

func TestQueryIssues_MissingProjectID_MultipleProjects(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedProject(t, store, "p2", "Beta")
	session := setupTestClient(t, store)

	// Multiple projects + no project_id/name → error.
	res := callTool(t, session, "query_issues", map[string]any{})
	if !res.IsError {
		t.Fatal("expected error for ambiguous project")
	}
}

func TestQueryIssues_AutoInferSingleProject(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	session := setupTestClient(t, store)

	// Single project → auto-inferred.
	res := callTool(t, session, "query_issues", map[string]any{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	var issues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
}

// --- query_issue_detail ---

func TestQueryIssueDetail_ReturnsIssueWithChangesAndReviews(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)

	if err := store.SaveIssueChange(&core.IssueChange{
		IssueID:  "i1",
		Field:    "status",
		OldValue: "draft",
		NewValue: "ready",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveReviewRecord(&core.ReviewRecord{
		IssueID:  "i1",
		Round:    1,
		Reviewer: "bot",
		Verdict:  "approved",
	}); err != nil {
		t.Fatal(err)
	}

	session := setupTestClient(t, store)
	res := callTool(t, session, "query_issue_detail", map[string]any{"issue_id": "i1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var detail mcpserver.IssueDetail
	if err := json.Unmarshal([]byte(resultText(t, res)), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Issue.ID != "i1" {
		t.Errorf("expected i1, got %s", detail.Issue.ID)
	}
	if len(detail.Changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(detail.Changes))
	}
	if len(detail.Reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(detail.Reviews))
	}
}

func TestQueryIssueDetail_EmptyChangesAndReviews(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_issue_detail", map[string]any{"issue_id": "i1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var detail mcpserver.IssueDetail
	if err := json.Unmarshal([]byte(resultText(t, res)), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(detail.Changes))
	}
	if len(detail.Reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(detail.Reviews))
	}
}

func TestQueryIssueDetail_NotFound(t *testing.T) {
	store := setupTestStore(t)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_issue_detail", map[string]any{"issue_id": "nonexistent"})
	if !res.IsError {
		t.Fatal("expected IsError for nonexistent issue")
	}
}

func TestQueryIssueDetail_MissingIssueID(t *testing.T) {
	store := setupTestStore(t)
	session := setupTestClient(t, store)

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "query_issue_detail",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing issue_id")
	}
}

// --- query_runs ---

func TestQueryRuns_ReturnsList(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusQueued, "")
	seedRun(t, store, "r2", "p1", core.StatusCompleted, core.ConclusionSuccess)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_runs", map[string]any{"project_id": "p1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var runs []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
}

func TestQueryRuns_FilterByStatus(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusQueued, "")
	seedRun(t, store, "r2", "p1", core.StatusCompleted, core.ConclusionSuccess)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_runs", map[string]any{
		"project_id": "p1",
		"status":     "completed",
	})
	var runs []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ID != "r2" {
		t.Errorf("expected r2, got %s", runs[0].ID)
	}
}

func TestQueryRuns_LimitOffset(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusQueued, "")
	seedRun(t, store, "r2", "p1", core.StatusQueued, "")
	seedRun(t, store, "r3", "p1", core.StatusQueued, "")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_runs", map[string]any{
		"project_id": "p1",
		"limit":      1,
	})
	var runs []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}

func TestQueryRuns_EmptyReturnsEmptyArray(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_runs", map[string]any{"project_id": "p1"})
	text := resultText(t, res)
	if text != "[]" {
		t.Errorf("expected [], got %s", text)
	}
}

func TestQueryRuns_MissingProjectID_MultipleProjects(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedProject(t, store, "p2", "Beta")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_runs", map[string]any{})
	if !res.IsError {
		t.Fatal("expected error for ambiguous project")
	}
}

// --- query_run_detail ---

func TestQueryRunDetail_ReturnsRunWithCheckpoints(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusCompleted, core.ConclusionSuccess)

	if err := store.SaveCheckpoint(&core.Checkpoint{
		RunID:     "r1",
		StageName: core.StageImplement,
		Status:    core.CheckpointSuccess,
		AgentUsed: "codex",
	}); err != nil {
		t.Fatal(err)
	}

	session := setupTestClient(t, store)
	res := callTool(t, session, "query_run_detail", map[string]any{"run_id": "r1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var detail mcpserver.RunDetail
	if err := json.Unmarshal([]byte(resultText(t, res)), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Run.ID != "r1" {
		t.Errorf("expected r1, got %s", detail.Run.ID)
	}
	if len(detail.Checkpoints) != 1 {
		t.Errorf("expected 1 checkpoint, got %d", len(detail.Checkpoints))
	}
}

func TestQueryRunDetail_NotFound(t *testing.T) {
	store := setupTestStore(t)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_run_detail", map[string]any{"run_id": "nonexistent"})
	if !res.IsError {
		t.Fatal("expected IsError for nonexistent run")
	}
}

func TestQueryRunDetail_MissingRunID(t *testing.T) {
	store := setupTestStore(t)
	session := setupTestClient(t, store)

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "query_run_detail",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing run_id")
	}
}

// --- query_run_events ---

func TestQueryRunEvents_ReturnsList(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusInProgress, "")

	if err := store.SaveRunEvent(core.RunEvent{
		RunID:     "r1",
		ProjectID: "p1",
		EventType: string(core.EventStageStart),
		Stage:     string(core.StageImplement),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRunEvent(core.RunEvent{
		RunID:     "r1",
		ProjectID: "p1",
		EventType: string(core.EventStageComplete),
		Stage:     string(core.StageImplement),
	}); err != nil {
		t.Fatal(err)
	}

	session := setupTestClient(t, store)
	res := callTool(t, session, "query_run_events", map[string]any{"run_id": "r1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var events []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestQueryRunEvents_EmptyReturnsEmptyArray(t *testing.T) {
	store := setupTestStore(t)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_run_events", map[string]any{"run_id": "r-none"})
	text := resultText(t, res)
	if text != "[]" {
		t.Errorf("expected [], got %s", text)
	}
}

func TestQueryRunEvents_MissingRunID(t *testing.T) {
	store := setupTestStore(t)
	session := setupTestClient(t, store)

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "query_run_events",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing run_id")
	}
}

// --- query_project_stats ---

func TestQueryProjectStats_CountsAndSuccessRate(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	seedIssue(t, store, "i2", "p1", "Bug2", "bugfix", core.IssueStateClosed, core.IssueStatusDone)
	seedIssue(t, store, "i3", "p1", "Bug3", "bugfix", core.IssueStateOpen, core.IssueStatusFailed)
	seedRun(t, store, "r1", "p1", core.StatusCompleted, core.ConclusionSuccess)
	seedRun(t, store, "r2", "p1", core.StatusCompleted, core.ConclusionFailure)
	seedRun(t, store, "r3", "p1", core.StatusQueued, "")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_project_stats", map[string]any{"project_id": "p1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var stats mcpserver.ProjectStats
	if err := json.Unmarshal([]byte(resultText(t, res)), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.TotalIssues != 3 {
		t.Errorf("expected 3 total issues, got %d", stats.TotalIssues)
	}
	if stats.OpenIssues != 2 {
		t.Errorf("expected 2 open issues, got %d", stats.OpenIssues)
	}
	if stats.ClosedIssues != 1 {
		t.Errorf("expected 1 closed issue, got %d", stats.ClosedIssues)
	}
	if stats.TotalRuns != 3 {
		t.Errorf("expected 3 total runs, got %d", stats.TotalRuns)
	}
	if stats.CompletedRuns != 2 {
		t.Errorf("expected 2 completed runs, got %d", stats.CompletedRuns)
	}
	// 1 success / 2 completed = 0.5
	if stats.SuccessRate != 0.5 {
		t.Errorf("expected 0.5 success rate, got %f", stats.SuccessRate)
	}
}

func TestQueryProjectStats_EmptyProjectReturnsZeros(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_project_stats", map[string]any{"project_id": "p1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var stats mcpserver.ProjectStats
	if err := json.Unmarshal([]byte(resultText(t, res)), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.TotalIssues != 0 {
		t.Errorf("expected 0 total issues, got %d", stats.TotalIssues)
	}
	if stats.OpenIssues != 0 {
		t.Errorf("expected 0 open issues, got %d", stats.OpenIssues)
	}
	if stats.ClosedIssues != 0 {
		t.Errorf("expected 0 closed issues, got %d", stats.ClosedIssues)
	}
	if stats.TotalRuns != 0 {
		t.Errorf("expected 0 total runs, got %d", stats.TotalRuns)
	}
	if stats.CompletedRuns != 0 {
		t.Errorf("expected 0 completed runs, got %d", stats.CompletedRuns)
	}
	if stats.SuccessRate != 0 {
		t.Errorf("expected 0 success rate, got %f", stats.SuccessRate)
	}
}

func TestQueryProjectStats_MissingProjectID_MultipleProjects(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedProject(t, store, "p2", "Beta")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_project_stats", map[string]any{})
	if !res.IsError {
		t.Fatal("expected error for ambiguous project")
	}
}

// --- new filter parameter tests ---

func TestQueryIssues_FilterBySessionID(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	// Create chat sessions first (FK constraint)
	store.CreateChatSession(&core.ChatSession{ID: "sess-a", ProjectID: "p1"})
	store.CreateChatSession(&core.ChatSession{ID: "sess-b", ProjectID: "p1"})
	if err := store.CreateIssue(&core.Issue{
		ID: "i1", ProjectID: "p1", Title: "Bug1", Template: "bugfix",
		State: core.IssueStateOpen, Status: core.IssueStatusReady, SessionID: "sess-a",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateIssue(&core.Issue{
		ID: "i2", ProjectID: "p1", Title: "Bug2", Template: "bugfix",
		State: core.IssueStateOpen, Status: core.IssueStatusReady, SessionID: "sess-b",
	}); err != nil {
		t.Fatal(err)
	}
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_issues", map[string]any{
		"project_id": "p1",
		"session_id": "sess-a",
	})
	var issues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].ID != "i1" {
		t.Errorf("expected i1, got %s", issues[0].ID)
	}
}

func TestQueryRuns_FilterByConclusion(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusCompleted, core.ConclusionSuccess)
	seedRun(t, store, "r2", "p1", core.StatusCompleted, core.ConclusionFailure)
	seedRun(t, store, "r3", "p1", core.StatusQueued, "")
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_runs", map[string]any{
		"project_id": "p1",
		"conclusion": "failure",
	})
	var runs []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ID != "r2" {
		t.Errorf("expected r2, got %s", runs[0].ID)
	}
}

func TestQueryRuns_FilterByIssueID(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	if err := store.SaveRun(&core.Run{
		ID: "r1", ProjectID: "p1", Name: "run-r1", Template: "default",
		Status: core.StatusCompleted, Conclusion: core.ConclusionSuccess, IssueID: "i1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(&core.Run{
		ID: "r2", ProjectID: "p1", Name: "run-r2", Template: "default",
		Status: core.StatusCompleted, Conclusion: core.ConclusionSuccess,
	}); err != nil {
		t.Fatal(err)
	}
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_runs", map[string]any{
		"project_id": "p1",
		"issue_id":   "i1",
	})
	var runs []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ID != "r1" {
		t.Errorf("expected r1, got %s", runs[0].ID)
	}
}

func TestQueryRunEvents_FilterByEventType(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusInProgress, "")
	store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: "implement"})
	store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageFailed), Stage: "implement"})
	store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: "review"})
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_run_events", map[string]any{
		"run_id":     "r1",
		"event_type": string(core.EventStageStart),
	})
	var events []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 stage_start events, got %d", len(events))
	}
}

func TestQueryRunEvents_FilterByStage(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusInProgress, "")
	store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: "implement"})
	store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageFailed), Stage: "implement"})
	store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: "review"})
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_run_events", map[string]any{
		"run_id": "r1",
		"stage":  "review",
	})
	var events []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 review event, got %d", len(events))
	}
}

func TestQueryRunEvents_Limit(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusInProgress, "")
	for i := 0; i < 5; i++ {
		store.SaveRunEvent(core.RunEvent{RunID: "r1", ProjectID: "p1", EventType: string(core.EventStageStart), Stage: "implement"})
	}
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_run_events", map[string]any{
		"run_id": "r1",
		"limit":  3,
	})
	var events []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestQueryRuns_ConclusionFieldPopulated(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedRun(t, store, "r1", "p1", core.StatusCompleted, core.ConclusionSuccess)
	session := setupTestClient(t, store)

	res := callTool(t, session, "query_runs", map[string]any{"project_id": "p1"})
	var runs []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Conclusion != core.ConclusionSuccess {
		t.Errorf("expected conclusion=%q, got %q", core.ConclusionSuccess, runs[0].Conclusion)
	}
}
