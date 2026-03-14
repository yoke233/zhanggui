package agent

import (
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

// NewConfigRegistryFromConfig creates a ConfigRegistry populated from TOML configuration.
func NewConfigRegistryFromConfig(cfg config.RuntimeAgentsConfig) *ConfigRegistry {
	reg := NewConfigRegistry()
	reg.LoadProfiles(convertProfilesFromConfig(cfg.Drivers, cfg.Profiles))
	return reg
}

func convertProfilesFromConfig(driverCfgs []config.RuntimeDriverConfig, profileCfgs []config.RuntimeProfileConfig) []*core.AgentProfile {
	// Build driver lookup map.
	driverMap := make(map[string]config.RuntimeDriverConfig, len(driverCfgs))
	for _, d := range driverCfgs {
		driverMap[d.ID] = d
	}

	out := make([]*core.AgentProfile, len(profileCfgs))
	for i, c := range profileCfgs {
		actions := make([]core.AgentAction, len(c.ActionsAllowed))
		for j, a := range c.ActionsAllowed {
			actions[j] = core.AgentAction(a)
		}
		var driverCfg core.DriverConfig
		if d, ok := driverMap[c.Driver]; ok {
			driverCfg = core.DriverConfig{
				LaunchCommand:   d.LaunchCommand,
				LaunchArgs:      d.LaunchArgs,
				Env:             d.Env,
				CapabilitiesMax: core.DriverCapabilities{
					FSRead:   d.CapabilitiesMax.FSRead,
					FSWrite:  d.CapabilitiesMax.FSWrite,
					Terminal: d.CapabilitiesMax.Terminal,
				},
			}
		}
		out[i] = &core.AgentProfile{
			ID:             c.ID,
			Name:           c.Name,
			Driver:         driverCfg,
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
