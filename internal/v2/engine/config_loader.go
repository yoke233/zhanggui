package engine

import (
	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// NewConfigRegistryFromConfig creates a ConfigRegistry populated from TOML configuration.
func NewConfigRegistryFromConfig(cfg config.V2AgentsConfig) *ConfigRegistry {
	reg := NewConfigRegistry()
	reg.LoadDrivers(convertDrivers(cfg.Drivers))
	reg.LoadProfiles(convertProfiles(cfg.Profiles))
	return reg
}

func convertDrivers(cfgs []config.V2DriverConfig) []*core.AgentDriver {
	out := make([]*core.AgentDriver, len(cfgs))
	for i, c := range cfgs {
		out[i] = &core.AgentDriver{
			ID:            c.ID,
			LaunchCommand: c.LaunchCommand,
			LaunchArgs:    c.LaunchArgs,
			Env:           c.Env,
			CapabilitiesMax: core.DriverCapabilities{
				FSRead:   c.CapabilitiesMax.FSRead,
				FSWrite:  c.CapabilitiesMax.FSWrite,
				Terminal: c.CapabilitiesMax.Terminal,
			},
		}
	}
	return out
}

func convertProfiles(cfgs []config.V2ProfileConfig) []*core.AgentProfile {
	out := make([]*core.AgentProfile, len(cfgs))
	for i, c := range cfgs {
		actions := make([]core.Action, len(c.ActionsAllowed))
		for j, a := range c.ActionsAllowed {
			actions[j] = core.Action(a)
		}
		out[i] = &core.AgentProfile{
			ID:             c.ID,
			Name:           c.Name,
			DriverID:       c.Driver,
			Role:           core.AgentRole(c.Role),
			Capabilities:   c.Capabilities,
			ActionsAllowed: actions,
			PromptTemplate: c.PromptTemplate,
			Session: core.ProfileSession{
				Reuse:    c.Session.Reuse,
				MaxTurns: c.Session.MaxTurns,
				IdleTTL:  c.Session.IdleTTL.Duration,
			},
			MCP: core.ProfileMCP{
				Enabled: c.MCP.Enabled,
				Tools:   c.MCP.Tools,
			},
		}
	}
	return out
}
