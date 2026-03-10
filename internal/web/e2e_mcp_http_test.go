package web_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	"github.com/yoke233/ai-workflow/internal/web"
)

// TestE2E_MCP_HTTP_FullChain tests the complete chain:
// HTTP client → SSE endpoint → MCP server → SQLite store → response.
// This is the same path the team_leader agent uses when configured with SSE transport.
func TestE2E_MCP_HTTP_FullChain(t *testing.T) {
	store := setupMCPTestStore(t)
	seedMCPTestData(t, store)

	// Start web server with MCP endpoint
	srv := web.NewServer(web.Config{
		Store: store,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Connect as MCP client via SSE transport (same as team_leader would)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-test", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: ts.URL + "/api/v1/mcp",
	}, nil)
	if err != nil {
		t.Fatalf("connect to MCP SSE endpoint: %v", err)
	}
	defer session.Close()

	// --- Verify tool list ---
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	wantTools := []string{
		"query_projects", "query_project_detail",
		"query_issues", "query_issue_detail",
		"query_runs", "query_run_detail",
		"query_run_events", "query_project_stats",
	}
	registered := map[string]bool{}
	for _, tool := range tools.Tools {
		registered[tool.Name] = true
	}
	for _, name := range wantTools {
		if !registered[name] {
			t.Errorf("tool %q not registered", name)
		}
	}

	// --- Call query_projects ---
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "query_projects",
	})
	if err != nil {
		t.Fatalf("call query_projects: %v", err)
	}
	var projects []core.Project
	if err := json.Unmarshal([]byte(textContent(t, res)), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "my-service" {
		t.Fatalf("projects = %+v, want 1 project 'my-service'", projects)
	}

	// --- Call query_runs with conclusion filter ---
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query_runs",
		Arguments: map[string]any{"project_id": "proj-1", "conclusion": "failure"},
	})
	if err != nil {
		t.Fatalf("call query_runs: %v", err)
	}
	var runs []core.Run
	if err := json.Unmarshal([]byte(textContent(t, res)), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "run-fail" {
		t.Fatalf("failed runs = %+v, want [run-fail]", runs)
	}
	if runs[0].Conclusion != core.ConclusionFailure {
		t.Errorf("conclusion = %q, want failure", runs[0].Conclusion)
	}

	// --- Call query_run_events with type filter ---
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query_run_events",
		Arguments: map[string]any{"run_id": "run-fail", "event_type": string(core.EventStageFailed)},
	})
	if err != nil {
		t.Fatalf("call query_run_events: %v", err)
	}
	var events []core.RunEvent
	if err := json.Unmarshal([]byte(textContent(t, res)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("failed events = %d, want 1", len(events))
	}
	if events[0].Error != "compile error" {
		t.Errorf("error = %q, want 'compile error'", events[0].Error)
	}

	// --- Call query_project_stats and verify SuccessRate ---
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query_project_stats",
		Arguments: map[string]any{"project_id": "proj-1"},
	})
	if err != nil {
		t.Fatalf("call query_project_stats: %v", err)
	}
	var stats mcpserver.ProjectStats
	if err := json.Unmarshal([]byte(textContent(t, res)), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.SuccessRate != 0.5 {
		t.Errorf("success rate = %f, want 0.5", stats.SuccessRate)
	}
	if stats.TotalRuns != 3 {
		t.Errorf("total runs = %d, want 3", stats.TotalRuns)
	}

	// --- Call query_issues with session_id filter ---
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query_issues",
		Arguments: map[string]any{"project_id": "proj-1", "session_id": "chat-1"},
	})
	if err != nil {
		t.Fatalf("call query_issues: %v", err)
	}
	var issues []core.Issue
	if err := json.Unmarshal([]byte(textContent(t, res)), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].ID != "iss-1" {
		t.Fatalf("issues = %+v, want [iss-1]", issues)
	}
}

// TestE2E_MCP_HTTP_MCPToolsConfig verifies that MCPToolsFromRoleConfig
// produces SSE config when ServerAddr is set.
func TestE2E_MCP_HTTP_MCPToolsConfig(t *testing.T) {
	store := setupMCPTestStore(t)
	seedMCPTestData(t, store)

	srv := web.NewServer(web.Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Verify SSE config points to correct URL
	from := testMCPToolsSSE(t, ts.URL)
	if from.Sse == nil {
		t.Fatal("expected SSE config, got nil")
	}
	if from.Sse.Url != ts.URL+"/api/v1/mcp" {
		t.Errorf("url = %q, want %q", from.Sse.Url, ts.URL+"/api/v1/mcp")
	}
	if from.Sse.Name != "ai-workflow-query" {
		t.Errorf("name = %q, want %q", from.Sse.Name, "ai-workflow-query")
	}
	if from.Stdio != nil {
		t.Error("expected no stdio config in SSE mode")
	}
}

// TestE2E_ACP_MCP_TeamLeaderFlow simulates the team_leader agent's real workflow:
// RoleProfile → MCPToolsFromRoleConfig → ACP receives McpServer SSE config →
// agent connects to MCP SSE endpoint → queries project status for decision-making.
//
// This is the exact path executed by startWebChatSession in chat_assistant_acp.go:
//
//	effectiveMCPServers := teamleader.MCPToolsFromRoleConfig(role, mcpEnv)
//	client.NewSession(ctx, NewSessionRequest{McpServers: effectiveMCPServers})
func TestE2E_ACP_MCP_TeamLeaderFlow(t *testing.T) {
	// 1. Setup: store with realistic project data
	store := setupMCPTestStore(t)
	seedMCPTestData(t, store)

	srv := web.NewServer(web.Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 2. Generate MCP config exactly as startWebChatSession does
	role := acpclient.RoleProfile{
		ID:         "team-leader",
		MCPEnabled: true,
	}
	mcpEnv := teamleader.MCPEnvConfig{
		DBPath:     "/tmp/test.db",
		ServerAddr: ts.URL, // same as commands.go: "http://" + listenAddr
	}
	acpMCPServers := teamleader.MCPToolsFromRoleConfig(role, mcpEnv, true)

	// Verify: ACP would receive exactly 1 SSE-mode McpServer
	if len(acpMCPServers) != 1 {
		t.Fatalf("expected 1 McpServer for ACP, got %d", len(acpMCPServers))
	}
	mcpCfg := acpMCPServers[0]
	if mcpCfg.Sse == nil {
		t.Fatal("expected SSE transport for team-leader, got nil")
	}
	sseURL := mcpCfg.Sse.Url

	// 3. Simulate: ACP agent uses the SSE URL to connect MCP client
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "team-leader-sim", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: sseURL,
	}, nil)
	if err != nil {
		t.Fatalf("team-leader connect MCP via ACP config: %v", err)
	}
	defer session.Close()

	// 4. Team leader workflow: check project health
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query_project_stats",
		Arguments: map[string]any{"project_id": "proj-1"},
	})
	if err != nil {
		t.Fatalf("query_project_stats: %v", err)
	}
	var stats mcpserver.ProjectStats
	if err := json.Unmarshal([]byte(textContent(t, res)), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.SuccessRate != 0.5 {
		t.Errorf("success_rate = %f, want 0.5 (1 success, 1 failure, 1 in-progress)", stats.SuccessRate)
	}

	// 5. Team leader workflow: find failed runs to diagnose
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query_runs",
		Arguments: map[string]any{"project_id": "proj-1", "conclusion": "failure"},
	})
	if err != nil {
		t.Fatalf("query_runs(failure): %v", err)
	}
	var failedRuns []core.Run
	if err := json.Unmarshal([]byte(textContent(t, res)), &failedRuns); err != nil {
		t.Fatal(err)
	}
	if len(failedRuns) != 1 || failedRuns[0].ID != "run-fail" {
		t.Fatalf("failed runs = %+v, want [run-fail]", failedRuns)
	}

	// 6. Team leader workflow: drill into failure events
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query_run_events",
		Arguments: map[string]any{"run_id": "run-fail", "event_type": string(core.EventStageFailed)},
	})
	if err != nil {
		t.Fatalf("query_run_events: %v", err)
	}
	var events []core.RunEvent
	if err := json.Unmarshal([]byte(textContent(t, res)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Error != "compile error" {
		t.Fatalf("events = %+v, want 1 event with error 'compile error'", events)
	}

	// 7. Team leader workflow: check open issues in current chat session
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query_issues",
		Arguments: map[string]any{"project_id": "proj-1", "session_id": "chat-1"},
	})
	if err != nil {
		t.Fatalf("query_issues: %v", err)
	}
	var issues []core.Issue
	if err := json.Unmarshal([]byte(textContent(t, res)), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Title != "Fix login bug" {
		t.Fatalf("issues = %+v, want 1 issue 'Fix login bug'", issues)
	}
}

// --- helpers ---

func setupMCPTestStore(t *testing.T) core.Store {
	t.Helper()
	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func seedMCPTestData(t *testing.T, store core.Store) {
	t.Helper()
	store.CreateProject(&core.Project{ID: "proj-1", Name: "my-service", RepoPath: "/tmp/svc"})
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
	store.SaveRun(&core.Run{
		ID: "run-ok", ProjectID: "proj-1", Name: "ok", Template: "standard",
		Status: core.StatusCompleted, Conclusion: core.ConclusionSuccess,
	})
	store.SaveRun(&core.Run{
		ID: "run-fail", ProjectID: "proj-1", Name: "fail", Template: "standard",
		Status: core.StatusCompleted, Conclusion: core.ConclusionFailure,
	})
	store.SaveRun(&core.Run{
		ID: "run-wip", ProjectID: "proj-1", Name: "wip", Template: "standard",
		Status: core.StatusInProgress,
	})
	store.SaveRunEvent(core.RunEvent{
		RunID: "run-fail", ProjectID: "proj-1",
		EventType: string(core.EventStageFailed), Stage: "implement", Error: "compile error",
	})
}

func textContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("empty content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func testMCPToolsSSE(t *testing.T, serverURL string) acpproto.McpServer {
	t.Helper()
	role := acpclient.RoleProfile{
		MCPEnabled: true,
	}
	env := teamleader.MCPEnvConfig{
		DBPath:     "/tmp/test.db",
		ServerAddr: serverURL,
	}
	got := teamleader.MCPToolsFromRoleConfig(role, env, true)
	if len(got) != 1 {
		t.Fatalf("expected 1 McpServer, got %d", len(got))
	}
	return got[0]
}
