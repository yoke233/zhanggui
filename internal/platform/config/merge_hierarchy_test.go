package config

import "testing"

func ptr[T any](v T) *T { return &v }

func TestMergeHierarchy_RuntimeAndGitHub(t *testing.T) {
	global := &Config{
		GitHub: GitHubConfig{
			Token: "token-global",
			PR: GitHubPRConfig{
				BranchPrefix: "global/",
			},
		},
		Runtime: RuntimeConfig{
			Agents: RuntimeAgentsConfig{
				Drivers: []RuntimeDriverConfig{
					{ID: "claude-acp", LaunchCommand: "npx", LaunchArgs: []string{"-y", "@zed-industries/claude-agent-acp"}},
				},
				Profiles: []RuntimeProfileConfig{
					{ID: "lead", Driver: "claude-acp", Role: "lead", PromptTemplate: "team_leader"},
				},
			},
		},
	}

	project := &ConfigLayer{
		GitHub: &GitHubLayer{
			Enabled: ptr(true),
			Owner:   ptr("project-owner"),
		},
		Runtime: &RuntimeLayer{
			Agents: &RuntimeAgentsLayerCfg{
				Profiles: &[]RuntimeProfileConfig{
					{ID: "worker", Driver: "claude-acp", Role: "worker", PromptTemplate: "implement"},
				},
			},
		},
	}

	override := map[string]any{
		"github": map[string]any{
			"repo": "override-repo",
			"pr": map[string]any{
				"branch_prefix": "flow/",
			},
		},
		"runtime": map[string]any{
			"agents": map[string]any{
				"drivers": []map[string]any{
					{
						"id":             "codex-acp",
						"launch_command": "npx",
						"launch_args":    []string{"-y", "@zed-industries/codex-acp"},
						"capabilities_max": map[string]any{
							"fs_read":  true,
							"fs_write": true,
							"terminal": true,
						},
					},
				},
			},
		},
	}

	merged, err := MergeForRun(global, project, override)
	if err != nil {
		t.Fatalf("MergeForRun returned error: %v", err)
	}

	if !merged.GitHub.Enabled {
		t.Fatal("expected github.enabled to inherit project override")
	}
	if merged.GitHub.Token != "token-global" {
		t.Fatalf("expected github.token inherited, got %q", merged.GitHub.Token)
	}
	if merged.GitHub.Owner != "project-owner" {
		t.Fatalf("expected github.owner from project, got %q", merged.GitHub.Owner)
	}
	if merged.GitHub.Repo != "override-repo" {
		t.Fatalf("expected github.repo from override, got %q", merged.GitHub.Repo)
	}
	if merged.GitHub.PR.BranchPrefix != "flow/" {
		t.Fatalf("expected github.pr.branch_prefix from override, got %q", merged.GitHub.PR.BranchPrefix)
	}
	if len(merged.Runtime.Agents.Profiles) != 1 || merged.Runtime.Agents.Profiles[0].ID != "worker" {
		t.Fatalf("expected runtime profiles to be replaced by project layer, got %#v", merged.Runtime.Agents.Profiles)
	}
	if len(merged.Runtime.Agents.Drivers) != 1 || merged.Runtime.Agents.Drivers[0].ID != "codex-acp" {
		t.Fatalf("expected runtime drivers to be replaced by override, got %#v", merged.Runtime.Agents.Drivers)
	}
}
