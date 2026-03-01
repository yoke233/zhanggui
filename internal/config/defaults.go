package config

import "time"

func Defaults() Config {
	return Config{
		Agents: AgentsConfig{
			Claude: &AgentConfig{
				Plugin:   ptrValue("claude"),
				Binary:   ptrValue("claude"),
				MaxTurns: ptrValue(30),
			},
			Codex: &AgentConfig{
				Plugin:    ptrValue("codex"),
				Binary:    ptrValue("codex"),
				Model:     ptrValue("gpt-5.3-codex"),
				Reasoning: ptrValue("high"),
				Sandbox:   ptrValue("workspace-write"),
				Approval:  ptrValue("never"),
			},
			OpenSpec: &AgentConfig{
				Binary: ptrValue("openspec"),
			},
		},
		Spec: SpecConfig{
			Enabled:   false,
			Provider:  "noop",
			OnFailure: "warn",
			OpenSpec: SpecOpenSpecConfig{
				Binary: "openspec",
			},
		},
		Runtime: RuntimeConfig{
			Driver: "process",
		},
		Pipeline: PipelineConfig{
			DefaultTemplate:   "standard",
			GlobalTimeout:     2 * time.Hour,
			AutoInferTemplate: true,
			MaxTotalRetries:   5,
		},
		Scheduler: SchedulerConfig{
			MaxGlobalAgents:     3,
			MaxProjectPipelines: 2,
		},
		Secretary: SecretaryConfig{
			ReviewGatePlugin: "review-ai-panel",
			ReviewPanel: ReviewPanelConfig{
				MaxRounds: 2,
			},
			DAGScheduler: DAGSchedulerConfig{
				MaxConcurrentTasks: 2,
			},
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
