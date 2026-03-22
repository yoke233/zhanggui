package config

import "fmt"

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
	out.GitHub = cloneGitHubConfig(in.GitHub)
	out.Audit = cloneAuditConfig(in.Audit)
	out.Runtime = cloneRuntimeConfig(in.Runtime)
	return out
}

func ApplyConfigLayer(cfg *Config, layer *ConfigLayer) {
	if cfg == nil || layer == nil {
		return
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
		if watchdog := scheduler.Watchdog; watchdog != nil {
			if watchdog.Enabled != nil {
				cfg.Scheduler.Watchdog.Enabled = *watchdog.Enabled
			}
			if watchdog.Interval != nil {
				cfg.Scheduler.Watchdog.Interval = *watchdog.Interval
			}
			if watchdog.StuckRunTTL != nil {
				cfg.Scheduler.Watchdog.StuckRunTTL = *watchdog.StuckRunTTL
			}
			if watchdog.StuckMergeTTL != nil {
				cfg.Scheduler.Watchdog.StuckMergeTTL = *watchdog.StuckMergeTTL
			}
			if watchdog.QueueStaleTTL != nil {
				cfg.Scheduler.Watchdog.QueueStaleTTL = *watchdog.QueueStaleTTL
			}
		}
	}

	if server := layer.Server; server != nil {
		if server.Host != nil {
			cfg.Server.Host = *server.Host
		}
		if server.Port != nil {
			cfg.Server.Port = *server.Port
		}
		if server.AuthRequired != nil {
			cfg.Server.AuthRequired = server.AuthRequired
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
			cfg.GitHub.LabelMapping = CloneStringMap(*github.LabelMapping)
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

	if audit := layer.Audit; audit != nil {
		if audit.Enabled != nil {
			cfg.Audit.Enabled = *audit.Enabled
		}
		if audit.FallbackDir != nil {
			cfg.Audit.FallbackDir = *audit.FallbackDir
		}
		if audit.RetentionDays != nil {
			cfg.Audit.RetentionDays = *audit.RetentionDays
		}
		if audit.RedactionLevel != nil {
			cfg.Audit.RedactionLevel = *audit.RedactionLevel
		}
		if otlp := audit.OTLP; otlp != nil {
			if otlp.Enabled != nil {
				cfg.Audit.OTLP.Enabled = *otlp.Enabled
			}
			if otlp.Endpoint != nil {
				cfg.Audit.OTLP.Endpoint = *otlp.Endpoint
			}
			if otlp.Headers != nil {
				cfg.Audit.OTLP.Headers = CloneStringMap(*otlp.Headers)
			}
		}
	}

	if llmFilter := layer.LLMFilter; llmFilter != nil {
		if llmFilter.Enabled != nil {
			cfg.LLMFilter.Enabled = *llmFilter.Enabled
		}
		if llmFilter.Provider != nil {
			cfg.LLMFilter.Provider = *llmFilter.Provider
		}
		if llmFilter.Model != nil {
			cfg.LLMFilter.Model = *llmFilter.Model
		}
	}

	if runtime := layer.Runtime; runtime != nil {
		if runtime.MockExecutor != nil {
			cfg.Runtime.MockExecutor = *runtime.MockExecutor
		}
		if collector := runtime.Collector; collector != nil {
			if collector.MaxRetries != nil {
				cfg.Runtime.Collector.MaxRetries = *collector.MaxRetries
			}
		}
		if llm := runtime.LLM; llm != nil {
			if llm.DefaultConfigID != nil {
				cfg.Runtime.LLM.DefaultConfigID = *llm.DefaultConfigID
			}
			if llm.Configs != nil {
				cfg.Runtime.LLM.Configs = cloneRuntimeLLMEntries(*llm.Configs)
			}
		}
		if sandbox := runtime.Sandbox; sandbox != nil {
			if sandbox.Enabled != nil {
				cfg.Runtime.Sandbox.Enabled = *sandbox.Enabled
			}
			if sandbox.Provider != nil {
				cfg.Runtime.Sandbox.Provider = *sandbox.Provider
			}
			if gc := sandbox.GC; gc != nil {
				if gc.ArchiveCleanup != nil {
					cfg.Runtime.Sandbox.GC.ArchiveCleanup = *gc.ArchiveCleanup
				}
				if gc.StartupCleanup != nil {
					cfg.Runtime.Sandbox.GC.StartupCleanup = *gc.StartupCleanup
				}
				if gc.Interval != nil {
					cfg.Runtime.Sandbox.GC.Interval = *gc.Interval
				}
				if gc.RepoMaxAge != nil {
					cfg.Runtime.Sandbox.GC.RepoMaxAge = *gc.RepoMaxAge
				}
			}
			if litebox := sandbox.LiteBox; litebox != nil {
				if litebox.BridgeCommand != nil {
					cfg.Runtime.Sandbox.LiteBox.BridgeCommand = *litebox.BridgeCommand
				}
				if litebox.BridgeArgs != nil {
					cfg.Runtime.Sandbox.LiteBox.BridgeArgs = cloneStringSlice(*litebox.BridgeArgs)
				}
				if litebox.RunnerPath != nil {
					cfg.Runtime.Sandbox.LiteBox.RunnerPath = *litebox.RunnerPath
				}
				if litebox.RunnerArgs != nil {
					cfg.Runtime.Sandbox.LiteBox.RunnerArgs = cloneStringSlice(*litebox.RunnerArgs)
				}
			}
			if docker := sandbox.Docker; docker != nil {
				if docker.Command != nil {
					cfg.Runtime.Sandbox.Docker.Command = *docker.Command
				}
				if docker.Image != nil {
					cfg.Runtime.Sandbox.Docker.Image = *docker.Image
				}
				if docker.RunArgs != nil {
					cfg.Runtime.Sandbox.Docker.RunArgs = cloneStringSlice(*docker.RunArgs)
				}
				if docker.CPUs != nil {
					cfg.Runtime.Sandbox.Docker.CPUs = *docker.CPUs
				}
				if docker.Memory != nil {
					cfg.Runtime.Sandbox.Docker.Memory = *docker.Memory
				}
				if docker.MemorySwap != nil {
					cfg.Runtime.Sandbox.Docker.MemorySwap = *docker.MemorySwap
				}
				if docker.Network != nil {
					cfg.Runtime.Sandbox.Docker.Network = *docker.Network
				}
				if docker.PidsLimit != nil {
					cfg.Runtime.Sandbox.Docker.PidsLimit = *docker.PidsLimit
				}
				if docker.ReadOnlyRootFS != nil {
					cfg.Runtime.Sandbox.Docker.ReadOnlyRootFS = *docker.ReadOnlyRootFS
				}
				if docker.Tmpfs != nil {
					cfg.Runtime.Sandbox.Docker.Tmpfs = cloneStringSlice(*docker.Tmpfs)
				}
			}
		}
		if agents := runtime.Agents; agents != nil {
			if agents.Drivers != nil {
				cfg.Runtime.Agents.Drivers = cloneRuntimeDrivers(*agents.Drivers)
			}
			if agents.Profiles != nil {
				cfg.Runtime.Agents.Profiles = cloneRuntimeProfiles(*agents.Profiles)
			}
		}
		if mcp := runtime.MCP; mcp != nil {
			if mcp.Servers != nil {
				cfg.Runtime.MCP.Servers = cloneRuntimeMCPServers(*mcp.Servers)
			}
			if mcp.ProfileBindings != nil {
				cfg.Runtime.MCP.ProfileBindings = cloneRuntimeMCPBindings(*mcp.ProfileBindings)
			}
		}
		if sm := runtime.SessionManager; sm != nil {
			if sm.Mode != nil {
				cfg.Runtime.SessionManager.Mode = *sm.Mode
			}
			if sm.ServerID != nil {
				cfg.Runtime.SessionManager.ServerID = *sm.ServerID
			}
			if n := sm.NATS; n != nil {
				if n.URL != nil {
					cfg.Runtime.SessionManager.NATS.URL = *n.URL
				}
				if n.Embedded != nil {
					cfg.Runtime.SessionManager.NATS.Embedded = *n.Embedded
				}
				if n.EmbeddedDataDir != nil {
					cfg.Runtime.SessionManager.NATS.EmbeddedDataDir = *n.EmbeddedDataDir
				}
				if n.StreamPrefix != nil {
					cfg.Runtime.SessionManager.NATS.StreamPrefix = *n.StreamPrefix
				}
			}
		}
		if probe := runtime.RunProbe; probe != nil {
			if probe.Enabled != nil {
				cfg.Runtime.RunProbe.Enabled = *probe.Enabled
			}
			if probe.Interval != nil {
				cfg.Runtime.RunProbe.Interval = *probe.Interval
			}
			if probe.After != nil {
				cfg.Runtime.RunProbe.After = *probe.After
			}
			if probe.IdleAfter != nil {
				cfg.Runtime.RunProbe.IdleAfter = *probe.IdleAfter
			}
			if probe.Timeout != nil {
				cfg.Runtime.RunProbe.Timeout = *probe.Timeout
			}
			if probe.MaxAttempts != nil {
				cfg.Runtime.RunProbe.MaxAttempts = *probe.MaxAttempts
			}
		}
		if cron := runtime.Cron; cron != nil {
			if cron.Enabled != nil {
				cfg.Runtime.Cron.Enabled = *cron.Enabled
			}
			if cron.Interval != nil {
				cfg.Runtime.Cron.Interval = *cron.Interval
			}
		}
		if inspection := runtime.Inspection; inspection != nil {
			if inspection.Enabled != nil {
				cfg.Runtime.Inspection.Enabled = *inspection.Enabled
			}
			if inspection.Interval != nil {
				cfg.Runtime.Inspection.Interval = *inspection.Interval
			}
			if inspection.LookbackH != nil {
				cfg.Runtime.Inspection.LookbackH = *inspection.LookbackH
			}
		}
		if prompts := runtime.Prompts; prompts != nil {
			if prompts.ThreadSharedBootTemplate != nil {
				cfg.Runtime.Prompts.ThreadSharedBootTemplate = *prompts.ThreadSharedBootTemplate
			}
			if prompts.ReworkFollowup != nil {
				cfg.Runtime.Prompts.ReworkFollowup = *prompts.ReworkFollowup
			}
			if prompts.ContinueFollowup != nil {
				cfg.Runtime.Prompts.ContinueFollowup = *prompts.ContinueFollowup
			}
			if prompts.PRImplementObjective != nil {
				cfg.Runtime.Prompts.PRImplementObjective = *prompts.PRImplementObjective
			}
			if prompts.PRGateObjective != nil {
				cfg.Runtime.Prompts.PRGateObjective = *prompts.PRGateObjective
			}
			if prompts.PRMergeReworkFeedback != nil {
				cfg.Runtime.Prompts.PRMergeReworkFeedback = *prompts.PRMergeReworkFeedback
			}
			mergeRuntimePromptProviders(&cfg.Runtime.Prompts.PRProviders, prompts.PRProviders)
		}
	}
}

func mergeRuntimePromptProviders(dst *RuntimePRPromptProvidersConfig, src *RuntimePRPromptProvidersLayer) {
	if dst == nil || src == nil {
		return
	}
	mergeRuntimePromptProvider(&dst.GitHub, src.GitHub)
	mergeRuntimePromptProvider(&dst.CodeUp, src.CodeUp)
	mergeRuntimePromptProvider(&dst.GitLab, src.GitLab)
}

func mergeRuntimePromptProvider(dst *RuntimePRProviderPromptConfig, src *RuntimePRProviderPromptLayer) {
	if dst == nil || src == nil {
		return
	}
	if src.ImplementObjective != nil {
		dst.ImplementObjective = *src.ImplementObjective
	}
	if src.GateObjective != nil {
		dst.GateObjective = *src.GateObjective
	}
	if src.MergeReworkFeedback != nil {
		dst.MergeReworkFeedback = *src.MergeReworkFeedback
	}
	mergeRuntimePromptMergeStates(&dst.MergeStates, src.MergeStates)
}

func mergeRuntimePromptMergeStates(dst *RuntimePRMergeStatePromptConfig, src *RuntimePRMergeStatePromptLayer) {
	if dst == nil || src == nil {
		return
	}
	if src.Default != nil {
		dst.Default = *src.Default
	}
	if src.Dirty != nil {
		dst.Dirty = *src.Dirty
	}
	if src.Blocked != nil {
		dst.Blocked = *src.Blocked
	}
	if src.Behind != nil {
		dst.Behind = *src.Behind
	}
	if src.Unstable != nil {
		dst.Unstable = *src.Unstable
	}
	if src.Draft != nil {
		dst.Draft = *src.Draft
	}
}

func cloneGitHubConfig(in GitHubConfig) GitHubConfig {
	out := in
	out.LabelMapping = CloneStringMap(in.LabelMapping)
	out.AuthorizedUsernames = cloneStringSlice(in.AuthorizedUsernames)
	out.PR.Reviewers = cloneStringSlice(in.PR.Reviewers)
	out.PR.Labels = cloneStringSlice(in.PR.Labels)
	return out
}

func cloneAuditConfig(in AuditConfig) AuditConfig {
	out := in
	out.OTLP.Headers = CloneStringMap(in.OTLP.Headers)
	return out
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string(nil), in...)
}

// CloneStringMap returns a shallow copy of a string-to-string map.
// It is exported so that sibling packages (e.g. configruntime) can reuse
// the same implementation instead of maintaining a duplicate.
func CloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneRuntimeConfig(in RuntimeConfig) RuntimeConfig {
	out := in
	out.LLM.Configs = cloneRuntimeLLMEntries(in.LLM.Configs)
	out.Sandbox.LiteBox.BridgeArgs = cloneStringSlice(in.Sandbox.LiteBox.BridgeArgs)
	out.Sandbox.LiteBox.RunnerArgs = cloneStringSlice(in.Sandbox.LiteBox.RunnerArgs)
	out.Sandbox.Docker.RunArgs = cloneStringSlice(in.Sandbox.Docker.RunArgs)
	out.Sandbox.Docker.Tmpfs = cloneStringSlice(in.Sandbox.Docker.Tmpfs)
	out.Agents.Drivers = cloneRuntimeDrivers(in.Agents.Drivers)
	out.Agents.Profiles = cloneRuntimeProfiles(in.Agents.Profiles)
	out.MCP.Servers = cloneRuntimeMCPServers(in.MCP.Servers)
	out.MCP.ProfileBindings = cloneRuntimeMCPBindings(in.MCP.ProfileBindings)
	return out
}

func cloneRuntimeLLMEntries(in []RuntimeLLMEntryConfig) []RuntimeLLMEntryConfig {
	if in == nil {
		return nil
	}
	out := make([]RuntimeLLMEntryConfig, len(in))
	copy(out, in)
	return out
}

func cloneRuntimeDrivers(in []RuntimeDriverConfig) []RuntimeDriverConfig {
	if in == nil {
		return nil
	}
	out := make([]RuntimeDriverConfig, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].LaunchArgs != nil {
			out[i].LaunchArgs = append([]string(nil), in[i].LaunchArgs...)
		}
		if in[i].Env != nil {
			out[i].Env = CloneStringMap(in[i].Env)
		}
	}
	return out
}

func cloneRuntimeProfiles(in []RuntimeProfileConfig) []RuntimeProfileConfig {
	if in == nil {
		return nil
	}
	out := make([]RuntimeProfileConfig, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].Capabilities != nil {
			out[i].Capabilities = cloneStringSlice(in[i].Capabilities)
		}
		if in[i].ActionsAllowed != nil {
			out[i].ActionsAllowed = cloneStringSlice(in[i].ActionsAllowed)
		}
		if in[i].Skills != nil {
			out[i].Skills = cloneStringSlice(in[i].Skills)
		}
		if in[i].MCP.Tools != nil {
			out[i].MCP.Tools = cloneStringSlice(in[i].MCP.Tools)
		}
	}
	return out
}

func cloneRuntimeMCPServers(in []RuntimeMCPServerConfig) []RuntimeMCPServerConfig {
	if in == nil {
		return nil
	}
	out := make([]RuntimeMCPServerConfig, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Args = cloneStringSlice(in[i].Args)
		out[i].Env = CloneStringMap(in[i].Env)
	}
	return out
}

func cloneRuntimeMCPBindings(in []RuntimeMCPProfileBindingConfig) []RuntimeMCPProfileBindingConfig {
	if in == nil {
		return nil
	}
	out := make([]RuntimeMCPProfileBindingConfig, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Tools = cloneStringSlice(in[i].Tools)
	}
	return out
}
