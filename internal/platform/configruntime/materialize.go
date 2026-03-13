package configruntime

import (
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func BuildAgents(cfg *config.Config) ([]*core.AgentDriver, []*core.AgentProfile) {
	if cfg == nil {
		return nil, nil
	}

	drivers := convertDrivers(cfg.Runtime.Agents.Drivers)
	profiles := convertProfiles(cfg.Runtime.Agents.Profiles)
	return drivers, profiles
}

func convertDrivers(cfgs []config.RuntimeDriverConfig) []*core.AgentDriver {
	out := make([]*core.AgentDriver, len(cfgs))
	for i, c := range cfgs {
		out[i] = &core.AgentDriver{
			ID:            c.ID,
			LaunchCommand: c.LaunchCommand,
			LaunchArgs:    append([]string(nil), c.LaunchArgs...),
			Env:           cloneStringMap(c.Env),
			CapabilitiesMax: core.DriverCapabilities{
				FSRead:   c.CapabilitiesMax.FSRead,
				FSWrite:  c.CapabilitiesMax.FSWrite,
				Terminal: c.CapabilitiesMax.Terminal,
			},
		}
	}
	return out
}

func convertProfiles(cfgs []config.RuntimeProfileConfig) []*core.AgentProfile {
	out := make([]*core.AgentProfile, len(cfgs))
	for i, c := range cfgs {
		actions := make([]core.AgentAction, len(c.ActionsAllowed))
		for j, action := range c.ActionsAllowed {
			actions[j] = core.AgentAction(action)
		}
		out[i] = &core.AgentProfile{
			ID:             c.ID,
			Name:           c.Name,
			DriverID:       c.Driver,
			Role:           core.AgentRole(c.Role),
			Capabilities:   append([]string(nil), c.Capabilities...),
			ActionsAllowed: actions,
			PromptTemplate: c.PromptTemplate,
			Skills:         append([]string(nil), c.Skills...),
			Session: core.ProfileSession{
				Reuse:              c.Session.Reuse,
				MaxTurns:           c.Session.MaxTurns,
				IdleTTL:            c.Session.IdleTTL.Duration,
				ThreadBootTemplate: c.Session.ThreadBootTemplate,
				MaxContextTokens:   c.Session.MaxContextTokens,
				ContextWarnRatio:   c.Session.ContextWarnRatio,
			},
			MCP: core.ProfileMCP{
				Enabled: c.MCP.Enabled,
				Tools:   append([]string(nil), c.MCP.Tools...),
			},
		}
	}
	return out
}
