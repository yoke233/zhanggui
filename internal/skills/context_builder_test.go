package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/zhanggui/internal/core"
)

// mockStore implements the subset of core.Store used by ActionContextBuilder.
type mockStore struct {
	core.Store // embed to satisfy interface; panics on unused methods

	workItems map[int64]*core.WorkItem
	actions   map[int64][]*core.Action
	runs      map[int64]*core.Run            // actionID → latest run with result
	entries   map[int64][]*core.FeatureEntry // projectID → entries
}

func newMockStore() *mockStore {
	return &mockStore{
		workItems: make(map[int64]*core.WorkItem),
		actions:   make(map[int64][]*core.Action),
		runs:      make(map[int64]*core.Run),
		entries:   make(map[int64][]*core.FeatureEntry),
	}
}

func (m *mockStore) GetWorkItem(_ context.Context, id int64) (*core.WorkItem, error) {
	if wi, ok := m.workItems[id]; ok {
		return wi, nil
	}
	return nil, core.ErrNotFound
}

func (m *mockStore) ListActionsByWorkItem(_ context.Context, workItemID int64) ([]*core.Action, error) {
	return m.actions[workItemID], nil
}

func (m *mockStore) GetLatestRunWithResult(_ context.Context, actionID int64) (*core.Run, error) {
	if run, ok := m.runs[actionID]; ok {
		return run, nil
	}
	return nil, core.ErrNotFound
}

func (m *mockStore) ListFeatureEntries(_ context.Context, filter core.FeatureEntryFilter) ([]*core.FeatureEntry, error) {
	return m.entries[filter.ProjectID], nil
}

func (m *mockStore) Close() error { return nil }

func TestBuild_FullMaterials(t *testing.T) {
	store := newMockStore()
	projectID := int64(1)
	store.workItems[10] = &core.WorkItem{
		ID:        10,
		ProjectID: &projectID,
		Title:     "Implement login page",
		Body:      "Full description of the login page requirements...",
		Priority:  core.PriorityHigh,
		Labels:    []string{"frontend", "auth"},
	}
	store.actions[10] = []*core.Action{
		{ID: 1, WorkItemID: 10, Name: "requirements", Position: 0, Type: core.ActionExec},
		{ID: 2, WorkItemID: 10, Name: "implement", Position: 1, Type: core.ActionExec},
	}
	store.runs[1] = &core.Run{
		ID:             1,
		ActionID:       1,
		ResultMarkdown: "## Requirements\n\n- Login form with email/password\n- OAuth support",
	}
	store.entries[projectID] = []*core.FeatureEntry{
		{ID: 1, ProjectID: projectID, Key: "login", Description: "Login page", Status: core.FeaturePending},
	}

	action := &core.Action{
		ID:         2,
		WorkItemID: 10,
		Name:       "implement",
		Position:   1,
		Type:       core.ActionExec,
		AcceptanceCriteria: []string{
			"Login form renders correctly",
			"OAuth redirects work",
		},
		Config: map[string]any{
			"last_gate_feedback": "Fix the CSS alignment",
			"rework_history":     "Round 1: Initial implementation rejected",
		},
	}
	run := &core.Run{ID: 42}

	builder := NewActionContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, action, run)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty dir")
	}
	defer Cleanup(dir)

	// Verify all files exist.
	for _, f := range []string{
		"SKILL.md",
		"work-item.md",
		"upstream/requirements.md",
		"acceptance.md",
		"gate-feedback.md",
		"manifest.json",
	} {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist: %v", f, err)
		}
	}

	// Verify work-item material content.
	issueContent, _ := os.ReadFile(filepath.Join(dir, "work-item.md"))
	if !strings.Contains(string(issueContent), "Implement login page") {
		t.Error("work-item.md should contain the work item title")
	}
	if !strings.Contains(string(issueContent), "Full description") {
		t.Error("work-item.md should contain the full body")
	}
	if !strings.Contains(string(issueContent), "high") {
		t.Error("work-item.md should contain priority")
	}

	// Verify upstream deliverable is not truncated.
	upstreamContent, _ := os.ReadFile(filepath.Join(dir, "upstream/requirements.md"))
	if !strings.Contains(string(upstreamContent), "OAuth support") {
		t.Error("upstream/requirements.md should contain full deliverable content")
	}

	// Verify acceptance.md.
	acceptContent, _ := os.ReadFile(filepath.Join(dir, "acceptance.md"))
	if !strings.Contains(string(acceptContent), "1. Login form renders correctly") {
		t.Error("acceptance.md should contain numbered criteria")
	}

	// Verify gate-feedback.md.
	feedbackContent, _ := os.ReadFile(filepath.Join(dir, "gate-feedback.md"))
	if !strings.Contains(string(feedbackContent), "Fix the CSS alignment") {
		t.Error("gate-feedback.md should contain latest feedback")
	}
	if !strings.Contains(string(feedbackContent), "Round 1") {
		t.Error("gate-feedback.md should contain rework history")
	}

	// Verify manifest.json.
	manifestContent, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	var manifestData map[string]any
	if err := json.Unmarshal(manifestContent, &manifestData); err != nil {
		t.Fatalf("manifest.json should be valid JSON: %v", err)
	}
	if manifestData["project_id"] != float64(projectID) {
		t.Error("manifest.json should contain project_id")
	}
}

func TestBuild_NoWorkItem(t *testing.T) {
	store := newMockStore()
	// No work item in store

	action := &core.Action{ID: 1, WorkItemID: 999, Position: 0, Type: core.ActionExec}
	run := &core.Run{ID: 1}

	builder := NewActionContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, action, run)
	if err != nil {
		t.Fatalf("Build should succeed even without work item: %v", err)
	}
	// No materials → empty dir returned
	if dir != "" {
		t.Error("expected empty dir when no materials generated")
	}
}

func TestBuild_NoUpstream(t *testing.T) {
	store := newMockStore()
	store.workItems[10] = &core.WorkItem{
		ID:    10,
		Title: "Solo action",
		Body:  "Just one action",
	}
	store.actions[10] = []*core.Action{
		{ID: 1, WorkItemID: 10, Name: "only-action", Position: 0, Type: core.ActionExec},
	}

	action := &core.Action{ID: 1, WorkItemID: 10, Name: "only-action", Position: 0, Type: core.ActionExec}
	run := &core.Run{ID: 1}

	builder := NewActionContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, action, run)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty dir (work-item.md should be generated)")
	}
	defer Cleanup(dir)

	// upstream/ directory should not exist.
	if _, err := os.Stat(filepath.Join(dir, "upstream")); !os.IsNotExist(err) {
		t.Error("upstream/ directory should not exist when there are no upstream actions")
	}
}

func TestBuild_GateFeedback(t *testing.T) {
	store := newMockStore()
	store.workItems[10] = &core.WorkItem{ID: 10, Title: "Test"}

	action := &core.Action{
		ID:         1,
		WorkItemID: 10,
		Type:       core.ActionExec,
		Config: map[string]any{
			"last_gate_feedback": "Please fix the tests",
		},
	}
	run := &core.Run{ID: 1}

	builder := NewActionContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, action, run)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	defer Cleanup(dir)

	content, _ := os.ReadFile(filepath.Join(dir, "gate-feedback.md"))
	if !strings.Contains(string(content), "Please fix the tests") {
		t.Error("gate-feedback.md should contain the feedback")
	}
}

func TestBuild_SkillMD_Index(t *testing.T) {
	store := newMockStore()
	store.workItems[10] = &core.WorkItem{
		ID:    10,
		Title: "Test work item",
		Body:  "Some body",
	}
	store.actions[10] = []*core.Action{
		{ID: 1, WorkItemID: 10, Name: "design", Position: 0},
		{ID: 2, WorkItemID: 10, Name: "code", Position: 1},
	}
	store.runs[1] = &core.Run{
		ActionID:       1,
		ResultMarkdown: "Design output",
	}

	action := &core.Action{
		ID:                 2,
		WorkItemID:         10,
		Name:               "code",
		Position:           1,
		AcceptanceCriteria: []string{"Tests pass"},
	}
	run := &core.Run{ID: 5}

	builder := NewActionContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, action, run)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	defer Cleanup(dir)

	skillMD, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	content := string(skillMD)

	// SKILL.md should reference all generated files.
	for _, expected := range []string{"work-item.md", "upstream/design.md", "acceptance.md"} {
		if !strings.Contains(content, expected) {
			t.Errorf("SKILL.md should reference %s", expected)
		}
	}
	// gate-feedback.md should NOT be in the index (not generated).
	if strings.Contains(content, "gate-feedback.md") {
		t.Error("SKILL.md should not reference gate-feedback.md when not generated")
	}
}

func TestBuild_LargeResult(t *testing.T) {
	store := newMockStore()
	store.workItems[10] = &core.WorkItem{ID: 10, Title: "Large result test"}
	largeContent := strings.Repeat("x", 100000) // 100KB — well above the old 4000 char limit
	store.actions[10] = []*core.Action{
		{ID: 1, WorkItemID: 10, Name: "big-action", Position: 0},
		{ID: 2, WorkItemID: 10, Name: "next", Position: 1},
	}
	store.runs[1] = &core.Run{
		ActionID:       1,
		ResultMarkdown: largeContent,
	}

	action := &core.Action{ID: 2, WorkItemID: 10, Name: "next", Position: 1}
	run := &core.Run{ID: 1}

	builder := NewActionContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, action, run)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	defer Cleanup(dir)

	content, _ := os.ReadFile(filepath.Join(dir, "upstream/big-action.md"))
	if len(content) != 100000 {
		t.Errorf("Large deliverable should not be truncated: got %d bytes, want 100000", len(content))
	}
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	testDir := filepath.Join(dir, "test-cleanup")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "test.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	Cleanup(testDir)

	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Error("directory should be removed after Cleanup")
	}
}

func TestCleanup_EmptyString(t *testing.T) {
	// Should not panic.
	Cleanup("")
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"requirements", "requirements"},
		{"My Step Name", "my-step-name"},
		{"step/with/slashes", "step-with-slashes"},
		{"step:with:colons", "step-with-colons"},
		{"  spaces  ", "spaces"},
		{"", ""},
		{"a" + strings.Repeat("b", 100), "a" + strings.Repeat("b", 59)},
		{"---leading---", "leading"},
	}

	for _, tt := range tests {
		got := sanitizeFileName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeFileName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// Ensure the mockStore satisfies compile-time checks for the methods we use.
var _ interface {
	GetWorkItem(context.Context, int64) (*core.WorkItem, error)
	ListActionsByWorkItem(context.Context, int64) ([]*core.Action, error)
	GetLatestRunWithResult(context.Context, int64) (*core.Run, error)
	ListFeatureEntries(context.Context, core.FeatureEntryFilter) ([]*core.FeatureEntry, error)
} = (*mockStore)(nil)
