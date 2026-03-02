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

func TestGitHubConfig_MergeHierarchy_Works(t *testing.T) {
	global := &Config{
		GitHub: GitHubConfig{
			Enabled:             false,
			Token:               "token-global",
			AppID:               1234,
			PrivateKeyPath:      "/global/key.pem",
			InstallationID:      5678,
			Owner:               "global-owner",
			Repo:                "global-repo",
			WebhookSecret:       "secret-global",
			WebhookEnabled:      false,
			PREnabled:           false,
			AutoTrigger:         false,
			LabelMapping:        map[string]string{"type:bug": "quick"},
			AuthorizedUsernames: []string{"global-user"},
			PR: GitHubPRConfig{
				AutoCreate:   false,
				Draft:        true,
				AutoMerge:    false,
				Reviewers:    []string{"global-reviewer"},
				Labels:       []string{"global-label"},
				BranchPrefix: "global/",
			},
		},
	}

	project := &ConfigLayer{
		GitHub: &GitHubLayer{
			Enabled:             ptr(true),
			Owner:               ptr("project-owner"),
			Repo:                ptr("project-repo"),
			WebhookEnabled:      ptr(true),
			PREnabled:           ptr(true),
			AutoTrigger:         ptr(true),
			LabelMapping:        ptrStringMap(map[string]string{"type:feature": "full"}),
			AuthorizedUsernames: ptrSlice("alice", "bob"),
			PR: &GitHubPRLayer{
				AutoCreate:   ptr(true),
				Draft:        ptr(true),
				Reviewers:    ptrSlice("reviewer-a"),
				Labels:       ptrSlice("ai-generated"),
				BranchPrefix: ptr("feature/"),
			},
		},
	}

	override := map[string]any{
		"github": map[string]any{
			"repo":                 "override-repo",
			"webhook_secret":       "secret-override",
			"label_mapping":        map[string]any{"type:hotfix": "hotfix"},
			"authorized_usernames": []any{"carol"},
			"pr": map[string]any{
				"draft":      false,
				"auto_merge": true,
			},
		},
	}

	merged, err := MergeForPipeline(global, project, override)
	if err != nil {
		t.Fatalf("MergeForPipeline returned error: %v", err)
	}

	if !merged.GitHub.Enabled {
		t.Fatalf("expected project layer to set github.enabled=true")
	}
	if merged.GitHub.Token != "token-global" {
		t.Fatalf("expected global github.token inheritance, got %q", merged.GitHub.Token)
	}
	if merged.GitHub.Owner != "project-owner" {
		t.Fatalf("expected project layer owner, got %q", merged.GitHub.Owner)
	}
	if merged.GitHub.Repo != "override-repo" {
		t.Fatalf("expected pipeline override repo, got %q", merged.GitHub.Repo)
	}
	if merged.GitHub.WebhookSecret != "secret-override" {
		t.Fatalf("expected pipeline override webhook_secret, got %q", merged.GitHub.WebhookSecret)
	}
	if !merged.GitHub.WebhookEnabled {
		t.Fatalf("expected project layer webhook_enabled=true")
	}
	if !merged.GitHub.PREnabled {
		t.Fatalf("expected project layer pr_enabled=true")
	}
	if !merged.GitHub.AutoTrigger {
		t.Fatalf("expected project layer auto_trigger=true")
	}
	if len(merged.GitHub.LabelMapping) != 1 || merged.GitHub.LabelMapping["type:hotfix"] != "hotfix" {
		t.Fatalf("expected pipeline override label_mapping, got %#v", merged.GitHub.LabelMapping)
	}
	if len(merged.GitHub.AuthorizedUsernames) != 1 || merged.GitHub.AuthorizedUsernames[0] != "carol" {
		t.Fatalf("expected pipeline override authorized_usernames, got %#v", merged.GitHub.AuthorizedUsernames)
	}
	if !merged.GitHub.PR.AutoCreate {
		t.Fatalf("expected project layer github.pr.auto_create=true")
	}
	if merged.GitHub.PR.Draft {
		t.Fatalf("expected pipeline override github.pr.draft=false")
	}
	if !merged.GitHub.PR.AutoMerge {
		t.Fatalf("expected pipeline override github.pr.auto_merge=true")
	}
	if len(merged.GitHub.PR.Reviewers) != 1 || merged.GitHub.PR.Reviewers[0] != "reviewer-a" {
		t.Fatalf("expected project layer github.pr.reviewers, got %#v", merged.GitHub.PR.Reviewers)
	}
	if len(merged.GitHub.PR.Labels) != 1 || merged.GitHub.PR.Labels[0] != "ai-generated" {
		t.Fatalf("expected project layer github.pr.labels, got %#v", merged.GitHub.PR.Labels)
	}
	if merged.GitHub.PR.BranchPrefix != "feature/" {
		t.Fatalf("expected project layer github.pr.branch_prefix, got %q", merged.GitHub.PR.BranchPrefix)
	}
}

func ptrSlice(values ...string) *[]string {
	v := append([]string(nil), values...)
	return &v
}

func ptrStringMap(values map[string]string) *map[string]string {
	cloned := make(map[string]string, len(values))
	for k, v := range values {
		cloned[k] = v
	}
	return &cloned
}
