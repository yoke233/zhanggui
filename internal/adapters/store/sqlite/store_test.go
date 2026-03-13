package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestIssueCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	f := &core.WorkItem{Title: "test-issue", Status: core.WorkItemOpen, Metadata: map[string]any{"env": "test"}}
	id, err := s.CreateWorkItem(ctx, f)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id <= 0 {
		t.Fatal("expected positive id")
	}

	got, err := s.GetWorkItem(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "test-issue" || got.Status != core.WorkItemOpen {
		t.Fatalf("unexpected issue: %+v", got)
	}
	if got.Metadata["env"] != "test" {
		t.Fatalf("metadata not preserved: %v", got.Metadata)
	}

	if err := s.UpdateWorkItemStatus(ctx, id, core.WorkItemRunning); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, _ = s.GetWorkItem(ctx, id)
	if got.Status != core.WorkItemRunning {
		t.Fatalf("expected running, got %s", got.Status)
	}
	if err := s.UpdateWorkItemStatus(ctx, id, core.WorkItemDone); err != nil {
		t.Fatalf("update status to done: %v", err)
	}

	issues, err := s.ListWorkItems(ctx, core.WorkItemFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	if err := s.SetWorkItemArchived(ctx, id, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	got, err = s.GetWorkItem(ctx, id)
	if err != nil {
		t.Fatalf("get archived issue: %v", err)
	}
	if got.ArchivedAt == nil {
		t.Fatal("expected archived_at to be set")
	}

	archived := true
	issues, err = s.ListWorkItems(ctx, core.WorkItemFilter{Archived: &archived, Limit: 10})
	if err != nil {
		t.Fatalf("list archived: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 archived issue, got %d", len(issues))
	}

	archived = false
	issues, err = s.ListWorkItems(ctx, core.WorkItemFilter{Archived: &archived, Limit: 10})
	if err != nil {
		t.Fatalf("list unarchived: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected 0 unarchived issues, got %d", len(issues))
	}

	issues, err = s.ListWorkItems(ctx, core.WorkItemFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list all issues by default: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 total issue by default, got %d", len(issues))
	}
}

func TestIssueNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetWorkItem(context.Background(), 9999)
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStepCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "f", Status: core.WorkItemOpen})

	st := &core.Action{
		WorkItemID:           fID,
		Name:                 "implement",
		Type:                 core.ActionExec,
		Status:               core.ActionPending,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"backend", "go"},
		AcceptanceCriteria:   []string{"unit tests pass", "no lint errors"},
		Timeout:              5 * time.Minute,
		MaxRetries:           2,
		Config:               map[string]any{"timeout": "5m"},
	}
	id, err := s.CreateAction(ctx, st)
	if err != nil {
		t.Fatalf("create step: %v", err)
	}

	got, err := s.GetAction(ctx, id)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if got.Name != "implement" || got.Type != core.ActionExec || got.MaxRetries != 2 {
		t.Fatalf("unexpected step: %+v", got)
	}
	if got.Config["timeout"] != "5m" {
		t.Fatalf("config not preserved: %v", got.Config)
	}
	if len(got.RequiredCapabilities) != 2 || got.RequiredCapabilities[0] != "backend" {
		t.Fatalf("required_capabilities not preserved: %v", got.RequiredCapabilities)
	}
	if len(got.AcceptanceCriteria) != 2 || got.AcceptanceCriteria[0] != "unit tests pass" {
		t.Fatalf("acceptance_criteria not preserved: %v", got.AcceptanceCriteria)
	}
	if got.Timeout != 5*time.Minute {
		t.Fatalf("timeout not preserved: %v", got.Timeout)
	}

	// Second step with position
	st2 := &core.Action{
		WorkItemID: fID,
		Name:       "review",
		Type:       core.ActionGate,
		Status:     core.ActionPending,
		Position:   1,
	}
	id2, _ := s.CreateAction(ctx, st2)
	got2, _ := s.GetAction(ctx, id2)
	if got2.Position != 1 {
		t.Fatalf("position not preserved: %v", got2.Position)
	}

	steps, err := s.ListActionsByWorkItem(ctx, fID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	if err := s.UpdateActionStatus(ctx, id, core.ActionRunning); err != nil {
		t.Fatalf("update step status: %v", err)
	}
}

func TestStepUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "f", Status: core.WorkItemOpen})
	id, _ := s.CreateAction(ctx, &core.Action{WorkItemID: fID, Name: "s", Type: core.ActionExec, Status: core.ActionPending})

	got, _ := s.GetAction(ctx, id)
	got.AcceptanceCriteria = []string{"new criteria"}
	got.RequiredCapabilities = []string{"frontend"}
	if err := s.UpdateAction(ctx, got); err != nil {
		t.Fatalf("update step: %v", err)
	}

	got, _ = s.GetAction(ctx, id)
	if len(got.AcceptanceCriteria) != 1 || got.AcceptanceCriteria[0] != "new criteria" {
		t.Fatalf("update not applied: %v", got.AcceptanceCriteria)
	}
}

func TestExecutionCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "f", Status: core.WorkItemOpen})
	sID, _ := s.CreateAction(ctx, &core.Action{WorkItemID: fID, Name: "s", Type: core.ActionExec, Status: core.ActionPending})

	now := time.Now().UTC()
	e := &core.Run{
		ActionID:         sID,
		WorkItemID:       fID,
		Status:           core.RunCreated,
		AgentID:          "claude-1",
		BriefingSnapshot: "implement login API",
		Attempt:          1,
		Input:            map[string]any{"prompt": "do something"},
	}
	id, err := s.CreateRun(ctx, e)
	if err != nil {
		t.Fatalf("create exec: %v", err)
	}

	got, err := s.GetRun(ctx, id)
	if err != nil {
		t.Fatalf("get exec: %v", err)
	}
	if got.AgentID != "claude-1" || got.Attempt != 1 {
		t.Fatalf("unexpected exec: %+v", got)
	}
	if got.BriefingSnapshot != "implement login API" {
		t.Fatalf("briefing_snapshot not preserved: %s", got.BriefingSnapshot)
	}

	// Update with error_kind
	got.Status = core.RunFailed
	got.StartedAt = &now
	got.ErrorMessage = "timeout"
	got.ErrorKind = core.ErrKindTransient
	if err := s.UpdateRun(ctx, got); err != nil {
		t.Fatalf("update exec: %v", err)
	}

	got, _ = s.GetRun(ctx, id)
	if got.Status != core.RunFailed || got.ErrorKind != core.ErrKindTransient {
		t.Fatalf("expected failed/transient, got %s/%s", got.Status, got.ErrorKind)
	}

	execs, err := s.ListRunsByAction(ctx, sID)
	if err != nil {
		t.Fatalf("list execs: %v", err)
	}
	if len(execs) != 1 {
		t.Fatalf("expected 1 exec, got %d", len(execs))
	}
}

func TestToolCallAuditCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	workItemID, err := s.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "audit-work-item",
		Status:   core.WorkItemOpen,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := s.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "audit-action",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	runID, err := s.CreateRun(ctx, &core.Run{
		ActionID:   actionID,
		WorkItemID: workItemID,
		Status:     core.RunCreated,
		Attempt:    1,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	startedAt := time.Now().UTC().Add(-2 * time.Second)
	createdAt := time.Now().UTC()
	auditItem := &core.ToolCallAudit{
		WorkItemID:     workItemID,
		ActionID:       actionID,
		RunID:          runID,
		SessionID:      "session-1",
		ToolCallID:     "call-1",
		ToolName:       "functions.shell_command",
		Status:         "started",
		StartedAt:      &startedAt,
		InputDigest:    "input-digest",
		InputPreview:   "{\"token\":\"[REDACTED]\"}",
		RedactionLevel: "basic",
		CreatedAt:      createdAt,
	}
	id, err := s.CreateToolCallAudit(ctx, auditItem)
	if err != nil {
		t.Fatalf("create tool call audit: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero tool call audit id")
	}

	got, err := s.GetToolCallAudit(ctx, id)
	if err != nil {
		t.Fatalf("get tool call audit: %v", err)
	}
	if got.ToolCallID != "call-1" || got.Status != "started" {
		t.Fatalf("unexpected audit item: %+v", got)
	}

	gotByToolCallID, err := s.GetToolCallAuditByToolCallID(ctx, runID, "call-1")
	if err != nil {
		t.Fatalf("get tool call audit by tool_call_id: %v", err)
	}
	if gotByToolCallID.ID != id {
		t.Fatalf("unexpected audit id by tool_call_id: got %d want %d", gotByToolCallID.ID, id)
	}

	items, err := s.ListToolCallAuditsByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list tool call audits by run: %v", err)
	}
	if len(items) != 1 || items[0].ID != id {
		t.Fatalf("unexpected audit list: %+v", items)
	}

	finishedAt := time.Now().UTC()
	exitCode := 0
	got.Status = "completed"
	got.FinishedAt = &finishedAt
	got.DurationMs = finishedAt.Sub(startedAt).Milliseconds()
	got.ExitCode = &exitCode
	got.OutputDigest = "output-digest"
	got.StdoutDigest = "stdout-digest"
	got.StderrDigest = "stderr-digest"
	got.OutputPreview = "{\"stdout\":\"[REDACTED]\"}"
	got.StdoutPreview = "[REDACTED]"
	got.StderrPreview = "[REDACTED]"
	if err := s.UpdateToolCallAudit(ctx, got); err != nil {
		t.Fatalf("update tool call audit: %v", err)
	}

	updated, err := s.GetToolCallAudit(ctx, id)
	if err != nil {
		t.Fatalf("get updated tool call audit: %v", err)
	}
	if updated.Status != "completed" {
		t.Fatalf("status = %q, want completed", updated.Status)
	}
	if updated.FinishedAt == nil {
		t.Fatalf("expected finished_at to be set: %+v", updated)
	}
	if updated.ExitCode == nil || *updated.ExitCode != 0 {
		t.Fatalf("exit_code = %v, want 0", updated.ExitCode)
	}
	if updated.OutputDigest != "output-digest" || updated.StdoutDigest != "stdout-digest" || updated.StderrDigest != "stderr-digest" {
		t.Fatalf("unexpected digests after update: %+v", updated)
	}
	if updated.OutputPreview != "{\"stdout\":\"[REDACTED]\"}" || updated.StdoutPreview != "[REDACTED]" || updated.StderrPreview != "[REDACTED]" {
		t.Fatalf("unexpected previews after update: %+v", updated)
	}
}

func TestArtifactCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "f", Status: core.WorkItemOpen})
	sID, _ := s.CreateAction(ctx, &core.Action{WorkItemID: fID, Name: "s", Type: core.ActionExec, Status: core.ActionPending})
	eID, _ := s.CreateRun(ctx, &core.Run{ActionID: sID, WorkItemID: fID, Status: core.RunCreated, Attempt: 1})

	art := &core.Deliverable{
		RunID:          eID,
		ActionID:       sID,
		WorkItemID:     fID,
		ResultMarkdown: "## Done\nImplemented login API.",
		Metadata:       map[string]any{"status": "completed", "deliverables": []any{map[string]any{"type": "branch", "ref": "feat/login"}}},
		Assets:         []core.Asset{{Name: "screenshot.png", URI: "file:///tmp/screenshot.png", MediaType: "image/png"}},
	}
	id, err := s.CreateDeliverable(ctx, art)
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	got, err := s.GetDeliverable(ctx, id)
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if got.ResultMarkdown != "## Done\nImplemented login API." {
		t.Fatalf("result_markdown not preserved")
	}
	if got.Metadata["status"] != "completed" {
		t.Fatalf("metadata not preserved: %v", got.Metadata)
	}
	if len(got.Assets) != 1 || got.Assets[0].Name != "screenshot.png" {
		t.Fatalf("assets not preserved: %v", got.Assets)
	}

	// GetLatestByStep
	latest, err := s.GetLatestDeliverableByAction(ctx, sID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest.ID != id {
		t.Fatalf("expected latest to be %d, got %d", id, latest.ID)
	}

	// ListByExecution
	artifacts, err := s.ListDeliverablesByRun(ctx, eID)
	if err != nil {
		t.Fatalf("list by exec: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	// UpdateDeliverable
	got.Metadata["verdict"] = "pass"
	got.ResultMarkdown = "## Updated\nRevised output."
	if err := s.UpdateDeliverable(ctx, got); err != nil {
		t.Fatalf("update artifact: %v", err)
	}
	updated, _ := s.GetDeliverable(ctx, id)
	if updated.Metadata["verdict"] != "pass" {
		t.Fatalf("metadata not updated: %v", updated.Metadata)
	}
	if updated.ResultMarkdown != "## Updated\nRevised output." {
		t.Fatalf("result_markdown not updated: %s", updated.ResultMarkdown)
	}
}

func TestAgentContextCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "f", Status: core.WorkItemOpen})

	ac := &core.AgentContext{
		AgentID:      "claude-1",
		WorkItemID:   fID,
		SystemPrompt: "You are a developer",
		TurnCount:    0,
	}
	id, err := s.CreateAgentContext(ctx, ac)
	if err != nil {
		t.Fatalf("create agent_context: %v", err)
	}

	got, err := s.GetAgentContext(ctx, id)
	if err != nil {
		t.Fatalf("get agent_context: %v", err)
	}
	if got.AgentID != "claude-1" {
		t.Fatalf("unexpected agent_context: %+v", got)
	}

	found, err := s.FindAgentContext(ctx, "claude-1", fID)
	if err != nil {
		t.Fatalf("find agent_context: %v", err)
	}
	if found.ID != id {
		t.Fatal("find returned wrong context")
	}

	ac.TurnCount = 5
	ac.Summary = "implemented feature X"
	if err := s.UpdateAgentContext(ctx, ac); err != nil {
		t.Fatalf("update agent_context: %v", err)
	}
	got, _ = s.GetAgentContext(ctx, id)
	if got.TurnCount != 5 {
		t.Fatalf("expected turn_count 5, got %d", got.TurnCount)
	}
}

func TestEventCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "f", Status: core.WorkItemOpen})

	e1 := &core.Event{Type: core.EventWorkItemStarted, WorkItemID: fID, Data: map[string]any{"reason": "manual"}}
	_, err := s.CreateEvent(ctx, e1)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	e2 := &core.Event{Type: core.EventActionReady, WorkItemID: fID, ActionID: 1}
	s.CreateEvent(ctx, e2)

	events, err := s.ListEvents(ctx, core.EventFilter{WorkItemID: &fID})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	types := []core.EventType{core.EventWorkItemStarted}
	events, err = s.ListEvents(ctx, core.EventFilter{WorkItemID: &fID, Types: types})
	if err != nil {
		t.Fatalf("list events filtered: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestEventListFiltersBySessionID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "f", Status: core.WorkItemOpen})

	if _, err := s.CreateEvent(ctx, &core.Event{
		Type:       core.EventChatOutput,
		WorkItemID: fID,
		Data: map[string]any{
			"session_id": "session-a",
			"type":       "agent_message",
			"content":    "hello",
		},
	}); err != nil {
		t.Fatalf("create chat event a: %v", err)
	}
	if _, err := s.CreateEvent(ctx, &core.Event{
		Type:       core.EventChatOutput,
		WorkItemID: fID,
		Data: map[string]any{
			"session_id": "session-b",
			"type":       "agent_message",
			"content":    "world",
		},
	}); err != nil {
		t.Fatalf("create chat event b: %v", err)
	}

	events, err := s.ListEvents(ctx, core.EventFilter{
		SessionID: "session-a",
		Types:     []core.EventType{core.EventChatOutput},
	})
	if err != nil {
		t.Fatalf("list events by session: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if got, _ := events[0].Data["session_id"].(string); got != "session-a" {
		t.Fatalf("expected session-a, got %q", got)
	}
}

func TestEventListFiltersByThreadID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	threadID := int64(101)
	otherThreadID := int64(202)

	if _, err := s.CreateEvent(ctx, &core.Event{
		Type: core.EventThreadMessage,
		Data: map[string]any{
			"thread_id": threadID,
			"message":   "hello",
		},
	}); err != nil {
		t.Fatalf("create thread event a: %v", err)
	}
	if _, err := s.CreateEvent(ctx, &core.Event{
		Type: core.EventThreadMessage,
		Data: map[string]any{
			"thread_id": otherThreadID,
			"message":   "world",
		},
	}); err != nil {
		t.Fatalf("create thread event b: %v", err)
	}

	events, err := s.ListEvents(ctx, core.EventFilter{
		ThreadID: &threadID,
		Types:    []core.EventType{core.EventThreadMessage},
	})
	if err != nil {
		t.Fatalf("list events by thread: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if got, _ := events[0].Data["thread_id"].(float64); int64(got) != threadID {
		t.Fatalf("expected thread_id=%d, got %#v", threadID, events[0].Data["thread_id"])
	}
}

func TestProjectCRUD_NewFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &core.Project{
		Name:        "营销Q3",
		Kind:        core.ProjectGeneral,
		Description: "第三季度营销活动",
		Metadata:    map[string]string{"team": "marketing", "quarter": "Q3"},
	}
	id, err := s.CreateProject(ctx, p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id <= 0 {
		t.Fatal("expected positive id")
	}

	got, err := s.GetProject(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "营销Q3" {
		t.Fatalf("unexpected name: %s", got.Name)
	}
	if got.Kind != core.ProjectGeneral {
		t.Fatalf("unexpected kind: %s", got.Kind)
	}
	if got.Description != "第三季度营销活动" {
		t.Fatalf("unexpected description: %s", got.Description)
	}
	if got.Metadata["team"] != "marketing" || got.Metadata["quarter"] != "Q3" {
		t.Fatalf("metadata not preserved: %v", got.Metadata)
	}

	// Default kind when empty
	p2 := &core.Project{Name: "default-kind"}
	id2, err := s.CreateProject(ctx, p2)
	if err != nil {
		t.Fatalf("create p2: %v", err)
	}
	got2, _ := s.GetProject(ctx, id2)
	if got2.Kind != core.ProjectGeneral {
		t.Fatalf("expected general kind, got %s", got2.Kind)
	}

	// List
	projects, err := s.ListProjects(ctx, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestProjectUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, _ := s.CreateProject(ctx, &core.Project{Name: "old-name", Kind: core.ProjectGeneral})
	got, _ := s.GetProject(ctx, id)

	got.Name = "new-name"
	got.Kind = core.ProjectDev
	got.Description = "updated desc"
	got.Metadata = map[string]string{"env": "prod"}
	if err := s.UpdateProject(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	updated, _ := s.GetProject(ctx, id)
	if updated.Name != "new-name" || updated.Kind != core.ProjectDev {
		t.Fatalf("update not applied: %+v", updated)
	}
	if updated.Description != "updated desc" {
		t.Fatalf("description not updated: %s", updated.Description)
	}
	if updated.Metadata["env"] != "prod" {
		t.Fatalf("metadata not updated: %v", updated.Metadata)
	}

	// Update non-existent
	if err := s.UpdateProject(ctx, &core.Project{ID: 9999, Name: "x", Kind: "general"}); err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProjectDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, _ := s.CreateProject(ctx, &core.Project{Name: "to-delete", Kind: core.ProjectGeneral})
	if err := s.DeleteProject(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := s.GetProject(ctx, id)
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// Delete non-existent
	if err := s.DeleteProject(ctx, 9999); err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestResourceBindingCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	pID, _ := s.CreateProject(ctx, &core.Project{Name: "proj", Kind: core.ProjectDev})

	rb := &core.ResourceBinding{
		ProjectID: pID,
		Kind:      "git",
		URI:       "D:/repos/test-repo",
		Config:    map[string]any{"default_branch": "main"},
		Label:     "源码",
	}
	id, err := s.CreateResourceBinding(ctx, rb)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id <= 0 {
		t.Fatal("expected positive id")
	}

	got, err := s.GetResourceBinding(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Kind != "git" || got.URI != "D:/repos/test-repo" {
		t.Fatalf("unexpected binding: %+v", got)
	}
	if got.Label != "源码" {
		t.Fatalf("label not preserved: %s", got.Label)
	}
	if got.Config["default_branch"] != "main" {
		t.Fatalf("config not preserved: %v", got.Config)
	}

	// Create second binding
	rb2 := &core.ResourceBinding{
		ProjectID: pID,
		Kind:      "local_fs",
		URI:       "D:/data/assets",
	}
	s.CreateResourceBinding(ctx, rb2)

	// List
	bindings, err := s.ListResourceBindings(ctx, pID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}

	// Delete
	if err := s.DeleteResourceBinding(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = s.GetResourceBinding(ctx, id)
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Delete non-existent
	if err := s.DeleteResourceBinding(ctx, 9999); err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStepSignalCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create prerequisite issue + step.
	issueID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "signal-test", Status: core.WorkItemOpen})
	stepID, _ := s.CreateAction(ctx, &core.Action{
		WorkItemID: issueID, Name: "exec-step", Type: core.ActionExec, Status: core.ActionRunning, Position: 0,
	})

	// Create a signal.
	sig := &core.ActionSignal{
		ActionID:   stepID,
		WorkItemID: issueID,
		RunID:      100,
		Type:       core.SignalComplete,
		Source:     core.SignalSourceAgent,
		Payload:    map[string]any{"summary": "did stuff"},
		Actor:      "agent",
	}
	id, err := s.CreateActionSignal(ctx, sig)
	if err != nil {
		t.Fatalf("create signal: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero signal ID")
	}

	// List signals.
	signals, err := s.ListActionSignals(ctx, stepID)
	if err != nil {
		t.Fatalf("list signals: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Type != core.SignalComplete {
		t.Fatalf("expected type complete, got %s", signals[0].Type)
	}
	if signals[0].Payload["summary"] != "did stuff" {
		t.Fatalf("payload mismatch: %v", signals[0].Payload)
	}

	// GetLatestActionSignal — match type.
	latest, err := s.GetLatestActionSignal(ctx, stepID, core.SignalComplete)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest == nil || latest.ID != id {
		t.Fatalf("expected signal %d, got %v", id, latest)
	}

	// GetLatestActionSignal — no match.
	none, err := s.GetLatestActionSignal(ctx, stepID, core.SignalApprove)
	if err != nil {
		t.Fatalf("get latest approve: %v", err)
	}
	if none != nil {
		t.Fatalf("expected nil, got %v", none)
	}

	// Create a second signal and verify ordering.
	sig2 := &core.ActionSignal{
		ActionID: stepID, WorkItemID: issueID, RunID: 100,
		Type: core.SignalNeedHelp, Source: core.SignalSourceAgent,
		Payload: map[string]any{"reason": "stuck"}, Actor: "agent",
	}
	s.CreateActionSignal(ctx, sig2)

	latest2, _ := s.GetLatestActionSignal(ctx, stepID, core.SignalComplete, core.SignalNeedHelp)
	if latest2 == nil || latest2.Type != core.SignalNeedHelp {
		t.Fatalf("expected need_help as latest, got %v", latest2)
	}
}

func TestListPendingHumanActions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	issueID, _ := s.CreateWorkItem(ctx, &core.WorkItem{Title: "pending-test", Status: core.WorkItemOpen})
	// Blocked step — should appear.
	s.CreateAction(ctx, &core.Action{
		WorkItemID: issueID, Name: "blocked", Type: core.ActionExec, Status: core.ActionBlocked, Position: 0,
	})
	// Running step — should NOT appear.
	s.CreateAction(ctx, &core.Action{
		WorkItemID: issueID, Name: "running", Type: core.ActionExec, Status: core.ActionRunning, Position: 1,
	})
	// Done step — should NOT appear.
	s.CreateAction(ctx, &core.Action{
		WorkItemID: issueID, Name: "done", Type: core.ActionExec, Status: core.ActionDone, Position: 2,
	})

	pending, err := s.ListPendingHumanActions(ctx, issueID)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending step, got %d", len(pending))
	}
	if pending[0].Name != "blocked" {
		t.Fatalf("expected blocked step, got %s", pending[0].Name)
	}

	// Global query.
	allPending, err := s.ListAllPendingHumanActions(ctx)
	if err != nil {
		t.Fatalf("list all pending: %v", err)
	}
	if len(allPending) != 1 {
		t.Fatalf("expected 1 all-pending step, got %d", len(allPending))
	}
}

func TestSignalTypeIsTerminal(t *testing.T) {
	terminal := []core.SignalType{core.SignalComplete, core.SignalNeedHelp, core.SignalBlocked, core.SignalApprove, core.SignalReject}
	for _, st := range terminal {
		if !st.IsTerminal() {
			t.Errorf("expected %s to be terminal", st)
		}
	}
	nonTerminal := []core.SignalType{core.SignalProgress, core.SignalUnblock, core.SignalOverride}
	for _, st := range nonTerminal {
		if st.IsTerminal() {
			t.Errorf("expected %s to be non-terminal", st)
		}
	}
}

// Verify Store implements core.Store interface.
var _ core.Store = (*Store)(nil)
