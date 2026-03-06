package acpclient

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrRoleResolverNil        = errors.New("role resolver is nil")
	ErrRoleNotFound           = errors.New("role not found")
	ErrAgentNotFound          = errors.New("agent not found")
	ErrRoleCapabilityOverflow = errors.New("role capabilities exceed agent capabilities_max")
)

type AgentProfile struct {
	ID              string
	LaunchCommand   string
	LaunchArgs      []string
	Env             map[string]string
	CapabilitiesMax ClientCapabilities
}

type SessionPolicy struct {
	Reuse             bool
	PreferLoadSession bool
	MaxTurns          int
	ResetPrompt       bool
	SessionIdleTTL    time.Duration
}

type PermissionRule struct {
	Pattern string
	Scope   string
	Action  string
}

type RoleProfile struct {
	ID               string
	AgentID          string
	PromptTemplate   string
	SessionPolicy    SessionPolicy
	Capabilities     ClientCapabilities
	PermissionPolicy []PermissionRule
	MCPEnabled       bool
	MCPTools         []string
}

type RoleResolver struct {
	agents map[string]AgentProfile
	roles  map[string]RoleProfile
}

func NewRoleResolver(agents []AgentProfile, roles []RoleProfile) *RoleResolver {
	resolver := &RoleResolver{
		agents: make(map[string]AgentProfile, len(agents)),
		roles:  make(map[string]RoleProfile, len(roles)),
	}
	for _, agent := range agents {
		resolver.agents[agent.ID] = cloneAgentProfile(agent)
	}
	for _, role := range roles {
		resolver.roles[role.ID] = cloneRoleProfile(role)
	}
	return resolver
}

func (r *RoleResolver) Resolve(roleID string) (AgentProfile, RoleProfile, error) {
	if r == nil {
		return AgentProfile{}, RoleProfile{}, ErrRoleResolverNil
	}
	if roleID == "" {
		return AgentProfile{}, RoleProfile{}, fmt.Errorf("%w: empty role id", ErrRoleNotFound)
	}

	role, ok := r.roles[roleID]
	if !ok {
		return AgentProfile{}, RoleProfile{}, fmt.Errorf("%w: role %q", ErrRoleNotFound, roleID)
	}
	if role.AgentID == "" {
		return AgentProfile{}, RoleProfile{}, fmt.Errorf("%w: role %q has empty agent", ErrAgentNotFound, roleID)
	}

	agent, ok := r.agents[role.AgentID]
	if !ok {
		return AgentProfile{}, RoleProfile{}, fmt.Errorf("%w: role %q -> agent %q", ErrAgentNotFound, roleID, role.AgentID)
	}

	overflows := capabilityOverflows(role.Capabilities, agent.CapabilitiesMax)
	if len(overflows) > 0 {
		return AgentProfile{}, RoleProfile{}, fmt.Errorf(
			"%w: role %q -> agent %q, overflows=%s",
			ErrRoleCapabilityOverflow,
			roleID,
			role.AgentID,
			strings.Join(overflows, ","),
		)
	}

	return cloneAgentProfile(agent), cloneRoleProfile(role), nil
}

func capabilityOverflows(roleCaps ClientCapabilities, maxCaps ClientCapabilities) []string {
	overflows := make([]string, 0, 3)
	if roleCaps.FSRead && !maxCaps.FSRead {
		overflows = append(overflows, "fs_read")
	}
	if roleCaps.FSWrite && !maxCaps.FSWrite {
		overflows = append(overflows, "fs_write")
	}
	if roleCaps.Terminal && !maxCaps.Terminal {
		overflows = append(overflows, "terminal")
	}
	sort.Strings(overflows)
	return overflows
}

func cloneAgentProfile(agent AgentProfile) AgentProfile {
	cloned := agent
	if agent.LaunchArgs != nil {
		cloned.LaunchArgs = append([]string(nil), agent.LaunchArgs...)
	}
	if agent.Env != nil {
		cloned.Env = make(map[string]string, len(agent.Env))
		for k, v := range agent.Env {
			cloned.Env[k] = v
		}
	}
	return cloned
}

func cloneRoleProfile(role RoleProfile) RoleProfile {
	cloned := role
	if role.PermissionPolicy != nil {
		cloned.PermissionPolicy = append([]PermissionRule(nil), role.PermissionPolicy...)
	}
	if role.MCPTools != nil {
		cloned.MCPTools = append([]string(nil), role.MCPTools...)
	}
	return cloned
}
