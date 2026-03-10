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

// Action represents an operation an agent can perform.
type Action string

const (
	ActionReadContext  Action = "read_context"
	ActionSearchFiles  Action = "search_files"
	ActionFSWrite      Action = "fs_write"
	ActionTerminal     Action = "terminal"
	ActionSubmit       Action = "submit"
	ActionMarkBlocked  Action = "mark_blocked"
	ActionRequestHelp  Action = "request_help"
	ActionApprove      Action = "approve"
	ActionReject       Action = "reject"
	ActionCreateStep   Action = "create_step"
	ActionExpandFlow   Action = "expand_flow"
)

// AgentProfile defines an agent's identity, role, capabilities, and constraints.
// It references an AgentDriver by DriverID for process launch configuration.
type AgentProfile struct {
	ID             string    `json:"id"`
	Name           string    `json:"name,omitempty"`
	DriverID       string    `json:"driver_id"`
	Role           AgentRole `json:"role"`
	Capabilities   []string  `json:"capabilities,omitempty"`    // capability tags (dev.backend, test.qa, ...)
	ActionsAllowed []Action  `json:"actions_allowed,omitempty"` // permitted actions
	PromptTemplate string    `json:"prompt_template,omitempty"`

	Session ProfileSession `json:"session,omitempty"`
	MCP     ProfileMCP     `json:"mcp,omitempty"`
}

// ProfileSession configures session management for this profile.
type ProfileSession struct {
	Reuse    bool          `json:"reuse,omitempty"`
	MaxTurns int           `json:"max_turns,omitempty"`
	IdleTTL  time.Duration `json:"idle_ttl,omitempty"`
}

// ProfileMCP configures MCP tool access for this profile.
type ProfileMCP struct {
	Enabled bool     `json:"enabled,omitempty"`
	Tools   []string `json:"tools,omitempty"`
}

// DefaultActions returns the default action whitelist for a role.
func DefaultActions(role AgentRole) []Action {
	common := []Action{ActionReadContext, ActionSearchFiles, ActionSubmit, ActionMarkBlocked, ActionRequestHelp}
	switch role {
	case RoleLead:
		return append(common, ActionFSWrite, ActionTerminal, ActionCreateStep, ActionExpandFlow)
	case RoleWorker:
		return append(common, ActionFSWrite, ActionTerminal)
	case RoleGate:
		return append(common, ActionApprove, ActionReject)
	case RoleSupport:
		return common
	default:
		return common
	}
}

// HasAction checks if the profile permits the given action.
func (p *AgentProfile) HasAction(action Action) bool {
	actions := p.ActionsAllowed
	if len(actions) == 0 {
		actions = DefaultActions(p.Role)
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
	for _, a := range p.EffectiveActions() {
		switch a {
		case ActionReadContext, ActionSearchFiles:
			caps.FSRead = true
		case ActionFSWrite:
			caps.FSWrite = true
		case ActionTerminal:
			caps.Terminal = true
		}
	}
	return caps
}

// EffectiveActions returns ActionsAllowed if set, otherwise DefaultActions for the role.
func (p *AgentProfile) EffectiveActions() []Action {
	if len(p.ActionsAllowed) > 0 {
		return p.ActionsAllowed
	}
	return DefaultActions(p.Role)
}
