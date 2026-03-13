package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

// mockStore implements the subset of core.Store used by StepContextBuilder.
type mockStore struct {
	core.Store // embed to satisfy interface; panics on unused methods

	issues    map[int64]*core.Issue
	steps     map[int64][]*core.Step
	artifacts map[int64]*core.Artifact // stepID → latest artifact
	manifests map[int64]*core.FeatureManifest
	entries   map[int64][]*core.FeatureEntry // manifestID → entries
}

func newMockStore() *mockStore {
	return &mockStore{
		issues:    make(map[int64]*core.Issue),
		steps:     make(map[int64][]*core.Step),
		artifacts: make(map[int64]*core.Artifact),
		manifests: make(map[int64]*core.FeatureManifest),
		entries:   make(map[int64][]*core.FeatureEntry),
	}
}

func (m *mockStore) GetIssue(_ context.Context, id int64) (*core.Issue, error) {
	if issue, ok := m.issues[id]; ok {
		return issue, nil
	}
	return nil, core.ErrNotFound
}

func (m *mockStore) ListStepsByIssue(_ context.Context, issueID int64) ([]*core.Step, error) {
	return m.steps[issueID], nil
}

func (m *mockStore) GetLatestArtifactByStep(_ context.Context, stepID int64) (*core.Artifact, error) {
	if art, ok := m.artifacts[stepID]; ok {
		return art, nil
	}
	return nil, core.ErrNotFound
}

func (m *mockStore) GetFeatureManifestByProject(_ context.Context, projectID int64) (*core.FeatureManifest, error) {
	if fm, ok := m.manifests[projectID]; ok {
		return fm, nil
	}
	return nil, core.ErrNotFound
}

func (m *mockStore) ListFeatureEntries(_ context.Context, filter core.FeatureEntryFilter) ([]*core.FeatureEntry, error) {
	return m.entries[filter.ManifestID], nil
}

func (m *mockStore) Close() error { return nil }

func TestBuild_FullMaterials(t *testing.T) {
	store := newMockStore()
	projectID := int64(1)
	store.issues[10] = &core.Issue{
		ID:        10,
		ProjectID: &projectID,
		Title:     "Implement login page",
		Body:      "Full description of the login page requirements...",
		Priority:  core.PriorityHigh,
		Labels:    []string{"frontend", "auth"},
	}
	store.steps[10] = []*core.Step{
		{ID: 1, IssueID: 10, Name: "requirements", Position: 0, Type: core.StepExec},
		{ID: 2, IssueID: 10, Name: "implement", Position: 1, Type: core.StepExec},
	}
	store.artifacts[1] = &core.Artifact{
		ID:             1,
		StepID:         1,
		ResultMarkdown: "## Requirements\n\n- Login form with email/password\n- OAuth support",
	}
	store.manifests[projectID] = &core.FeatureManifest{
		ID:        100,
		ProjectID: projectID,
		Version:   1,
		Summary:   "Auth features",
	}
	store.entries[100] = []*core.FeatureEntry{
		{ID: 1, ManifestID: 100, Key: "login", Description: "Login page", Status: core.FeaturePending},
	}

	step := &core.Step{
		ID:       2,
		IssueID:  10,
		Name:     "implement",
		Position: 1,
		Type:     core.StepExec,
		AcceptanceCriteria: []string{
			"Login form renders correctly",
			"OAuth redirects work",
		},
		Config: map[string]any{
			"last_gate_feedback": "Fix the CSS alignment",
			"rework_history":     "Round 1: Initial implementation rejected",
		},
	}
	exec := &core.Execution{ID: 42}

	builder := NewStepContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, step, exec)
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
		"issue.md",
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

	// Verify issue.md content.
	issueContent, _ := os.ReadFile(filepath.Join(dir, "issue.md"))
	if !strings.Contains(string(issueContent), "Implement login page") {
		t.Error("issue.md should contain the issue title")
	}
	if !strings.Contains(string(issueContent), "Full description") {
		t.Error("issue.md should contain the full body")
	}
	if !strings.Contains(string(issueContent), "high") {
		t.Error("issue.md should contain priority")
	}

	// Verify upstream artifact is not truncated.
	upstreamContent, _ := os.ReadFile(filepath.Join(dir, "upstream/requirements.md"))
	if !strings.Contains(string(upstreamContent), "OAuth support") {
		t.Error("upstream/requirements.md should contain full artifact content")
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
	if manifestData["summary"] != "Auth features" {
		t.Error("manifest.json should contain manifest summary")
	}
}

func TestBuild_NoIssue(t *testing.T) {
	store := newMockStore()
	// No issue in store

	step := &core.Step{ID: 1, IssueID: 999, Position: 0, Type: core.StepExec}
	exec := &core.Execution{ID: 1}

	builder := NewStepContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, step, exec)
	if err != nil {
		t.Fatalf("Build should succeed even without issue: %v", err)
	}
	// No materials → empty dir returned
	if dir != "" {
		t.Error("expected empty dir when no materials generated")
	}
}

func TestBuild_NoUpstream(t *testing.T) {
	store := newMockStore()
	store.issues[10] = &core.Issue{
		ID:    10,
		Title: "Solo step",
		Body:  "Just one step",
	}
	store.steps[10] = []*core.Step{
		{ID: 1, IssueID: 10, Name: "only-step", Position: 0, Type: core.StepExec},
	}

	step := &core.Step{ID: 1, IssueID: 10, Name: "only-step", Position: 0, Type: core.StepExec}
	exec := &core.Execution{ID: 1}

	builder := NewStepContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, step, exec)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty dir (issue.md should be generated)")
	}
	defer Cleanup(dir)

	// upstream/ directory should not exist.
	if _, err := os.Stat(filepath.Join(dir, "upstream")); !os.IsNotExist(err) {
		t.Error("upstream/ directory should not exist when there are no upstream steps")
	}
}

func TestBuild_GateFeedback(t *testing.T) {
	store := newMockStore()
	store.issues[10] = &core.Issue{ID: 10, Title: "Test"}

	step := &core.Step{
		ID:      1,
		IssueID: 10,
		Type:    core.StepExec,
		Config: map[string]any{
			"last_gate_feedback": "Please fix the tests",
		},
	}
	exec := &core.Execution{ID: 1}

	builder := NewStepContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, step, exec)
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
	store.issues[10] = &core.Issue{
		ID:    10,
		Title: "Test issue",
		Body:  "Some body",
	}
	store.steps[10] = []*core.Step{
		{ID: 1, IssueID: 10, Name: "design", Position: 0},
		{ID: 2, IssueID: 10, Name: "code", Position: 1},
	}
	store.artifacts[1] = &core.Artifact{
		StepID:         1,
		ResultMarkdown: "Design output",
	}

	step := &core.Step{
		ID:       2,
		IssueID:  10,
		Name:     "code",
		Position: 1,
		AcceptanceCriteria: []string{"Tests pass"},
	}
	exec := &core.Execution{ID: 5}

	builder := NewStepContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, step, exec)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	defer Cleanup(dir)

	skillMD, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	content := string(skillMD)

	// SKILL.md should reference all generated files.
	for _, expected := range []string{"issue.md", "upstream/design.md", "acceptance.md"} {
		if !strings.Contains(content, expected) {
			t.Errorf("SKILL.md should reference %s", expected)
		}
	}
	// gate-feedback.md should NOT be in the index (not generated).
	if strings.Contains(content, "gate-feedback.md") {
		t.Error("SKILL.md should not reference gate-feedback.md when not generated")
	}
}

func TestBuild_LargeArtifact(t *testing.T) {
	store := newMockStore()
	store.issues[10] = &core.Issue{ID: 10, Title: "Large artifact test"}
	largeContent := strings.Repeat("x", 100000) // 100KB — well above the old 4000 char limit
	store.steps[10] = []*core.Step{
		{ID: 1, IssueID: 10, Name: "big-step", Position: 0},
		{ID: 2, IssueID: 10, Name: "next", Position: 1},
	}
	store.artifacts[1] = &core.Artifact{
		StepID:         1,
		ResultMarkdown: largeContent,
	}

	step := &core.Step{ID: 2, IssueID: 10, Name: "next", Position: 1}
	exec := &core.Execution{ID: 1}

	builder := NewStepContextBuilder(store)
	parentDir := t.TempDir()
	dir, err := builder.Build(context.Background(), parentDir, step, exec)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	defer Cleanup(dir)

	content, _ := os.ReadFile(filepath.Join(dir, "upstream/big-step.md"))
	if len(content) != 100000 {
		t.Errorf("Large artifact should not be truncated: got %d bytes, want 100000", len(content))
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
	GetIssue(context.Context, int64) (*core.Issue, error)
	ListStepsByIssue(context.Context, int64) ([]*core.Step, error)
	GetLatestArtifactByStep(context.Context, int64) (*core.Artifact, error)
	GetFeatureManifestByProject(context.Context, int64) (*core.FeatureManifest, error)
	ListFeatureEntries(context.Context, core.FeatureEntryFilter) ([]*core.FeatureEntry, error)
} = (*mockStore)(nil)

