package agent

import (
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

// NewConfigRegistryFromConfig creates a ConfigRegistry populated from TOML configuration.
func NewConfigRegistryFromConfig(cfg config.RuntimeAgentsConfig) *ConfigRegistry {
	reg := NewConfigRegistry()
	reg.LoadDrivers(convertDrivers(cfg.Drivers))
	reg.LoadProfiles(convertProfiles(cfg.Profiles))
	return reg
}

func convertDrivers(cfgs []config.RuntimeDriverConfig) []*core.AgentDriver {
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

func convertProfiles(cfgs []config.RuntimeProfileConfig) []*core.AgentProfile {
	out := make([]*core.AgentProfile, len(cfgs))
	for i, c := range cfgs {
		actions := make([]core.AgentAction, len(c.ActionsAllowed))
		for j, a := range c.ActionsAllowed {
			actions[j] = core.AgentAction(a)
		}
		out[i] = &core.AgentProfile{
			ID:             c.ID,
			Name:           c.Name,
			DriverID:       c.Driver,
			Role:           core.AgentRole(c.Role),
			Capabilities:   c.Capabilities,
			ActionsAllowed: actions,
			PromptTemplate: c.PromptTemplate,
			Skills:         c.Skills,
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
