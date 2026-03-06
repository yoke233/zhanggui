package config

import "fmt"

func MergeAgentConfig(base, override *AgentConfig) *AgentConfig {
	if override == nil {
		return base
	}
	if base == nil {
		return override
	}

	out := *base
	if override.Plugin != nil {
		out.Plugin = override.Plugin
	}
	if override.Binary != nil {
		out.Binary = override.Binary
	}
	if override.MaxTurns != nil {
		out.MaxTurns = override.MaxTurns
	}
	if override.DefaultTools != nil {
		out.DefaultTools = cloneStringSlicePtr(override.DefaultTools)
	}
	if override.Model != nil {
		out.Model = override.Model
	}
	if override.Reasoning != nil {
		out.Reasoning = override.Reasoning
	}
	if override.Sandbox != nil {
		out.Sandbox = override.Sandbox
	}
	if override.Approval != nil {
		out.Approval = override.Approval
	}
	if override.CapabilitiesMax != nil {
		caps := *override.CapabilitiesMax
		out.CapabilitiesMax = &caps
	}
	return &out
}

func MergeForRun(global *Config, project *ConfigLayer, override map[string]any) (*Config, error) {
	merged := Defaults()
	if global != nil {
		merged = cloneConfig(*global)
	}

	ApplyConfigLayer(&merged, project)

	if len(override) > 0 {
		layer, err := decodeLayerFromMap(override)
		if err != nil {
			return nil, fmt.Errorf("decode Run override: %w", err)
		}
		ApplyConfigLayer(&merged, layer)
	}

	if err := ApplyEnvOverrides(&merged); err != nil {
		return nil, err
	}
	if err := validateConfig(&merged); err != nil {
		return nil, err
	}
	return &merged, nil
}

func cloneConfig(in Config) Config {
	out := in
	out.Agents = AgentsConfig{
		Claude:   cloneAgentConfig(in.Agents.Claude),
		Codex:    cloneAgentConfig(in.Agents.Codex),
		OpenSpec: cloneAgentConfig(in.Agents.OpenSpec),
		Profiles: cloneAgentProfiles(in.Agents.Profiles),
	}
	out.Roles = cloneRoles(in.Roles)
	out.RoleBinds = cloneRoleBindings(in.RoleBinds)
	out.GitHub = cloneGitHubConfig(in.GitHub)
	return out
}

func cloneAgentConfig(in *AgentConfig) *AgentConfig {
	if in == nil {
		return nil
	}
	out := *in
	if in.Binary != nil {
		out.Binary = ptrValue(*in.Binary)
	}
	if in.Plugin != nil {
		out.Plugin = ptrValue(*in.Plugin)
	}
	if in.MaxTurns != nil {
		out.MaxTurns = ptrValue(*in.MaxTurns)
	}
	if in.DefaultTools != nil {
		out.DefaultTools = cloneStringSlicePtr(in.DefaultTools)
	}
	if in.Model != nil {
		out.Model = ptrValue(*in.Model)
	}
	if in.Reasoning != nil {
		out.Reasoning = ptrValue(*in.Reasoning)
	}
	if in.Sandbox != nil {
		out.Sandbox = ptrValue(*in.Sandbox)
	}
	if in.Approval != nil {
		out.Approval = ptrValue(*in.Approval)
	}
	if in.CapabilitiesMax != nil {
		caps := *in.CapabilitiesMax
		out.CapabilitiesMax = &caps
	}
	return &out
}

func ApplyConfigLayer(cfg *Config, layer *ConfigLayer) {
	if cfg == nil || layer == nil {
		return
	}

	if agents := layer.Agents; agents != nil {
		cfg.Agents.Claude = MergeAgentConfig(cfg.Agents.Claude, agents.Claude)
		cfg.Agents.Codex = MergeAgentConfig(cfg.Agents.Codex, agents.Codex)
		cfg.Agents.OpenSpec = MergeAgentConfig(cfg.Agents.OpenSpec, agents.OpenSpec)
		if agents.Profiles != nil {
			cfg.Agents.Profiles = cloneAgentProfiles(*agents.Profiles)
		}
	}

	if roles := layer.Roles; roles != nil {
		cfg.Roles = cloneRoles(*roles)
	}

	if binds := layer.RoleBinds; binds != nil {
		if v := binds.TeamLeader; v != nil && v.Role != nil {
			cfg.RoleBinds.TeamLeader.Role = *v.Role
		}
		if v := binds.PlanParser; v != nil && v.Role != nil {
			cfg.RoleBinds.PlanParser.Role = *v.Role
		}
		if v := binds.Run; v != nil && v.StageRoles != nil {
			cfg.RoleBinds.Run.StageRoles = cloneStringMap(*v.StageRoles)
		}
		if v := binds.ReviewOrchestrator; v != nil {
			if v.Reviewers != nil {
				cfg.RoleBinds.ReviewOrchestrator.Reviewers = cloneStringMap(*v.Reviewers)
			}
			if v.Aggregator != nil {
				cfg.RoleBinds.ReviewOrchestrator.Aggregator = *v.Aggregator
			}
		}
	}

	if run := layer.Run; run != nil {
		if run.DefaultTemplate != nil {
			cfg.Run.DefaultTemplate = *run.DefaultTemplate
		}
		if run.GlobalTimeout != nil {
			cfg.Run.GlobalTimeout = *run.GlobalTimeout
		}
		if run.AutoInferTemplate != nil {
			cfg.Run.AutoInferTemplate = *run.AutoInferTemplate
		}
		if run.MaxTotalRetries != nil {
			cfg.Run.MaxTotalRetries = *run.MaxTotalRetries
		}
	}

	if scheduler := layer.Scheduler; scheduler != nil {
		if scheduler.MaxGlobalAgents != nil {
			cfg.Scheduler.MaxGlobalAgents = *scheduler.MaxGlobalAgents
		}
		if scheduler.MaxProjectRuns != nil {
			cfg.Scheduler.MaxProjectRuns = *scheduler.MaxProjectRuns
		}
	}

	if teamLeader := layer.TeamLeader; teamLeader != nil {
		if teamLeader.ReviewGatePlugin != nil {
			cfg.TeamLeader.ReviewGatePlugin = *teamLeader.ReviewGatePlugin
		}
		if panel := teamLeader.ReviewOrchestrator; panel != nil {
			if panel.MaxRounds != nil {
				cfg.TeamLeader.ReviewOrchestrator.MaxRounds = *panel.MaxRounds
			}
		}
		if dag := teamLeader.DAGScheduler; dag != nil {
			if dag.MaxConcurrentTasks != nil {
				cfg.TeamLeader.DAGScheduler.MaxConcurrentTasks = *dag.MaxConcurrentTasks
			}
		}
	}

	if a2a := layer.A2A; a2a != nil {
		if a2a.Enabled != nil {
			cfg.A2A.Enabled = *a2a.Enabled
		}
		if a2a.Token != nil {
			cfg.A2A.Token = *a2a.Token
		}
		if a2a.Version != nil {
			cfg.A2A.Version = *a2a.Version
		}
	}

	if server := layer.Server; server != nil {
		if server.Host != nil {
			cfg.Server.Host = *server.Host
		}
		if server.Port != nil {
			cfg.Server.Port = *server.Port
		}
	}

	if github := layer.GitHub; github != nil {
		if github.Enabled != nil {
			cfg.GitHub.Enabled = *github.Enabled
		}
		if github.Token != nil {
			cfg.GitHub.Token = *github.Token
		}
		if github.AppID != nil {
			cfg.GitHub.AppID = *github.AppID
		}
		if github.PrivateKeyPath != nil {
			cfg.GitHub.PrivateKeyPath = *github.PrivateKeyPath
		}
		if github.InstallationID != nil {
			cfg.GitHub.InstallationID = *github.InstallationID
		}
		if github.Owner != nil {
			cfg.GitHub.Owner = *github.Owner
		}
		if github.Repo != nil {
			cfg.GitHub.Repo = *github.Repo
		}
		if github.WebhookSecret != nil {
			cfg.GitHub.WebhookSecret = *github.WebhookSecret
		}
		if github.WebhookEnabled != nil {
			cfg.GitHub.WebhookEnabled = *github.WebhookEnabled
		}
		if github.PREnabled != nil {
			cfg.GitHub.PREnabled = *github.PREnabled
		}
		if github.LabelMapping != nil {
			cfg.GitHub.LabelMapping = cloneStringMap(*github.LabelMapping)
		}
		if github.AuthorizedUsernames != nil {
			cfg.GitHub.AuthorizedUsernames = cloneStringSlice(*github.AuthorizedUsernames)
		}
		if github.AutoTrigger != nil {
			cfg.GitHub.AutoTrigger = *github.AutoTrigger
		}
		if github.AllowPATFallback != nil {
			cfg.GitHub.AllowPATFallback = *github.AllowPATFallback
		}
		if pr := github.PR; pr != nil {
			if pr.AutoCreate != nil {
				cfg.GitHub.PR.AutoCreate = *pr.AutoCreate
			}
			if pr.Draft != nil {
				cfg.GitHub.PR.Draft = *pr.Draft
			}
			if pr.AutoMerge != nil {
				cfg.GitHub.PR.AutoMerge = *pr.AutoMerge
			}
			if pr.Reviewers != nil {
				cfg.GitHub.PR.Reviewers = cloneStringSlice(*pr.Reviewers)
			}
			if pr.Labels != nil {
				cfg.GitHub.PR.Labels = cloneStringSlice(*pr.Labels)
			}
			if pr.BranchPrefix != nil {
				cfg.GitHub.PR.BranchPrefix = *pr.BranchPrefix
			}
		}
	}

	if store := layer.Store; store != nil {
		if store.Driver != nil {
			cfg.Store.Driver = *store.Driver
		}
		if store.Path != nil {
			cfg.Store.Path = *store.Path
		}
	}

	if ctx := layer.Context; ctx != nil {
		if ctx.Provider != nil {
			cfg.Context.Provider = *ctx.Provider
		}
		if ctx.Path != nil {
			cfg.Context.Path = *ctx.Path
		}
	}

	if log := layer.Log; log != nil {
		if log.Level != nil {
			cfg.Log.Level = *log.Level
		}
		if log.File != nil {
			cfg.Log.File = *log.File
		}
		if log.MaxSizeMB != nil {
			cfg.Log.MaxSizeMB = *log.MaxSizeMB
		}
		if log.MaxAgeDays != nil {
			cfg.Log.MaxAgeDays = *log.MaxAgeDays
		}
	}
}

func cloneRoles(in []RoleConfig) []RoleConfig {
	if in == nil {
		return nil
	}
	out := make([]RoleConfig, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].PermissionPolicy != nil {
			out[i].PermissionPolicy = append([]PermissionRule(nil), in[i].PermissionPolicy...)
		}
		if in[i].MCP.Tools != nil {
			out[i].MCP.Tools = append([]string(nil), in[i].MCP.Tools...)
		}
	}
	return out
}

func cloneRoleBindings(in RoleBindings) RoleBindings {
	out := in
	out.Run.StageRoles = cloneStringMap(in.Run.StageRoles)
	out.ReviewOrchestrator.Reviewers = cloneStringMap(in.ReviewOrchestrator.Reviewers)
	return out
}

func cloneStringSlicePtr(in *[]string) *[]string {
	if in == nil {
		return nil
	}
	out := append([]string(nil), (*in)...)
	return &out
}

func cloneGitHubConfig(in GitHubConfig) GitHubConfig {
	out := in
	out.LabelMapping = cloneStringMap(in.LabelMapping)
	out.AuthorizedUsernames = cloneStringSlice(in.AuthorizedUsernames)
	out.PR.Reviewers = cloneStringSlice(in.PR.Reviewers)
	out.PR.Labels = cloneStringSlice(in.PR.Labels)
	return out
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string(nil), in...)
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
