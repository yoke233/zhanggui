package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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
	if v, ok := os.LookupEnv("AI_WORKFLOW_SCHEDULER_WATCHDOG_ENABLED"); ok {
		enabled, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid AI_WORKFLOW_SCHEDULER_WATCHDOG_ENABLED: %w", err)
		}
		cfg.Scheduler.Watchdog.Enabled = enabled
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_SCHEDULER_WATCHDOG_INTERVAL"); ok {
		duration, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid AI_WORKFLOW_SCHEDULER_WATCHDOG_INTERVAL: %w", err)
		}
		cfg.Scheduler.Watchdog.Interval = Duration{Duration: duration}
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_SCHEDULER_WATCHDOG_STUCK_RUN_TTL"); ok {
		duration, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid AI_WORKFLOW_SCHEDULER_WATCHDOG_STUCK_RUN_TTL: %w", err)
		}
		cfg.Scheduler.Watchdog.StuckRunTTL = Duration{Duration: duration}
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_SCHEDULER_WATCHDOG_STUCK_MERGE_TTL"); ok {
		duration, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid AI_WORKFLOW_SCHEDULER_WATCHDOG_STUCK_MERGE_TTL: %w", err)
		}
		cfg.Scheduler.Watchdog.StuckMergeTTL = Duration{Duration: duration}
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_SCHEDULER_WATCHDOG_QUEUE_STALE_TTL"); ok {
		duration, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid AI_WORKFLOW_SCHEDULER_WATCHDOG_QUEUE_STALE_TTL: %w", err)
		}
		cfg.Scheduler.Watchdog.QueueStaleTTL = Duration{Duration: duration}
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

	// V2 collector OpenAI overrides (optional)
	if v, ok := os.LookupEnv("AI_WORKFLOW_V2_COLLECTOR_MAX_RETRIES"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("invalid AI_WORKFLOW_V2_COLLECTOR_MAX_RETRIES: %w", err)
		}
		cfg.V2.Collector.MaxRetries = n
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_V2_COLLECTOR_OPENAI_BASE_URL"); ok {
		cfg.V2.Collector.OpenAI.BaseURL = v
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_V2_COLLECTOR_OPENAI_API_KEY"); ok {
		cfg.V2.Collector.OpenAI.APIKey = v
	}
	if v, ok := os.LookupEnv("AI_WORKFLOW_V2_COLLECTOR_OPENAI_MODEL"); ok {
		cfg.V2.Collector.OpenAI.Model = v
	}

	return nil
}
