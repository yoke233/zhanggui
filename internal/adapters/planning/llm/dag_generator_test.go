package llmplanning

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/adapters/llm"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

// mockDAGComplete returns a mock llm.Client-compatible Complete function
// that returns a predefined DAG JSON.
func mockDAGComplete(dagJSON string) func(ctx context.Context, prompt string, tools []llm.ToolDef) (json.RawMessage, error) {
	return func(_ context.Context, _ string, _ []llm.ToolDef) (json.RawMessage, error) {
		return json.RawMessage(dagJSON), nil
	}
}

func TestValidateGeneratedDAG_Valid(t *testing.T) {
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "parse", Type: "exec"},
			{Name: "implement", Type: "exec", DependsOn: []string{"parse"}},
			{Name: "review", Type: "gate", DependsOn: []string{"implement"}},
			{Name: "deploy", Type: "exec", DependsOn: []string{"review"}},
		},
	}
	if err := validateGeneratedDAG(dag); err != nil {
		t.Fatalf("expected valid DAG, got: %v", err)
	}
}

func TestValidateGeneratedDAG_DuplicateName(t *testing.T) {
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "build", Type: "exec"},
			{Name: "build", Type: "exec"},
		},
	}
	if err := validateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestValidateGeneratedDAG_MissingDependency(t *testing.T) {
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "build", Type: "exec", DependsOn: []string{"nonexistent"}},
		},
	}
	if err := validateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for missing dependency")
	}
}

func TestValidateGeneratedDAG_InvalidType(t *testing.T) {
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "build", Type: "unknown"},
		},
	}
	if err := validateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestValidateGeneratedDAG_EmptyName(t *testing.T) {
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "", Type: "exec"},
		},
	}
	if err := validateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateGeneratedDAG_BackwardReference(t *testing.T) {
	// step B depends on C, but C appears after B → should fail
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "A", Type: "exec"},
			{Name: "B", Type: "exec", DependsOn: []string{"C"}},
			{Name: "C", Type: "exec", DependsOn: []string{"A"}},
		},
	}
	if err := validateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for backward reference")
	}
}

func TestValidateCapabilityFit_Pass(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "be", Role: core.RoleWorker, Capabilities: []string{"go", "backend"}},
		{ID: "gater", Role: core.RoleGate, Capabilities: []string{"review"}},
	}
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "code", Type: "exec", AgentRole: "worker", RequiredCapabilities: []string{"go"}},
			{Name: "review", Type: "gate", AgentRole: "gate", RequiredCapabilities: []string{"review"}},
		},
	}
	if err := validateCapabilityFit(dag, profiles); err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}
}

func TestValidateCapabilityFit_NoMatch(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "be", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "frontend", Type: "exec", AgentRole: "worker", RequiredCapabilities: []string{"react"}},
		},
	}
	if err := validateCapabilityFit(dag, profiles); err == nil {
		t.Fatal("expected error for unmatched capability")
	}
}

func TestValidateCapabilityFit_RoleMismatch(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "worker", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "review", Type: "gate", AgentRole: "gate", RequiredCapabilities: []string{"go"}},
		},
	}
	if err := validateCapabilityFit(dag, profiles); err == nil {
		t.Fatal("expected error for role mismatch")
	}
}

func TestValidateCapabilityFit_NoRoleFilter(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "any", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}
	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			// No agent_role specified — should match any profile with the capability.
			{Name: "code", Type: "exec", RequiredCapabilities: []string{"go"}},
		},
	}
	if err := validateCapabilityFit(dag, profiles); err != nil {
		t.Fatalf("expected pass with no role filter, got: %v", err)
	}
}

func TestDAGGenerator_Materialize(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	issueID, err := store.CreateIssue(ctx, &core.Issue{Title: "gen-test", Status: core.IssueOpen})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "parse-requirements", Type: "exec", AgentRole: "worker",
				Description:          "Parse the task requirements into actionable items",
				RequiredCapabilities: []string{"backend"},
				AcceptanceCriteria:   []string{"requirements parsed"}},
			{Name: "implement-api", Type: "exec", AgentRole: "worker",
				Description:          "Implement the REST API endpoints",
				DependsOn:            []string{"parse-requirements"},
				RequiredCapabilities: []string{"go", "backend"},
				AcceptanceCriteria:   []string{"API endpoints implemented"}},
			{Name: "code-review", Type: "gate", AgentRole: "gate",
				Description:          "Review the implementation for quality",
				DependsOn:            []string{"implement-api"},
				RequiredCapabilities: []string{"review"},
				AcceptanceCriteria:   []string{"code quality approved"}},
			{Name: "deploy", Type: "exec", AgentRole: "worker",
				Description:          "Deploy to staging environment",
				DependsOn:            []string{"code-review"},
				RequiredCapabilities: []string{"deploy"},
				AcceptanceCriteria:   []string{"deployed to staging"}},
		},
	}

	gen := &DAGGenerator{}
	steps, err := gen.Materialize(ctx, store, issueID, dag)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if len(steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(steps))
	}

	// Verify names.
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = s.Name
	}
	expected := []string{"parse-requirements", "implement-api", "code-review", "deploy"}
	for i, name := range expected {
		if names[i] != name {
			t.Fatalf("step[%d] expected %q, got %q", i, name, names[i])
		}
	}

	// Verify linearized execution order is preserved via Position.
	for i, step := range steps {
		if step.Position != i {
			t.Fatalf("step[%d] expected position %d, got %d", i, i, step.Position)
		}
	}

	// Verify types.
	if steps[2].Type != core.StepGate {
		t.Fatalf("code-review expected gate, got %s", steps[2].Type)
	}

	// Verify acceptance criteria.
	if len(steps[0].AcceptanceCriteria) != 1 || steps[0].AcceptanceCriteria[0] != "requirements parsed" {
		t.Fatalf("unexpected acceptance_criteria: %v", steps[0].AcceptanceCriteria)
	}

	// Verify description is mapped.
	if steps[0].Description != "Parse the task requirements into actionable items" {
		t.Fatalf("description not mapped: %q", steps[0].Description)
	}

	// Verify required_capabilities is mapped.
	if len(steps[1].RequiredCapabilities) != 2 || steps[1].RequiredCapabilities[0] != "go" {
		t.Fatalf("required_capabilities not mapped: %v", steps[1].RequiredCapabilities)
	}

	// Verify all steps are persisted and fetchable.
	stored, err := store.ListStepsByIssue(ctx, issueID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(stored) != 4 {
		t.Fatalf("expected 4 stored steps, got %d", len(stored))
	}

	// Verify persisted description round-trips.
	if stored[0].Description != "Parse the task requirements into actionable items" {
		t.Fatalf("persisted description mismatch: %q", stored[0].Description)
	}

	// Verify persisted required_capabilities round-trips.
	if len(stored[1].RequiredCapabilities) != 2 {
		t.Fatalf("persisted required_capabilities mismatch: %v", stored[1].RequiredCapabilities)
	}

	// Verify DAG is valid.
	if err := ValidateDAG(stored); err != nil {
		t.Fatalf("generated DAG validation failed: %v", err)
	}
}

func TestDAGGenerator_Materialize_BadReference(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	issueID, _ := store.CreateIssue(ctx, &core.Issue{Title: "bad-ref", Status: core.IssueOpen})

	dag := &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "A", Type: "exec", DependsOn: []string{"nonexistent"}},
		},
	}

	gen := &DAGGenerator{}
	_, err = gen.Materialize(ctx, store, issueID, dag)
	if err == nil {
		t.Fatal("expected error for unresolvable dependency")
	}
}

func TestBuildDAGGenPrompt_WithProfiles(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "be-worker", Role: core.RoleWorker, Capabilities: []string{"go", "backend"}},
		{ID: "reviewer", Role: core.RoleGate, Capabilities: []string{"review"}},
	}
	prompt := buildDAGGenPrompt("build an API", profiles)

	// Should contain profile info.
	if !strings.Contains(prompt, "be-worker") {
		t.Fatal("prompt should mention profile ID")
	}
	if !strings.Contains(prompt, "go, backend") {
		t.Fatal("prompt should list capabilities")
	}
	if !strings.Contains(prompt, "ONLY capability tags") {
		t.Fatal("prompt should instruct to use only known tags")
	}
}

func TestBuildDAGGenPrompt_NoProfiles(t *testing.T) {
	prompt := buildDAGGenPrompt("build an API", nil)

	// Should not contain profile section.
	if strings.Contains(prompt, "Available agent profiles") {
		t.Fatal("prompt should not contain profiles section when none available")
	}
}

func TestDagGenSchema_WithProfiles(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "w", Role: core.RoleWorker, Capabilities: []string{"go", "api"}},
	}
	tools := dagGenSchema(profiles)
	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}

	// Verify the schema includes required_capabilities field.
	schema := tools[0].InputSchema
	props := schema["properties"].(map[string]any)
	steps := props["steps"].(map[string]any)
	items := steps["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	if _, ok := itemProps["required_capabilities"]; !ok {
		t.Fatal("schema should include required_capabilities")
	}

	// Verify capability items have enum constraint.
	rc := itemProps["required_capabilities"].(map[string]any)
	capItems := rc["items"].(map[string]any)
	if _, ok := capItems["enum"]; !ok {
		t.Fatal("capability items should have enum constraint when profiles are provided")
	}
}

func TestDagGenSchema_NoProfiles(t *testing.T) {
	tools := dagGenSchema(nil)
	schema := tools[0].InputSchema
	props := schema["properties"].(map[string]any)
	steps := props["steps"].(map[string]any)
	items := steps["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	rc := itemProps["required_capabilities"].(map[string]any)
	capItems := rc["items"].(map[string]any)
	if _, ok := capItems["enum"]; ok {
		t.Fatal("capability items should NOT have enum constraint when no profiles")
	}
}
