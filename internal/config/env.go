package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ApplyEnvOverrides(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	if v, ok := os.LookupEnv("AI_WORKFLOW_AGENTS_CLAUDE_BINARY"); ok {
		if cfg.Agents.Claude == nil {
			cfg.Agents.Claude = &AgentConfig{}
		}
		cfg.Agents.Claude.Binary = ptrValue(v)
	}

	if v, ok := os.LookupEnv("AI_WORKFLOW_SERVER_PORT"); ok {
		port, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid AI_WORKFLOW_SERVER_PORT: %w", err)
		}
		cfg.Server.Port = port
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_SERVER_HOST"); ok {
		cfg.Server.Host = v
	}

	if v, ok := os.LookupEnv("AI_WORKFLOW_SCHEDULER_MAX_GLOBAL_AGENTS"); ok {
		maxAgents, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid AI_WORKFLOW_SCHEDULER_MAX_GLOBAL_AGENTS: %w", err)
		}
		cfg.Scheduler.MaxGlobalAgents = maxAgents
	}

	if v, ok := os.LookupEnv("AI_WORKFLOW_A2A_ENABLED"); ok {
		enabled, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid AI_WORKFLOW_A2A_ENABLED: %w", err)
		}
		cfg.A2A.Enabled = enabled
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_A2A_TOKEN"); ok {
		cfg.A2A.Token = v
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_A2A_VERSION"); ok {
		cfg.A2A.Version = v
	}

	if v, ok := os.LookupEnv("AI_WORKFLOW_GITHUB_TOKEN"); ok {
		cfg.GitHub.Token = v
	}

	return nil
}
