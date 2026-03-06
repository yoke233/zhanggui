package mcpserver_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
)

// mockIssueManager implements mcpserver.IssueManager for testing submit_task.
type mockIssueManager struct {
	store      core.Store
	lastCreate mcpserver.CreateIssueInput
	lastAction string
	issue      *core.Issue
}

func (m *mockIssueManager) CreateIssue(_ context.Context, input mcpserver.CreateIssueInput) (*core.Issue, error) {
	m.lastCreate = input
	m.issue = &core.Issue{
		ID:        "test-issue-1",
		ProjectID: input.ProjectID,
		Title:     input.Title,
		Body:      input.Body,
		Template:  input.Template,
		State:     core.IssueStateOpen,
		Status:    core.IssueStatusDraft,
	}
	// Persist to store so FK constraints on attachments work.
	if m.store != nil {
		_ = m.store.CreateIssue(m.issue)
	}
	return m.issue, nil
}

func (m *mockIssueManager) UpdateIssue(_ context.Context, input mcpserver.UpdateIssueInput) (*core.Issue, error) {
	return m.issue, nil
}

func (m *mockIssueManager) ApplyIssueAction(_ context.Context, issueID, action, feedback string) (*core.Issue, error) {
	m.lastAction = action
	if m.issue != nil {
		m.issue.Status = core.IssueStatusReady
	}
	return m.issue, nil
}

func setupSubmitTestClient(t *testing.T, store core.Store, mgr mcpserver.IssueManager) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := mcpserver.NewServer(mcpserver.Deps{Store: store, IssueManager: mgr}, mcpserver.Options{})
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

func TestSubmitTask_Basic(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	mgr := &mockIssueManager{store: store}
	session := setupSubmitTestClient(t, store, mgr)

	res := callTool(t, session, "submit_task", map[string]any{
		"project_name": "Alpha",
		"description":  "Fix the login bug\nDetailed description here",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var result mcpserver.SubmitTaskResult
	if err := json.Unmarshal([]byte(resultText(t, res)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Issue == nil {
		t.Fatal("expected issue in result")
	}
	if result.Issue.Title != "Fix the login bug" {
		t.Errorf("expected title 'Fix the login bug', got %q", result.Issue.Title)
	}
	if mgr.lastCreate.ProjectID != "p1" {
		t.Errorf("expected project_id p1, got %s", mgr.lastCreate.ProjectID)
	}
	if mgr.lastAction != "approve" {
		t.Errorf("expected auto-approve, got action %q", mgr.lastAction)
	}
}

func TestSubmitTask_NoAutoApprove(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	mgr := &mockIssueManager{store: store}
	session := setupSubmitTestClient(t, store, mgr)

	res := callTool(t, session, "submit_task", map[string]any{
		"project_id":   "p1",
		"description":  "Do something",
		"auto_approve": false,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	if mgr.lastAction != "" {
		t.Errorf("expected no action, got %q", mgr.lastAction)
	}
}

func TestSubmitTask_WithAttachment(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	mgr := &mockIssueManager{store: store}
	session := setupSubmitTestClient(t, store, mgr)

	res := callTool(t, session, "submit_task", map[string]any{
		"project_id":  "p1",
		"description": "Task with files",
		"files": []any{
			map[string]any{
				"path":    "spec.md",
				"content": "# Spec\nHello",
			},
		},
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var result mcpserver.SubmitTaskResult
	if err := json.Unmarshal([]byte(resultText(t, res)), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Attachments) != 1 {
		t.Errorf("expected 1 attachment, got %d", len(result.Attachments))
	}
}

func TestSubmitTask_MissingDescription(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	mgr := &mockIssueManager{store: store}
	session := setupSubmitTestClient(t, store, mgr)

	// MCP SDK validates required fields at schema level, returning an error.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "submit_task",
		Arguments: map[string]any{"project_id": "p1"},
	})
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}
