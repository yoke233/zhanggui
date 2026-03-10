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

	if err := validateV2MCPConfig(cfg); err != nil {
		return err
	}

	if !hasRoleDrivenData(cfg) {
		return nil
	}

	agents, err := indexAgents(cfg.EffectiveAgentProfiles())
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		return fmt.Errorf("role-driven config requires non-empty agents")
	}

	roles, err := indexRoles(cfg.Roles)
	if err != nil {
		return err
	}
	for _, role := range cfg.Roles {
		roleName := strings.TrimSpace(role.Name)
		agentName := strings.TrimSpace(role.Agent)
		agent, ok := agents[agentName]
		if !ok {
			return fmt.Errorf("role %q references missing agent %q", roleName, agentName)
		}
		if !capabilitySubset(role.Capabilities, agent.CapabilitiesMax) {
			return fmt.Errorf("role %q capabilities exceed agent %q capabilities_max", roleName, agentName)
		}
	}

	if err := validateRoleRef("role_bindings.team_leader.role", cfg.RoleBinds.TeamLeader.Role, roles); err != nil {
		return err
	}
	if err := validateRoleRef("role_bindings.plan_parser.role", cfg.RoleBinds.PlanParser.Role, roles); err != nil {
		return err
	}
	for stage, roleName := range cfg.RoleBinds.Run.StageRoles {
		if err := validateRoleRef("role_bindings.Run.stage_roles."+stage, roleName, roles); err != nil {
			return err
		}
	}
	for reviewer, roleName := range cfg.RoleBinds.ReviewOrchestrator.Reviewers {
		if err := validateRoleRef("role_bindings.review_orchestrator.reviewers."+reviewer, roleName, roles); err != nil {
			return err
		}
	}
	if err := validateRoleRef("role_bindings.review_orchestrator.aggregator", cfg.RoleBinds.ReviewOrchestrator.Aggregator, roles); err != nil {
		return err
	}

	return nil
}

func validateV2MCPConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	profileIDs := make(map[string]struct{}, len(cfg.V2.Agents.Profiles))
	for _, profile := range cfg.V2.Agents.Profiles {
		id := strings.TrimSpace(profile.ID)
		if id == "" {
			continue
		}
		profileIDs[id] = struct{}{}
	}

	serverIDs := make(map[string]struct{}, len(cfg.V2.MCP.Servers))
	for _, server := range cfg.V2.MCP.Servers {
		id := strings.TrimSpace(server.ID)
		if id == "" {
			return fmt.Errorf("v2.mcp.servers.id is required")
		}
		if _, exists := serverIDs[id]; exists {
			return fmt.Errorf("duplicate v2.mcp.servers id %q", id)
		}
		serverIDs[id] = struct{}{}

		kind := strings.ToLower(strings.TrimSpace(server.Kind))
		transport := strings.ToLower(strings.TrimSpace(server.Transport))
		switch kind {
		case "", "internal", "external":
		default:
			return fmt.Errorf("v2.mcp.servers[%q].kind must be internal or external", id)
		}
		switch transport {
		case "stdio":
			if strings.TrimSpace(server.Command) == "" {
				return fmt.Errorf("v2.mcp.servers[%q].command is required for stdio transport", id)
			}
		case "sse":
			if kind != "internal" && strings.TrimSpace(server.Endpoint) == "" {
				return fmt.Errorf("v2.mcp.servers[%q].endpoint is required for sse transport", id)
			}
		default:
			return fmt.Errorf("v2.mcp.servers[%q].transport must be stdio or sse", id)
		}
	}

	for _, binding := range cfg.V2.MCP.ProfileBindings {
		profile := strings.TrimSpace(binding.Profile)
		if profile == "" {
			return fmt.Errorf("v2.mcp.profile_bindings.profile is required")
		}
		if _, ok := profileIDs[profile]; !ok {
			return fmt.Errorf("v2.mcp.profile_bindings references missing profile %q", profile)
		}
		server := strings.TrimSpace(binding.Server)
		if server == "" {
			return fmt.Errorf("v2.mcp.profile_bindings.server is required")
		}
		if _, ok := serverIDs[server]; !ok {
			return fmt.Errorf("v2.mcp.profile_bindings for profile %q references missing server %q", profile, server)
		}
		mode := strings.ToLower(strings.TrimSpace(binding.ToolMode))
		switch mode {
		case "", "all":
		case "allow_list":
			if len(binding.Tools) == 0 {
				return fmt.Errorf("v2.mcp.profile_bindings for profile %q and server %q requires tools when tool_mode=allow_list", profile, server)
			}
		default:
			return fmt.Errorf("v2.mcp.profile_bindings for profile %q and server %q has invalid tool_mode %q", profile, server, binding.ToolMode)
		}
		if hasDuplicateStrings(binding.Tools) {
			return fmt.Errorf("v2.mcp.profile_bindings for profile %q and server %q contains duplicate tools", profile, server)
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

func hasRoleDrivenData(cfg *Config) bool {
	if len(cfg.Agents.Profiles) > 0 || len(cfg.Roles) > 0 {
		return true
	}
	if strings.TrimSpace(cfg.RoleBinds.TeamLeader.Role) != "" {
		return true
	}
	if strings.TrimSpace(cfg.RoleBinds.PlanParser.Role) != "" {
		return true
	}
	if strings.TrimSpace(cfg.RoleBinds.ReviewOrchestrator.Aggregator) != "" {
		return true
	}
	return len(cfg.RoleBinds.Run.StageRoles) > 0 || len(cfg.RoleBinds.ReviewOrchestrator.Reviewers) > 0
}

func indexAgents(agents []AgentProfileConfig) (map[string]AgentProfileConfig, error) {
	out := make(map[string]AgentProfileConfig, len(agents))
	for _, agent := range agents {
		name := strings.TrimSpace(agent.Name)
		if name == "" {
			return nil, fmt.Errorf("agent.name is required")
		}
		if strings.TrimSpace(agent.LaunchCommand) == "" {
			return nil, fmt.Errorf("agent %q launch_command is required", name)
		}
		if _, exists := out[name]; exists {
			return nil, fmt.Errorf("duplicate agent name %q", name)
		}
		cloned := agent
		cloned.Name = name
		out[name] = cloned
	}
	return out, nil
}

func indexRoles(roles []RoleConfig) (map[string]RoleConfig, error) {
	out := make(map[string]RoleConfig, len(roles))
	for _, role := range roles {
		name := strings.TrimSpace(role.Name)
		if name == "" {
			return nil, fmt.Errorf("role.name is required")
		}
		agent := strings.TrimSpace(role.Agent)
		if agent == "" {
			return nil, fmt.Errorf("role %q agent is required", name)
		}
		if _, exists := out[name]; exists {
			return nil, fmt.Errorf("duplicate role name %q", name)
		}
		role.Name = name
		role.Agent = agent
		out[name] = role
	}
	return out, nil
}

func validateRoleRef(path, roleName string, roles map[string]RoleConfig) error {
	name := strings.TrimSpace(roleName)
	if name == "" {
		return nil
	}
	if _, ok := roles[name]; !ok {
		return fmt.Errorf("%s references missing role %q", path, name)
	}
	return nil
}

func capabilitySubset(roleCaps, maxCaps CapabilitiesConfig) bool {
	if roleCaps.FSRead && !maxCaps.FSRead {
		return false
	}
	if roleCaps.FSWrite && !maxCaps.FSWrite {
		return false
	}
	if roleCaps.Terminal && !maxCaps.Terminal {
		return false
	}
	return true
}

func (cfg Config) EffectiveAgentProfiles() []AgentProfileConfig {
	if len(cfg.Agents.Profiles) > 0 {
		return cloneAgentProfiles(cfg.Agents.Profiles)
	}
	return legacyAgentProfiles(cfg.Agents)
}

func legacyAgentProfiles(agents AgentsConfig) []AgentProfileConfig {
	out := make([]AgentProfileConfig, 0, 3)
	appendLegacy := func(name string, cfg *AgentConfig) {
		if cfg == nil {
			return
		}
		cmd := ""
		if cfg.Binary != nil {
			cmd = strings.TrimSpace(*cfg.Binary)
		}
		if cmd == "" {
			return
		}
		caps := CapabilitiesConfig{FSRead: true, FSWrite: true, Terminal: true}
		if cfg.CapabilitiesMax != nil {
			caps = *cfg.CapabilitiesMax
		}
		out = append(out, AgentProfileConfig{
			Name:            name,
			LaunchCommand:   cmd,
			LaunchArgs:      nil,
			Env:             nil,
			CapabilitiesMax: caps,
		})
	}
	appendLegacy("claude", agents.Claude)
	appendLegacy("codex", agents.Codex)
	appendLegacy("openspec", agents.OpenSpec)
	return out
}
