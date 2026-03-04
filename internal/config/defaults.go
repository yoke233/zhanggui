package config

import "time"

func Defaults() Config {
	return Config{
		Agents: AgentsConfig{
			Claude: &AgentConfig{
				Plugin:   ptrValue("claude"),
				Binary:   ptrValue("claude"),
				MaxTurns: ptrValue(30),
				CapabilitiesMax: &CapabilitiesConfig{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
			Codex: &AgentConfig{
				Plugin:    ptrValue("codex"),
				Binary:    ptrValue("codex"),
				Model:     ptrValue("gpt-5.3-codex"),
				Reasoning: ptrValue("high"),
				Sandbox:   ptrValue("workspace-write"),
				Approval:  ptrValue("never"),
				CapabilitiesMax: &CapabilitiesConfig{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
			OpenSpec: &AgentConfig{
				Binary: ptrValue("openspec"),
			},
			Profiles: []AgentProfileConfig{
				{
					Name:          "claude",
					LaunchCommand: "npx",
					LaunchArgs:    []string{"-y", "@zed-industries/claude-agent-acp@latest"},
					Env:           map[string]string{},
					CapabilitiesMax: CapabilitiesConfig{
						FSRead:   true,
						FSWrite:  true,
						Terminal: true,
					},
				},
				{
					Name:          "codex",
					LaunchCommand: "npx",
					LaunchArgs:    []string{"-y", "@zed-industries/codex-acp@latest"},
					Env:           map[string]string{},
					CapabilitiesMax: CapabilitiesConfig{
						FSRead:   true,
						FSWrite:  true,
						Terminal: true,
					},
				},
			},
		},
		Roles: []RoleConfig{
			{
				Name:           "team_leader",
				Agent:          "claude",
				PromptTemplate: "team_leader",
				Capabilities: CapabilitiesConfig{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
				Session: SessionConfig{
					Reuse:             true,
					PreferLoadSession: true,
				},
			},
			{
				Name:           "worker",
				Agent:          "codex",
				PromptTemplate: "implement",
				Capabilities: CapabilitiesConfig{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
				Session: SessionConfig{
					Reuse: true,
				},
			},
			{
				Name:           "reviewer",
				Agent:          "claude",
				PromptTemplate: "code_review",
				Capabilities: CapabilitiesConfig{
					FSRead:   true,
					FSWrite:  false,
					Terminal: false,
				},
				Session: SessionConfig{
					Reuse:       true,
					ResetPrompt: true,
				},
			},
			{
				Name:           "aggregator",
				Agent:          "claude",
				PromptTemplate: "review_aggregator",
				Capabilities: CapabilitiesConfig{
					FSRead:   true,
					FSWrite:  false,
					Terminal: false,
				},
				Session: SessionConfig{
					Reuse:       true,
					ResetPrompt: true,
				},
			},
			{
				Name:           "plan_parser",
				Agent:          "claude",
				PromptTemplate: "plan_parser",
				Capabilities: CapabilitiesConfig{
					FSRead:   true,
					FSWrite:  false,
					Terminal: false,
				},
			},
		},
		RoleBinds: RoleBindings{
			TeamLeader: SingleRoleBinding{
				Role: "team_leader",
			},
			Run: RunRoleBindings{
				StageRoles: map[string]string{
					"requirements": "worker",
					"implement":    "worker",
					"code_review":  "reviewer",
					"fixup":        "worker",
					"e2e_test":     "worker",
				},
			},
			ReviewOrchestrator: ReviewRoleBindings{
				Reviewers: map[string]string{
					"completeness": "reviewer",
					"dependency":   "reviewer",
					"feasibility":  "reviewer",
				},
				Aggregator: "aggregator",
			},
			PlanParser: SingleRoleBinding{
				Role: "plan_parser",
			},
		},
		Runtime: RuntimeConfig{
			Driver: "process",
		},
		Run: RunConfig{
			DefaultTemplate:   "standard",
			GlobalTimeout:     2 * time.Hour,
			AutoInferTemplate: true,
			MaxTotalRetries:   5,
		},
		Scheduler: SchedulerConfig{
			MaxGlobalAgents: 3,
			MaxProjectRuns:  2,
		},
		TeamLeader: TeamLeaderConfig{
			ReviewGatePlugin: "review-ai-panel",
			ReviewOrchestrator: ReviewOrchestratorConfig{
				MaxRounds: 2,
			},
			DAGScheduler: DAGSchedulerConfig{
				MaxConcurrentTasks: 2,
			},
		},
		A2A: A2AConfig{
			Enabled: false,
			Token:   "",
			Version: "0.3",
		},
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
		GitHub: GitHubConfig{
			Enabled: false,
		},
		Store: StoreConfig{
			Driver: "sqlite",
			Path:   "~/.ai-workflow/data.db",
		},
		Log: LogConfig{
			Level:      "info",
			File:       "~/.ai-workflow/logs/app.log",
			MaxSizeMB:  100,
			MaxAgeDays: 30,
		},
	}
}

func ptrValue[T any](v T) *T { return &v }
