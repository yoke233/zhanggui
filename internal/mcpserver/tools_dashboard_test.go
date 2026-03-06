package mcpserver_test

import (
	"encoding/json"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
)

func TestProjectDashboard_Basic(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Bug1", "bugfix", core.IssueStateOpen, core.IssueStatusReady)
	seedIssue(t, store, "i2", "p1", "Bug2", "bugfix", core.IssueStateClosed, core.IssueStatusDone)
	seedRun(t, store, "r1", "p1", core.StatusCompleted, core.ConclusionSuccess)
	session := setupTestClient(t, store)

	res := callTool(t, session, "project_dashboard", map[string]any{"project_id": "p1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var dash mcpserver.ProjectDashboardResult
	if err := json.Unmarshal([]byte(resultText(t, res)), &dash); err != nil {
		t.Fatal(err)
	}
	if dash.Project.Name != "Alpha" {
		t.Errorf("expected Alpha, got %s", dash.Project.Name)
	}
	if dash.Stats.TotalIssues != 2 {
		t.Errorf("expected 2 total issues, got %d", dash.Stats.TotalIssues)
	}
	if len(dash.ActiveIssues) != 1 {
		t.Errorf("expected 1 active issue, got %d", len(dash.ActiveIssues))
	}
	if len(dash.RecentRuns) != 1 {
		t.Errorf("expected 1 recent run, got %d", len(dash.RecentRuns))
	}
}

func TestProjectDashboard_ByName(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	session := setupTestClient(t, store)

	res := callTool(t, session, "project_dashboard", map[string]any{"project_name": "Alpha"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var dash mcpserver.ProjectDashboardResult
	if err := json.Unmarshal([]byte(resultText(t, res)), &dash); err != nil {
		t.Fatal(err)
	}
	if dash.Project.ID != "p1" {
		t.Errorf("expected p1, got %s", dash.Project.ID)
	}
}

func TestProjectDashboard_NeedsAction(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	seedIssue(t, store, "i1", "p1", "Review me", "bugfix", core.IssueStateOpen, core.IssueStatusReviewing)
	seedRun(t, store, "r1", "p1", core.StatusActionRequired, "")
	session := setupTestClient(t, store)

	res := callTool(t, session, "project_dashboard", map[string]any{"project_id": "p1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var dash mcpserver.ProjectDashboardResult
	if err := json.Unmarshal([]byte(resultText(t, res)), &dash); err != nil {
		t.Fatal(err)
	}
	if len(dash.NeedsAction) < 2 {
		t.Errorf("expected at least 2 action items, got %d", len(dash.NeedsAction))
	}
}

func TestProjectDashboard_EmptyProject(t *testing.T) {
	store := setupTestStore(t)
	seedProject(t, store, "p1", "Alpha")
	session := setupTestClient(t, store)

	res := callTool(t, session, "project_dashboard", map[string]any{"project_id": "p1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	var dash mcpserver.ProjectDashboardResult
	if err := json.Unmarshal([]byte(resultText(t, res)), &dash); err != nil {
		t.Fatal(err)
	}
	if len(dash.ActiveIssues) != 0 {
		t.Errorf("expected 0 active issues, got %d", len(dash.ActiveIssues))
	}
	if len(dash.RecentRuns) != 0 {
		t.Errorf("expected 0 recent runs, got %d", len(dash.RecentRuns))
	}
	if dash.NeedsAction != nil {
		t.Errorf("expected nil needs_action, got %v", dash.NeedsAction)
	}
}
