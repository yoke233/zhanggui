package planning

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	"github.com/yoke233/zhanggui/internal/core"
)

func TestValidateGeneratedDAG_Valid(t *testing.T) {
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "parse", Type: "exec"},
			{Name: "implement", Type: "exec", DependsOn: []string{"parse"}},
			{Name: "review", Type: "gate", DependsOn: []string{"implement"}},
			{Name: "deploy", Type: "exec", DependsOn: []string{"review"}},
		},
	}
	if err := ValidateGeneratedDAG(dag); err != nil {
		t.Fatalf("expected valid DAG, got: %v", err)
	}
}

func TestValidateGeneratedDAG_DuplicateName(t *testing.T) {
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "build", Type: "exec"},
			{Name: "build", Type: "exec"},
		},
	}
	if err := ValidateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestValidateGeneratedDAG_MissingDependency(t *testing.T) {
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "build", Type: "exec", DependsOn: []string{"nonexistent"}},
		},
	}
	if err := ValidateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for missing dependency")
	}
}

func TestValidateGeneratedDAG_InvalidType(t *testing.T) {
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "build", Type: "unknown"},
		},
	}
	if err := ValidateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestValidateGeneratedDAG_EmptyName(t *testing.T) {
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "", Type: "exec"},
		},
	}
	if err := ValidateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateGeneratedDAG_BackwardReference(t *testing.T) {
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "A", Type: "exec"},
			{Name: "B", Type: "exec", DependsOn: []string{"C"}},
			{Name: "C", Type: "exec", DependsOn: []string{"A"}},
		},
	}
	if err := ValidateGeneratedDAG(dag); err == nil {
		t.Fatal("expected error for backward reference")
	}
}

func TestValidateCapabilityFit_Pass(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "be", Role: core.RoleWorker, Capabilities: []string{"go", "backend"}},
		{ID: "gater", Role: core.RoleGate, Capabilities: []string{"review"}},
	}
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "code", Type: "exec", AgentRole: "worker", RequiredCapabilities: []string{"go"}},
			{Name: "review", Type: "gate", AgentRole: "gate", RequiredCapabilities: []string{"review"}},
		},
	}
	if err := ValidateCapabilityFit(dag, profiles); err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}
}

func TestValidateCapabilityFit_NoMatch(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "be", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "frontend", Type: "exec", AgentRole: "worker", RequiredCapabilities: []string{"react"}},
		},
	}
	if err := ValidateCapabilityFit(dag, profiles); err == nil {
		t.Fatal("expected error for unmatched capability")
	}
}

func TestValidateCapabilityFit_RoleMismatch(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "worker", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "review", Type: "gate", AgentRole: "gate", RequiredCapabilities: []string{"go"}},
		},
	}
	if err := ValidateCapabilityFit(dag, profiles); err == nil {
		t.Fatal("expected error for role mismatch")
	}
}

func TestValidateCapabilityFit_NoRoleFilter(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "any", Role: core.RoleWorker, Capabilities: []string{"go"}},
	}
	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "code", Type: "exec", RequiredCapabilities: []string{"go"}},
		},
	}
	if err := ValidateCapabilityFit(dag, profiles); err != nil {
		t.Fatalf("expected pass with no role filter, got: %v", err)
	}
}

func TestService_Materialize(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{Title: "gen-test", Status: core.WorkItemOpen})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}

	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
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

	svc := &Service{}
	actions, err := svc.Materialize(ctx, store, workItemID, dag)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if len(actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(actions))
	}

	names := make([]string, len(actions))
	for i, action := range actions {
		names[i] = action.Name
	}
	expected := []string{"parse-requirements", "implement-api", "code-review", "deploy"}
	for i, name := range expected {
		if names[i] != name {
			t.Fatalf("action[%d] expected %q, got %q", i, name, names[i])
		}
	}

	for i, action := range actions {
		if action.Position != i {
			t.Fatalf("action[%d] expected position %d, got %d", i, i, action.Position)
		}
	}

	if actions[2].Type != core.ActionGate {
		t.Fatalf("code-review expected gate, got %s", actions[2].Type)
	}

	if len(actions[0].AcceptanceCriteria) != 1 || actions[0].AcceptanceCriteria[0] != "requirements parsed" {
		t.Fatalf("unexpected acceptance_criteria: %v", actions[0].AcceptanceCriteria)
	}

	if actions[0].Description != "Parse the task requirements into actionable items" {
		t.Fatalf("description not mapped: %q", actions[0].Description)
	}

	if len(actions[1].RequiredCapabilities) != 2 || actions[1].RequiredCapabilities[0] != "go" {
		t.Fatalf("required_capabilities not mapped: %v", actions[1].RequiredCapabilities)
	}

	stored, err := store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	if len(stored) != 4 {
		t.Fatalf("expected 4 stored actions, got %d", len(stored))
	}

	if stored[0].Description != "Parse the task requirements into actionable items" {
		t.Fatalf("persisted description mismatch: %q", stored[0].Description)
	}

	if len(stored[1].RequiredCapabilities) != 2 {
		t.Fatalf("persisted required_capabilities mismatch: %v", stored[1].RequiredCapabilities)
	}
}

func TestService_Materialize_BadReference(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	issueID, _ := store.CreateWorkItem(ctx, &core.WorkItem{Title: "bad-ref", Status: core.WorkItemOpen})

	dag := &GeneratedDAG{
		Actions: []GeneratedAction{
			{Name: "A", Type: "exec", DependsOn: []string{"nonexistent"}},
		},
	}

	svc := &Service{}
	_, err = svc.Materialize(ctx, store, issueID, dag)
	if err == nil {
		t.Fatal("expected error for unresolvable dependency")
	}
}

func TestBuildDAGGenPrompt_WithProfiles(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "be-worker", Role: core.RoleWorker, Capabilities: []string{"go", "backend"}},
		{ID: "reviewer", Role: core.RoleGate, Capabilities: []string{"review"}},
	}
	prompt := BuildDAGGenPrompt(GenerateInput{Description: "build an API"}, profiles)

	if !strings.Contains(prompt, "be-worker") {
		t.Fatal("prompt should mention profile ID")
	}
	if !strings.Contains(prompt, "go, backend") {
		t.Fatal("prompt should list capabilities")
	}
	if !strings.Contains(prompt, "Planning Guidance") {
		t.Fatal("prompt should include planning guidance section")
	}
	if !strings.Contains(prompt, "ONLY capability tags") {
		t.Fatal("prompt should instruct to use only known tags")
	}
}

func TestBuildDAGGenPrompt_NoProfiles(t *testing.T) {
	prompt := BuildDAGGenPrompt(GenerateInput{Description: "build an API"}, nil)

	if strings.Contains(prompt, "Available agent profiles") {
		t.Fatal("prompt should not contain profiles section when none available")
	}
}

func TestBuildDAGGenSchema_WithProfiles(t *testing.T) {
	profiles := []*core.AgentProfile{
		{ID: "w", Role: core.RoleWorker, Capabilities: []string{"go", "api"}},
	}
	tools := BuildDAGGenSchema(profiles)
	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}

	schema := tools[0].InputSchema
	props := schema["properties"].(map[string]any)
	steps := props["actions"].(map[string]any)
	items := steps["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	if _, ok := itemProps["required_capabilities"]; !ok {
		t.Fatal("schema should include required_capabilities")
	}

	rc := itemProps["required_capabilities"].(map[string]any)
	capItems := rc["items"].(map[string]any)
	if _, ok := capItems["enum"]; !ok {
		t.Fatal("capability items should have enum constraint when profiles are provided")
	}
}

func TestBuildDAGGenSchema_NoProfiles(t *testing.T) {
	tools := BuildDAGGenSchema(nil)
	schema := tools[0].InputSchema
	props := schema["properties"].(map[string]any)
	steps := props["actions"].(map[string]any)
	items := steps["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	rc := itemProps["required_capabilities"].(map[string]any)
	capItems := rc["items"].(map[string]any)
	if _, ok := capItems["enum"]; ok {
		t.Fatal("capability items should NOT have enum constraint when no profiles")
	}
}
