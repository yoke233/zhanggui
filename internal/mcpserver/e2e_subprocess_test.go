package mcpserver_test

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

const mcpE2EServerEnv = "_MCP_E2E_SERVER"

// TestMain enables the fork-and-exec pattern: the test binary re-executes
// itself as an MCP stdio server when the env var is set.
func TestMain(m *testing.M) {
	if os.Getenv(mcpE2EServerEnv) != "" {
		runMCPServer()
		return
	}
	os.Exit(m.Run())
}

func runMCPServer() {
	dbPath := os.Getenv("AI_WORKFLOW_DB_PATH")
	if dbPath == "" {
		log.Fatal("AI_WORKFLOW_DB_PATH is required")
	}
	store, err := storesqlite.New(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	server := mcpserver.NewServer(mcpserver.Deps{Store: store}, mcpserver.Options{})
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server run: %v", err)
	}
}

// seedE2EDB creates a temp SQLite DB with realistic test data and returns the path.
func seedE2EDB(t *testing.T) string {
	t.Helper()
	dbPath := t.TempDir() + "/e2e.db"
	store, err := storesqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Project
	store.CreateProject(&core.Project{ID: "proj-1", Name: "my-service", RepoPath: "/tmp/my-service"})

	// Issues
	store.CreateChatSession(&core.ChatSession{ID: "chat-1", ProjectID: "proj-1"})
	store.CreateIssue(&core.Issue{
		ID: "iss-1", ProjectID: "proj-1", Title: "Fix login bug",
		Template: "bugfix", State: core.IssueStateOpen, Status: core.IssueStatusReady,
		SessionID: "chat-1",
	})
	store.CreateIssue(&core.Issue{
		ID: "iss-2", ProjectID: "proj-1", Title: "Add caching",
		Template: "feature", State: core.IssueStateClosed, Status: core.IssueStatusDone,
	})

	// Runs with different conclusions
	store.SaveRun(&core.Run{
		ID: "run-1", ProjectID: "proj-1", Name: "fix-login", Template: "standard",
		Status: core.StatusCompleted, Conclusion: core.ConclusionSuccess, IssueID: "iss-1",
	})
	store.SaveRun(&core.Run{
		ID: "run-2", ProjectID: "proj-1", Name: "add-cache", Template: "standard",
		Status: core.StatusCompleted, Conclusion: core.ConclusionFailure,
	})
	store.SaveRun(&core.Run{
		ID: "run-3", ProjectID: "proj-1", Name: "retry-cache", Template: "standard",
		Status: core.StatusInProgress,
	})

	// Events
	store.SaveRunEvent(core.RunEvent{RunID: "run-2", ProjectID: "proj-1", EventType: string(core.EventStageStart), Stage: "implement"})
	store.SaveRunEvent(core.RunEvent{RunID: "run-2", ProjectID: "proj-1", EventType: string(core.EventStageFailed), Stage: "implement", Error: "compile error"})
	store.SaveRunEvent(core.RunEvent{RunID: "run-2", ProjectID: "proj-1", EventType: string(core.EventStageStart), Stage: "test"})
	store.SaveRunEvent(core.RunEvent{RunID: "run-2", ProjectID: "proj-1", EventType: string(core.EventStageFailed), Stage: "test", Error: "timeout"})

	// Checkpoint
	store.SaveCheckpoint(&core.Checkpoint{
		RunID: "run-1", StageName: core.StageImplement, Status: core.CheckpointSuccess, AgentUsed: "codex",
	})

	return dbPath
}

func startMCPSubprocess(t *testing.T, dbPath string) *mcp.ClientSession {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$") // no-op test run, but TestMain intercepts
	cmd.Env = append(os.Environ(),
		mcpE2EServerEnv+"=1",
		"AI_WORKFLOW_DB_PATH="+dbPath,
	)
	cmd.Stderr = os.Stderr

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-test", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect to mcp subprocess: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// --- E2E Tests: verify the full binary→stdio→MCP protocol→SQLite path ---

func TestE2E_Subprocess_ListTools(t *testing.T) {
	dbPath := seedE2EDB(t)
	session := startMCPSubprocess(t, dbPath)

	ctx := context.Background()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"query_projects":       false,
		"query_project_detail": false,
		"query_issues":         false,
		"query_issue_detail":   false,
		"query_runs":           false,
		"query_run_detail":     false,
		"query_run_events":     false,
		"query_project_stats":  false,
	}
	for _, tool := range result.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestE2E_Subprocess_QueryProjects(t *testing.T) {
	dbPath := seedE2EDB(t)
	session := startMCPSubprocess(t, dbPath)

	res := callTool(t, session, "query_projects", nil)
	var projects []core.Project
	if err := json.Unmarshal([]byte(resultText(t, res)), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "my-service" {
		t.Fatalf("expected 1 project 'my-service', got %+v", projects)
	}
}

func TestE2E_Subprocess_QueryRunsFilterConclusion(t *testing.T) {
	dbPath := seedE2EDB(t)
	session := startMCPSubprocess(t, dbPath)

	res := callTool(t, session, "query_runs", map[string]any{
		"project_id": "proj-1",
		"conclusion": "failure",
	})
	var runs []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "run-2" {
		t.Fatalf("expected 1 failed run (run-2), got %+v", runs)
	}
	if runs[0].Conclusion != core.ConclusionFailure {
		t.Errorf("conclusion = %q, want %q", runs[0].Conclusion, core.ConclusionFailure)
	}
}

func TestE2E_Subprocess_QueryRunsByIssue(t *testing.T) {
	dbPath := seedE2EDB(t)
	session := startMCPSubprocess(t, dbPath)

	res := callTool(t, session, "query_runs", map[string]any{
		"project_id": "proj-1",
		"issue_id":   "iss-1",
	})
	var runs []core.Run
	if err := json.Unmarshal([]byte(resultText(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "run-1" {
		t.Fatalf("expected 1 run for iss-1, got %+v", runs)
	}
}

func TestE2E_Subprocess_QueryRunEventsFilterType(t *testing.T) {
	dbPath := seedE2EDB(t)
	session := startMCPSubprocess(t, dbPath)

	res := callTool(t, session, "query_run_events", map[string]any{
		"run_id":     "run-2",
		"event_type": string(core.EventStageFailed),
	})
	var events []core.RunEvent
	if err := json.Unmarshal([]byte(resultText(t, res)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 stage_failed events, got %d", len(events))
	}
	for _, e := range events {
		if e.Error == "" {
			t.Error("expected non-empty error")
		}
	}
}

func TestE2E_Subprocess_QueryProjectStats(t *testing.T) {
	dbPath := seedE2EDB(t)
	session := startMCPSubprocess(t, dbPath)

	res := callTool(t, session, "query_project_stats", map[string]any{"project_id": "proj-1"})
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
}

// TestE2E_Subprocess_DiagnoseFailedRun is a full scenario:
// find failed runs → get detail → get failed events → identify root cause.
func TestE2E_Subprocess_DiagnoseFailedRun(t *testing.T) {
	dbPath := seedE2EDB(t)
	session := startMCPSubprocess(t, dbPath)

	// Step 1: Stats show 50% success rate
	res := callTool(t, session, "query_project_stats", map[string]any{"project_id": "proj-1"})
	var stats mcpserver.ProjectStats
	json.Unmarshal([]byte(resultText(t, res)), &stats)
	if stats.SuccessRate != 0.5 {
		t.Fatalf("success rate = %f, want 0.5", stats.SuccessRate)
	}

	// Step 2: Find the failed run
	res = callTool(t, session, "query_runs", map[string]any{
		"project_id": "proj-1",
		"conclusion": "failure",
	})
	var failedRuns []core.Run
	json.Unmarshal([]byte(resultText(t, res)), &failedRuns)
	if len(failedRuns) != 1 {
		t.Fatalf("expected 1 failed run, got %d", len(failedRuns))
	}
	failedRunID := failedRuns[0].ID

	// Step 3: Get run detail with checkpoints
	res = callTool(t, session, "query_run_detail", map[string]any{"run_id": failedRunID})
	var detail mcpserver.RunDetail
	json.Unmarshal([]byte(resultText(t, res)), &detail)
	if detail.Run.Conclusion != core.ConclusionFailure {
		t.Errorf("conclusion = %q", detail.Run.Conclusion)
	}

	// Step 4: Get only the failure events
	res = callTool(t, session, "query_run_events", map[string]any{
		"run_id":     failedRunID,
		"event_type": string(core.EventStageFailed),
	})
	var failEvents []core.RunEvent
	json.Unmarshal([]byte(resultText(t, res)), &failEvents)
	if len(failEvents) != 2 {
		t.Fatalf("expected 2 failure events, got %d", len(failEvents))
	}
	// Verify both stages reported errors
	stages := map[string]string{}
	for _, e := range failEvents {
		stages[e.Stage] = e.Error
	}
	if stages["implement"] != "compile error" {
		t.Errorf("implement error = %q", stages["implement"])
	}
	if stages["test"] != "timeout" {
		t.Errorf("test error = %q", stages["test"])
	}
}

func TestE2E_Subprocess_FilterIssuesBySession(t *testing.T) {
	dbPath := seedE2EDB(t)
	session := startMCPSubprocess(t, dbPath)

	res := callTool(t, session, "query_issues", map[string]any{
		"project_id": "proj-1",
		"session_id": "chat-1",
	})
	var issues []core.Issue
	if err := json.Unmarshal([]byte(resultText(t, res)), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].ID != "iss-1" {
		t.Fatalf("expected 1 issue from chat-1, got %+v", issues)
	}
}
