package config

import "testing"

func TestMergeHierarchy_GlobalProjectPipeline(t *testing.T) {
	global := &Config{
		Agents: AgentsConfig{
			Claude: &AgentConfig{
				Binary:       ptr("claude-global"),
				MaxTurns:     ptr(30),
				Model:        ptr("global-model"),
				DefaultTools: ptrSlice("Read(*)", "Write(*)"),
			},
		},
	}

	project := &ConfigLayer{
		Agents: &AgentsLayer{
			Claude: &AgentConfig{
				MaxTurns: ptr(50),
			},
		},
	}

	override := map[string]any{
		"agents": map[string]any{
			"claude": map[string]any{
				"binary":        "claude-pipeline",
				"default_tools": []any{},
			},
		},
	}

	merged, err := MergeForPipeline(global, project, override)
	if err != nil {
		t.Fatalf("MergeForPipeline returned error: %v", err)
	}

	if merged.Agents.Claude == nil {
		t.Fatal("expected merged agents.claude to be set")
	}
	if merged.Agents.Claude.Binary == nil || *merged.Agents.Claude.Binary != "claude-pipeline" {
		t.Fatalf("expected pipeline override binary, got %v", merged.Agents.Claude.Binary)
	}
	if merged.Agents.Claude.MaxTurns == nil || *merged.Agents.Claude.MaxTurns != 50 {
		t.Fatalf("expected project max_turns=50, got %v", merged.Agents.Claude.MaxTurns)
	}
	if merged.Agents.Claude.Model == nil || *merged.Agents.Claude.Model != "global-model" {
		t.Fatalf("expected nil inheritance for model, got %v", merged.Agents.Claude.Model)
	}
	if merged.Agents.Claude.DefaultTools == nil {
		t.Fatal("expected default_tools to be present after explicit empty override")
	}
	if len(*merged.Agents.Claude.DefaultTools) != 0 {
		t.Fatalf("expected empty array to clear inherited tools, got %v", *merged.Agents.Claude.DefaultTools)
	}
}

func TestMergeForPipeline_DoesNotMutateGlobalWithEnvOverride(t *testing.T) {
	t.Setenv("AI_WORKFLOW_AGENTS_CLAUDE_BINARY", "claude-env")

	global := &Config{
		Agents: AgentsConfig{
			Claude: &AgentConfig{
				Binary: ptr("claude-global"),
			},
		},
	}

	merged, err := MergeForPipeline(global, nil, nil)
	if err != nil {
		t.Fatalf("MergeForPipeline returned error: %v", err)
	}

	if merged.Agents.Claude == nil || merged.Agents.Claude.Binary == nil {
		t.Fatal("expected merged claude binary to be set")
	}
	if got := *merged.Agents.Claude.Binary; got != "claude-env" {
		t.Fatalf("expected merged binary from env override, got %q", got)
	}

	if global.Agents.Claude == nil || global.Agents.Claude.Binary == nil {
		t.Fatal("expected global claude binary to remain set")
	}
	if got := *global.Agents.Claude.Binary; got != "claude-global" {
		t.Fatalf("expected global binary unchanged, got %q", got)
	}
}

func TestMergeHierarchy_SpecLayerOverridesGlobal(t *testing.T) {
	global := &Config{
		Spec: SpecConfig{
			Enabled:   true,
			Provider:  "openspec",
			OnFailure: "warn",
			OpenSpec: SpecOpenSpecConfig{
				Binary: "openspec-global",
			},
		},
	}

	project := &ConfigLayer{
		Spec: &SpecLayer{
			Provider: ptr("noop"),
			OpenSpec: &SpecOpenSpecLayer{
				Binary: ptr("openspec-project"),
			},
		},
	}

	override := map[string]any{
		"spec": map[string]any{
			"enabled":    false,
			"on_failure": "fail",
		},
	}

	merged, err := MergeForPipeline(global, project, override)
	if err != nil {
		t.Fatalf("MergeForPipeline returned error: %v", err)
	}

	if merged.Spec.Enabled {
		t.Fatalf("expected pipeline override to disable spec, got enabled=true")
	}
	if merged.Spec.Provider != "noop" {
		t.Fatalf("expected project layer provider noop, got %q", merged.Spec.Provider)
	}
	if merged.Spec.OnFailure != "fail" {
		t.Fatalf("expected pipeline override on_failure=fail, got %q", merged.Spec.OnFailure)
	}
	if merged.Spec.OpenSpec.Binary != "openspec-project" {
		t.Fatalf("expected project layer openspec.binary, got %q", merged.Spec.OpenSpec.Binary)
	}
}

func ptrSlice(values ...string) *[]string {
	v := append([]string(nil), values...)
	return &v
}
