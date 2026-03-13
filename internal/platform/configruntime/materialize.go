package configruntime

import (
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func BuildAgents(cfg *config.Config) ([]*core.AgentDriver, []*core.AgentProfile) {
	if cfg == nil {
		return nil, nil
	}

	drivers := convertDrivers(cfg.Runtime.Agents.Drivers)
	profiles := convertProfiles(cfg.Runtime.Agents.Profiles)
	return ensurePRReviewer(drivers, profiles)
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

func ensurePRReviewer(drivers []*core.AgentDriver, profiles []*core.AgentProfile) ([]*core.AgentDriver, []*core.AgentProfile) {
	const reviewerID = "pr-reviewer"
	for _, profile := range profiles {
		if profile != nil && profile.ID == reviewerID {
			return drivers, profiles
		}
	}

	hasCodex := false
	for _, driver := range drivers {
		if driver != nil && strings.TrimSpace(driver.ID) == "codex" {
			hasCodex = true
			break
		}
	}
	if !hasCodex {
		drivers = append(drivers, &core.AgentDriver{
			ID:            "codex",
			LaunchCommand: "npx",
			LaunchArgs:    []string{"-y", "@zed-industries/codex-acp"},
			CapabilitiesMax: core.DriverCapabilities{
				FSRead:   true,
				FSWrite:  true,
				Terminal: true,
			},
		})
	}

	profiles = append(profiles, &core.AgentProfile{
		ID:           reviewerID,
		Name:         "PR Reviewer (Codex)",
		DriverID:     "codex",
		Role:         core.RoleGate,
		Capabilities: []string{"prreview"},
		ActionsAllowed: []core.AgentAction{
			core.AgentActionReadContext,
			core.AgentActionSearchFiles,
			core.AgentActionTerminal,
			core.AgentActionApprove,
			core.AgentActionReject,
			core.AgentActionSubmit,
		},
		PromptTemplate: "review",
		Session: core.ProfileSession{
			Reuse:    true,
			MaxTurns: 12,
		},
	})
	return drivers, profiles
}
