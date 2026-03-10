package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
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

func TestFlowCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	f := &core.Flow{Name: "test-flow", Status: core.FlowPending, Metadata: map[string]string{"env": "test"}}
	id, err := s.CreateFlow(ctx, f)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id <= 0 {
		t.Fatal("expected positive id")
	}

	got, err := s.GetFlow(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "test-flow" || got.Status != core.FlowPending {
		t.Fatalf("unexpected flow: %+v", got)
	}
	if got.Metadata["env"] != "test" {
		t.Fatalf("metadata not preserved: %v", got.Metadata)
	}

	if err := s.UpdateFlowStatus(ctx, id, core.FlowRunning); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, _ = s.GetFlow(ctx, id)
	if got.Status != core.FlowRunning {
		t.Fatalf("expected running, got %s", got.Status)
	}

	flows, err := s.ListFlows(ctx, core.FlowFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
}

func TestFlowNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetFlow(context.Background(), 9999)
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStepCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateFlow(ctx, &core.Flow{Name: "f", Status: core.FlowPending})

	st := &core.Step{
		FlowID:               fID,
		Name:                 "implement",
		Type:                 core.StepExec,
		Status:               core.StepPending,
		AgentRole:            "worker",
		RequiredCapabilities: []string{"dev.backend", "dev.go"},
		AcceptanceCriteria:   []string{"unit tests pass", "no lint errors"},
		Timeout:              5 * time.Minute,
		MaxRetries:           2,
		Config:               map[string]any{"timeout": "5m"},
	}
	id, err := s.CreateStep(ctx, st)
	if err != nil {
		t.Fatalf("create step: %v", err)
	}

	got, err := s.GetStep(ctx, id)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if got.Name != "implement" || got.Type != core.StepExec || got.MaxRetries != 2 {
		t.Fatalf("unexpected step: %+v", got)
	}
	if got.Config["timeout"] != "5m" {
		t.Fatalf("config not preserved: %v", got.Config)
	}
	if len(got.RequiredCapabilities) != 2 || got.RequiredCapabilities[0] != "dev.backend" {
		t.Fatalf("required_capabilities not preserved: %v", got.RequiredCapabilities)
	}
	if len(got.AcceptanceCriteria) != 2 || got.AcceptanceCriteria[0] != "unit tests pass" {
		t.Fatalf("acceptance_criteria not preserved: %v", got.AcceptanceCriteria)
	}
	if got.Timeout != 5*time.Minute {
		t.Fatalf("timeout not preserved: %v", got.Timeout)
	}

	// Step with dependencies
	st2 := &core.Step{
		FlowID:    fID,
		Name:      "review",
		Type:      core.StepGate,
		Status:    core.StepPending,
		DependsOn: []int64{id},
	}
	id2, _ := s.CreateStep(ctx, st2)
	got2, _ := s.GetStep(ctx, id2)
	if len(got2.DependsOn) != 1 || got2.DependsOn[0] != id {
		t.Fatalf("depends_on not preserved: %v", got2.DependsOn)
	}

	steps, err := s.ListStepsByFlow(ctx, fID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	if err := s.UpdateStepStatus(ctx, id, core.StepRunning); err != nil {
		t.Fatalf("update step status: %v", err)
	}
}

func TestStepUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateFlow(ctx, &core.Flow{Name: "f", Status: core.FlowPending})
	id, _ := s.CreateStep(ctx, &core.Step{FlowID: fID, Name: "s", Type: core.StepExec, Status: core.StepPending})

	got, _ := s.GetStep(ctx, id)
	got.AcceptanceCriteria = []string{"new criteria"}
	got.RequiredCapabilities = []string{"dev.frontend"}
	if err := s.UpdateStep(ctx, got); err != nil {
		t.Fatalf("update step: %v", err)
	}

	got, _ = s.GetStep(ctx, id)
	if len(got.AcceptanceCriteria) != 1 || got.AcceptanceCriteria[0] != "new criteria" {
		t.Fatalf("update not applied: %v", got.AcceptanceCriteria)
	}
}

func TestExecutionCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateFlow(ctx, &core.Flow{Name: "f", Status: core.FlowPending})
	sID, _ := s.CreateStep(ctx, &core.Step{FlowID: fID, Name: "s", Type: core.StepExec, Status: core.StepPending})

	now := time.Now().UTC()
	e := &core.Execution{
		StepID:           sID,
		FlowID:           fID,
		Status:           core.ExecCreated,
		AgentID:          "claude-1",
		BriefingSnapshot: "implement login API",
		Attempt:          1,
		Input:            map[string]any{"prompt": "do something"},
	}
	id, err := s.CreateExecution(ctx, e)
	if err != nil {
		t.Fatalf("create exec: %v", err)
	}

	got, err := s.GetExecution(ctx, id)
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
	got.Status = core.ExecFailed
	got.StartedAt = &now
	got.ErrorMessage = "timeout"
	got.ErrorKind = core.ErrKindTransient
	if err := s.UpdateExecution(ctx, got); err != nil {
		t.Fatalf("update exec: %v", err)
	}

	got, _ = s.GetExecution(ctx, id)
	if got.Status != core.ExecFailed || got.ErrorKind != core.ErrKindTransient {
		t.Fatalf("expected failed/transient, got %s/%s", got.Status, got.ErrorKind)
	}

	execs, err := s.ListExecutionsByStep(ctx, sID)
	if err != nil {
		t.Fatalf("list execs: %v", err)
	}
	if len(execs) != 1 {
		t.Fatalf("expected 1 exec, got %d", len(execs))
	}
}

func TestArtifactCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateFlow(ctx, &core.Flow{Name: "f", Status: core.FlowPending})
	sID, _ := s.CreateStep(ctx, &core.Step{FlowID: fID, Name: "s", Type: core.StepExec, Status: core.StepPending})
	eID, _ := s.CreateExecution(ctx, &core.Execution{StepID: sID, FlowID: fID, Status: core.ExecCreated, Attempt: 1})

	art := &core.Artifact{
		ExecutionID:    eID,
		StepID:         sID,
		FlowID:         fID,
		ResultMarkdown: "## Done\nImplemented login API.",
		Metadata:       map[string]any{"status": "completed", "deliverables": []any{map[string]any{"type": "branch", "ref": "feat/login"}}},
		Assets:         []core.Asset{{Name: "screenshot.png", URI: "file:///tmp/screenshot.png", MediaType: "image/png"}},
	}
	id, err := s.CreateArtifact(ctx, art)
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	got, err := s.GetArtifact(ctx, id)
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
	latest, err := s.GetLatestArtifactByStep(ctx, sID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest.ID != id {
		t.Fatalf("expected latest to be %d, got %d", id, latest.ID)
	}

	// ListByExecution
	artifacts, err := s.ListArtifactsByExecution(ctx, eID)
	if err != nil {
		t.Fatalf("list by exec: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	// UpdateArtifact
	got.Metadata["verdict"] = "pass"
	got.ResultMarkdown = "## Updated\nRevised output."
	if err := s.UpdateArtifact(ctx, got); err != nil {
		t.Fatalf("update artifact: %v", err)
	}
	updated, _ := s.GetArtifact(ctx, id)
	if updated.Metadata["verdict"] != "pass" {
		t.Fatalf("metadata not updated: %v", updated.Metadata)
	}
	if updated.ResultMarkdown != "## Updated\nRevised output." {
		t.Fatalf("result_markdown not updated: %s", updated.ResultMarkdown)
	}
}

func TestBriefingCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateFlow(ctx, &core.Flow{Name: "f", Status: core.FlowPending})
	sID, _ := s.CreateStep(ctx, &core.Step{FlowID: fID, Name: "s", Type: core.StepExec, Status: core.StepPending})

	b := &core.Briefing{
		StepID:    sID,
		Objective: "Implement user login API with JWT authentication",
		ContextRefs: []core.ContextRef{
			{Type: core.CtxFlowSummary, RefID: fID, Label: "flow summary"},
			{Type: core.CtxUpstreamArtifact, RefID: 42, Label: "design doc"},
		},
		Constraints: []string{"use existing auth middleware", "no new dependencies"},
	}
	id, err := s.CreateBriefing(ctx, b)
	if err != nil {
		t.Fatalf("create briefing: %v", err)
	}

	got, err := s.GetBriefing(ctx, id)
	if err != nil {
		t.Fatalf("get briefing: %v", err)
	}
	if got.Objective != b.Objective {
		t.Fatalf("objective not preserved")
	}
	if len(got.ContextRefs) != 2 {
		t.Fatalf("context_refs not preserved: %v", got.ContextRefs)
	}
	if len(got.Constraints) != 2 {
		t.Fatalf("constraints not preserved: %v", got.Constraints)
	}

	// GetByStep
	byStep, err := s.GetBriefingByStep(ctx, sID)
	if err != nil {
		t.Fatalf("get by step: %v", err)
	}
	if byStep.ID != id {
		t.Fatalf("expected %d, got %d", id, byStep.ID)
	}
}

func TestAgentContextCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fID, _ := s.CreateFlow(ctx, &core.Flow{Name: "f", Status: core.FlowPending})

	ac := &core.AgentContext{
		AgentID:      "claude-1",
		FlowID:       fID,
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

	fID, _ := s.CreateFlow(ctx, &core.Flow{Name: "f", Status: core.FlowPending})

	e1 := &core.Event{Type: core.EventFlowStarted, FlowID: fID, Data: map[string]any{"reason": "manual"}}
	_, err := s.CreateEvent(ctx, e1)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	e2 := &core.Event{Type: core.EventStepReady, FlowID: fID, StepID: 1}
	s.CreateEvent(ctx, e2)

	events, err := s.ListEvents(ctx, core.EventFilter{FlowID: &fID})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	types := []core.EventType{core.EventFlowStarted}
	events, err = s.ListEvents(ctx, core.EventFilter{FlowID: &fID, Types: types})
	if err != nil {
		t.Fatalf("list events filtered: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
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

// Verify Store implements core.Store interface.
var _ core.Store = (*Store)(nil)
