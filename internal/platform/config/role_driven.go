package config

import (
	"fmt"
	"slices"
	"strings"
)

func Validate(cfg *Config) error {
	return validateConfig(cfg)
}

func validateConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	if err := validateWatchdogConfig(cfg.Scheduler.Watchdog); err != nil {
		return err
	}

	if err := validateRuntimeMCPConfig(cfg); err != nil {
		return err
	}
	if err := validateRuntimeLLMConfig(cfg); err != nil {
		return err
	}
	if err := validateAuditConfig(cfg); err != nil {
		return err
	}

	return nil
}

func validateAuditConfig(cfg *Config) error {
	if cfg == nil || !cfg.Audit.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.Audit.RedactionLevel) == "" {
		return fmt.Errorf("audit.redaction_level is required when audit is enabled")
	}
	if cfg.Audit.RetentionDays < 0 {
		return fmt.Errorf("audit.retention_days must be >= 0")
	}
	return nil
}

func validateRuntimeLLMConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(cfg.Runtime.LLM.Configs))
	for _, item := range cfg.Runtime.LLM.Configs {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return fmt.Errorf("runtime.llm.configs.id is required")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate runtime.llm.configs id %q", id)
		}
		seen[id] = struct{}{}
		typ := strings.ToLower(strings.TrimSpace(item.Type))
		switch typ {
		case "openai_chat_completion", "openai_response", "anthropic":
		default:
			return fmt.Errorf("runtime.llm.configs[%q].type must be openai_chat_completion, openai_response, or anthropic", id)
		}
	}
	if defaultID := strings.TrimSpace(cfg.Runtime.LLM.DefaultConfigID); defaultID != "" {
		if _, ok := seen[defaultID]; !ok {
			return fmt.Errorf("runtime.llm.default_config_id %q not found in runtime.llm.configs", defaultID)
		}
	}
	return nil
}

func validateRuntimeMCPConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	profileIDs := make(map[string]struct{}, len(cfg.Runtime.Agents.Profiles))
	for _, profile := range cfg.Runtime.Agents.Profiles {
		id := strings.TrimSpace(profile.ID)
		if id == "" {
			continue
		}
		profileIDs[id] = struct{}{}
	}

	serverIDs := make(map[string]struct{}, len(cfg.Runtime.MCP.Servers))
	for _, server := range cfg.Runtime.MCP.Servers {
		id := strings.TrimSpace(server.ID)
		if id == "" {
			return fmt.Errorf("runtime.mcp.servers.id is required")
		}
		if _, exists := serverIDs[id]; exists {
			return fmt.Errorf("duplicate runtime.mcp.servers id %q", id)
		}
		serverIDs[id] = struct{}{}

		kind := strings.ToLower(strings.TrimSpace(server.Kind))
		transport := strings.ToLower(strings.TrimSpace(server.Transport))
		switch kind {
		case "", "internal", "external":
		default:
			return fmt.Errorf("runtime.mcp.servers[%q].kind must be internal or external", id)
		}
		switch transport {
		case "stdio":
			if strings.TrimSpace(server.Command) == "" {
				return fmt.Errorf("runtime.mcp.servers[%q].command is required for stdio transport", id)
			}
		case "sse":
			if kind != "internal" && strings.TrimSpace(server.Endpoint) == "" {
				return fmt.Errorf("runtime.mcp.servers[%q].endpoint is required for sse transport", id)
			}
		default:
			return fmt.Errorf("runtime.mcp.servers[%q].transport must be stdio or sse", id)
		}
	}

	for _, binding := range cfg.Runtime.MCP.ProfileBindings {
		profile := strings.TrimSpace(binding.Profile)
		if profile == "" {
			return fmt.Errorf("runtime.mcp.profile_bindings.profile is required")
		}
		if _, ok := profileIDs[profile]; !ok {
			return fmt.Errorf("runtime.mcp.profile_bindings references missing profile %q", profile)
		}
		server := strings.TrimSpace(binding.Server)
		if server == "" {
			return fmt.Errorf("runtime.mcp.profile_bindings.server is required")
		}
		if _, ok := serverIDs[server]; !ok {
			return fmt.Errorf("runtime.mcp.profile_bindings for profile %q references missing server %q", profile, server)
		}
		mode := strings.ToLower(strings.TrimSpace(binding.ToolMode))
		switch mode {
		case "", "all":
		case "allow_list":
			if len(binding.Tools) == 0 {
				return fmt.Errorf("runtime.mcp.profile_bindings for profile %q and server %q requires tools when tool_mode=allow_list", profile, server)
			}
		default:
			return fmt.Errorf("runtime.mcp.profile_bindings for profile %q and server %q has invalid tool_mode %q", profile, server, binding.ToolMode)
		}
		if hasDuplicateStrings(binding.Tools) {
			return fmt.Errorf("runtime.mcp.profile_bindings for profile %q and server %q contains duplicate tools", profile, server)
		}
	}

	return nil
}

func hasDuplicateStrings(items []string) bool {
	if len(items) <= 1 {
		return false
	}
	seen := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if slices.Contains(seen, trimmed) {
			return true
		}
		seen = append(seen, trimmed)
	}
	return false
}

func validateWatchdogConfig(cfg WatchdogConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.Interval.Duration <= 0 {
		return fmt.Errorf("scheduler.watchdog.interval must be > 0")
	}
	if cfg.StuckRunTTL.Duration <= 0 {
		return fmt.Errorf("scheduler.watchdog.stuck_run_ttl must be > 0")
	}
	if cfg.StuckMergeTTL.Duration <= 0 {
		return fmt.Errorf("scheduler.watchdog.stuck_merge_ttl must be > 0")
	}
	if cfg.QueueStaleTTL.Duration <= 0 {
		return fmt.Errorf("scheduler.watchdog.queue_stale_ttl must be > 0")
	}
	return nil
}
