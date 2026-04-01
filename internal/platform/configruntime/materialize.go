package configruntime

import (
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/platform/config"
	"github.com/yoke233/zhanggui/internal/platform/profilellm"
)

// CoreProfileToRuntimeConfig converts a core.AgentProfile back to
// config.RuntimeProfileConfig for persistence into config.toml.
func CoreProfileToRuntimeConfig(p *core.AgentProfile) config.RuntimeProfileConfig {
	actions := make([]string, len(p.ActionsAllowed))
	for i, a := range p.ActionsAllowed {
		actions[i] = string(a)
	}
	return config.RuntimeProfileConfig{
		ID:               p.ID,
		Name:             p.Name,
		ManagerProfileID: p.ManagerProfileID,
		Driver:           p.DriverID,
		LLMConfigID:      p.LLMConfigID,
		Role:             string(p.Role),
		Capabilities:     append([]string(nil), p.Capabilities...),
		ActionsAllowed:   actions,
		PromptTemplate:   p.PromptTemplate,
		Skills:           append([]string(nil), p.Skills...),
		Session: config.RuntimeSessionConfig{
			Reuse:              p.Session.Reuse,
			MaxTurns:           p.Session.MaxTurns,
			IdleTTL:            config.Duration{Duration: p.Session.IdleTTL},
			ThreadBootTemplate: p.Session.ThreadBootTemplate,
			MaxContextTokens:   p.Session.MaxContextTokens,
			ContextWarnRatio:   p.Session.ContextWarnRatio,
		},
		MCP: config.MCPConfig{
			Enabled: p.MCP.Enabled,
			Tools:   append([]string(nil), p.MCP.Tools...),
		},
	}
}

func BuildAgents(cfg *config.Config) []*core.AgentProfile {
	if cfg == nil {
		return nil
	}

	return convertProfiles(cfg.Runtime.Agents.Drivers, cfg.Runtime.Agents.Profiles, cfg.Runtime.LLM.Configs)
}

func convertProfiles(driverCfgs []config.RuntimeDriverConfig, profileCfgs []config.RuntimeProfileConfig, llmCfgs []config.RuntimeLLMEntryConfig) []*core.AgentProfile {
	// Build driver lookup map.
	driverMap := make(map[string]config.RuntimeDriverConfig, len(driverCfgs))
	for _, d := range driverCfgs {
		driverMap[d.ID] = normalizeDriverConfigForPlatform(d)
	}
	llmMap := make(map[string]config.RuntimeLLMEntryConfig, len(llmCfgs))
	for _, item := range llmCfgs {
		llmMap[item.ID] = item
	}

	out := make([]*core.AgentProfile, len(profileCfgs))
	for i, c := range profileCfgs {
		actions := make([]core.AgentAction, len(c.ActionsAllowed))
		for j, action := range c.ActionsAllowed {
			actions[j] = core.AgentAction(action)
		}
		var driverCfg core.DriverConfig
		if d, ok := driverMap[c.Driver]; ok {
			driverCfg = core.DriverConfig{
				ID:            d.ID,
				LaunchCommand: d.LaunchCommand,
				LaunchArgs:    append([]string(nil), d.LaunchArgs...),
				SandboxArgs:   append([]string(nil), d.SandboxArgs...),
				Env:           config.CloneStringMap(d.Env),
				CapabilitiesMax: core.DriverCapabilities{
					FSRead:   d.CapabilitiesMax.FSRead,
					FSWrite:  d.CapabilitiesMax.FSWrite,
					Terminal: d.CapabilitiesMax.Terminal,
				},
			}
			if llmCfg, ok := llmMap[c.LLMConfigID]; ok {
				driverCfg.Env = profilellm.MergeEnv(profilellm.BuildEnv(NewProviderConfigFromEntry(&llmCfg)), driverCfg.Env)
			}
		}
		out[i] = &core.AgentProfile{
			ID:               c.ID,
			Name:             c.Name,
			ManagerProfileID: c.ManagerProfileID,
			DriverID:         c.Driver,
			LLMConfigID:      c.LLMConfigID,
			Driver:           driverCfg,
			Role:             core.AgentRole(c.Role),
			Capabilities:     append([]string(nil), c.Capabilities...),
			ActionsAllowed:   actions,
			PromptTemplate:   c.PromptTemplate,
			Skills:           append([]string(nil), c.Skills...),
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
