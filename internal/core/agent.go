package core

import "time"

// AgentRole is the role classification for an agent.
type AgentRole string

const (
	RoleLead    AgentRole = "lead"
	RoleWorker  AgentRole = "worker"
	RoleGate    AgentRole = "gate"
	RoleSupport AgentRole = "support"
)

// AgentAction represents an operation an agent can perform.
type AgentAction string

const (
	AgentActionReadContext AgentAction = "read_context"
	AgentActionSearchFiles AgentAction = "search_files"
	AgentActionFSWrite     AgentAction = "fs_write"
	AgentActionTerminal    AgentAction = "terminal"
	AgentActionSubmit      AgentAction = "submit"
	AgentActionMarkBlocked AgentAction = "mark_blocked"
	AgentActionRequestHelp AgentAction = "request_help"
	AgentActionApprove     AgentAction = "approve"
	AgentActionReject      AgentAction = "reject"
	AgentActionCreateStep  AgentAction = "create_step"
	AgentActionExpandFlow  AgentAction = "expand_flow"
)

// DriverConfig holds process-level launch configuration for an agent profile.
type DriverConfig struct {
	LaunchCommand   string             `json:"launch_command"`
	LaunchArgs      []string           `json:"launch_args,omitempty"`
	Env             map[string]string  `json:"env,omitempty"`
	CapabilitiesMax DriverCapabilities `json:"capabilities_max"`
}

// AgentProfile defines an agent's identity, role, capabilities, and constraints.
// Driver configuration is embedded directly via the Driver field.
type AgentProfile struct {
	ID             string       `json:"id"`
	Name           string       `json:"name,omitempty"`
	Driver         DriverConfig `json:"driver"`
	Role           AgentRole    `json:"role"`
	Capabilities   []string  `json:"capabilities,omitempty"`    // capability tags (backend, qa, review, ...)
	ActionsAllowed []AgentAction `json:"actions_allowed,omitempty"` // permitted actions
	PromptTemplate string    `json:"prompt_template,omitempty"`
	Skills         []string  `json:"skills,omitempty"` // skill folder names to enable for this profile

	Session ProfileSession `json:"session,omitempty"`
	MCP     ProfileMCP     `json:"mcp,omitempty"`
}

// ProfileSession configures session management for this profile.
type ProfileSession struct {
	Reuse             bool          `json:"reuse,omitempty"`
	MaxTurns          int           `json:"max_turns,omitempty"`
	IdleTTL           time.Duration `json:"idle_ttl,omitempty"`
	ThreadBootTemplate string       `json:"thread_boot_template,omitempty"`
	MaxContextTokens  int64         `json:"max_context_tokens,omitempty"`
	ContextWarnRatio  float64       `json:"context_warn_ratio,omitempty"` // default 0.8
}

// ProfileMCP configures MCP tool access for this profile.
type ProfileMCP struct {
	Enabled bool     `json:"enabled,omitempty"`
	Tools   []string `json:"tools,omitempty"`
}

// DefaultAgentActions returns the default action whitelist for a role.
func DefaultAgentActions(role AgentRole) []AgentAction {
	common := []AgentAction{AgentActionReadContext, AgentActionSearchFiles, AgentActionSubmit, AgentActionMarkBlocked, AgentActionRequestHelp}
	switch role {
	case RoleLead:
		return append(common, AgentActionFSWrite, AgentActionTerminal, AgentActionCreateStep, AgentActionExpandFlow)
	case RoleWorker:
		return append(common, AgentActionFSWrite, AgentActionTerminal)
	case RoleGate:
		return append(common, AgentActionApprove, AgentActionReject)
	case RoleSupport:
		return common
	default:
		return common
	}
}

// HasAgentAction checks if the profile permits the given action.
func (p *AgentProfile) HasAgentAction(action AgentAction) bool {
	actions := p.ActionsAllowed
	if len(actions) == 0 {
		actions = DefaultAgentActions(p.Role)
	}
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}

// HasCapability checks if the profile has the given capability tag.
func (p *AgentProfile) HasCapability(cap string) bool {
	for _, c := range p.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// MatchesRequirements checks if the profile satisfies all required capability tags.
func (p *AgentProfile) MatchesRequirements(required []string) bool {
	for _, req := range required {
		if !p.HasCapability(req) {
			return false
		}
	}
	return true
}

// EffectiveCapabilities returns the ACP capabilities derived from the profile's actions.
func (p *AgentProfile) EffectiveCapabilities() DriverCapabilities {
	var caps DriverCapabilities
	for _, a := range p.EffectiveAgentActions() {
		switch a {
		case AgentActionReadContext, AgentActionSearchFiles:
			caps.FSRead = true
		case AgentActionFSWrite:
			caps.FSWrite = true
		case AgentActionTerminal:
			caps.Terminal = true
		}
	}
	return caps
}

// EffectiveAgentActions returns ActionsAllowed if set, otherwise DefaultAgentActions for the role.
func (p *AgentProfile) EffectiveAgentActions() []AgentAction {
	if len(p.ActionsAllowed) > 0 {
		return p.ActionsAllowed
	}
	return DefaultAgentActions(p.Role)
}
