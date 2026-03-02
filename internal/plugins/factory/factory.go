package factory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/ai-workflow/internal/acpclient"
	"github.com/user/ai-workflow/internal/config"
	"github.com/user/ai-workflow/internal/core"
	githubsvc "github.com/user/ai-workflow/internal/github"
	agentclaude "github.com/user/ai-workflow/internal/plugins/agent-claude"
	agentcodex "github.com/user/ai-workflow/internal/plugins/agent-codex"
	notifierdesktop "github.com/user/ai-workflow/internal/plugins/notifier-desktop"
	reviewaipanel "github.com/user/ai-workflow/internal/plugins/review-ai-panel"
	reviewgithubpr "github.com/user/ai-workflow/internal/plugins/review-github-pr"
	reviewlocal "github.com/user/ai-workflow/internal/plugins/review-local"
	runtimeprocess "github.com/user/ai-workflow/internal/plugins/runtime-process"
	scmgithub "github.com/user/ai-workflow/internal/plugins/scm-github"
	scmlocalgit "github.com/user/ai-workflow/internal/plugins/scm-local-git"
	specnoop "github.com/user/ai-workflow/internal/plugins/spec-noop"
	storesqlite "github.com/user/ai-workflow/internal/plugins/store-sqlite"
	trackergithub "github.com/user/ai-workflow/internal/plugins/tracker-github"
	trackerlocal "github.com/user/ai-workflow/internal/plugins/tracker-local"
	"github.com/user/ai-workflow/internal/secretary"
)

// BootstrapSet contains initialized plugins required by engine bootstrap.
type BootstrapSet struct {
	Agents       map[string]core.AgentPlugin
	RoleResolver *acpclient.RoleResolver
	Runtime      core.RuntimePlugin
	Store        core.Store
	ReviewGate   core.ReviewGate
	Tracker      core.Tracker
	SCM          core.SCM
	Notifier     core.Notifier
	Spec         core.SpecPlugin
}

const (
	defaultReviewGatePlugin = "review-ai-panel"
	localReviewGatePlugin   = "review-local"
	defaultTrackerPlugin    = "tracker-local"
	defaultSCMPlugin        = "local-git"
	defaultNotifierPlugin   = "desktop"
	githubTrackerPluginName = "tracker-github"
	githubSCMPluginName     = "scm-github"
)

type pluginNameOverrides struct {
	Tracker string
	SCM     string
}

type trackerAndSCMPluginNames struct {
	Tracker string
	SCM     string
}

func selectTrackerAndSCMPluginNames(githubEnabled bool, overrides pluginNameOverrides) trackerAndSCMPluginNames {
	selected := trackerAndSCMPluginNames{
		Tracker: defaultTrackerPlugin,
		SCM:     defaultSCMPlugin,
	}
	if githubEnabled {
		selected.Tracker = githubTrackerPluginName
		selected.SCM = githubSCMPluginName
	}

	if overrideTracker := strings.TrimSpace(overrides.Tracker); overrideTracker != "" {
		selected.Tracker = overrideTracker
	}
	if overrideSCM := strings.TrimSpace(overrides.SCM); overrideSCM != "" {
		selected.SCM = overrideSCM
	}
	return selected
}

type storeProvider interface {
	core.Plugin
	Store() core.Store
}

type storeAdapter struct {
	name  string
	store core.Store
}

func (s *storeAdapter) Name() string               { return s.name }
func (s *storeAdapter) Init(context.Context) error { return nil }
func (s *storeAdapter) Close() error               { return s.store.Close() }
func (s *storeAdapter) Store() core.Store          { return s.store }

func BuildFromConfig(cfg config.Config) (*BootstrapSet, error) {
	registry, err := newDefaultRegistry()
	if err != nil {
		return nil, err
	}
	return buildWithRegistry(registry, cfg)
}

func buildWithRegistry(registry *core.Registry, cfg config.Config) (*BootstrapSet, error) {
	effective := withDefaults(cfg)
	if err := config.Validate(&effective); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	roleResolver, err := buildRoleResolver(effective)
	if err != nil {
		return nil, fmt.Errorf("build role resolver: %w", err)
	}

	storeName := strings.TrimSpace(effective.Store.Driver)
	storeModule, ok := registry.Get(core.SlotStore, storeName)
	if !ok {
		return nil, fmt.Errorf("unknown plugin: slot=%s name=%s", core.SlotStore, storeName)
	}

	storePluginRaw, err := storeModule.Factory(map[string]any{
		"path": effective.Store.Path,
	})
	if err != nil {
		return nil, fmt.Errorf("build store plugin %q: %w", storeName, err)
	}
	storePlugin, ok := storePluginRaw.(storeProvider)
	if !ok {
		return nil, fmt.Errorf("plugin is not a store provider: slot=%s name=%s", core.SlotStore, storeName)
	}

	runtimeName := strings.TrimSpace(effective.Runtime.Driver)
	runtimeModule, ok := registry.Get(core.SlotRuntime, runtimeName)
	if !ok {
		return nil, fmt.Errorf("unknown plugin: slot=%s name=%s", core.SlotRuntime, runtimeName)
	}
	runtimeRaw, err := runtimeModule.Factory(nil)
	if err != nil {
		return nil, fmt.Errorf("build runtime plugin %q: %w", runtimeName, err)
	}
	runtimePlugin, ok := runtimeRaw.(core.RuntimePlugin)
	if !ok {
		return nil, fmt.Errorf("plugin is not a runtime plugin: slot=%s name=%s", core.SlotRuntime, runtimeName)
	}

	agentConfigs := map[string]*config.AgentConfig{
		"claude": effective.Agents.Claude,
		"codex":  effective.Agents.Codex,
	}
	agents := make(map[string]core.AgentPlugin, len(agentConfigs))
	for agentName, agentCfg := range agentConfigs {
		if agentCfg == nil {
			continue
		}
		moduleName := agentName
		if agentCfg.Plugin != nil && strings.TrimSpace(*agentCfg.Plugin) != "" {
			moduleName = strings.TrimSpace(*agentCfg.Plugin)
		}

		module, ok := registry.Get(core.SlotAgent, moduleName)
		if !ok {
			return nil, fmt.Errorf("unknown plugin: slot=%s name=%s", core.SlotAgent, moduleName)
		}
		raw, err := module.Factory(agentConfigToMap(agentCfg))
		if err != nil {
			return nil, fmt.Errorf("build agent plugin %q: %w", moduleName, err)
		}
		agentPlugin, ok := raw.(core.AgentPlugin)
		if !ok {
			return nil, fmt.Errorf("plugin is not an agent plugin: slot=%s name=%s", core.SlotAgent, moduleName)
		}
		agents[agentName] = agentPlugin
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("no agent plugins configured")
	}
	for _, role := range effective.Roles {
		roleName := strings.TrimSpace(role.Name)
		agentName := strings.TrimSpace(role.Agent)
		if agentName == "" {
			continue
		}
		if _, ok := agents[agentName]; !ok {
			return nil, fmt.Errorf("role %q resolves to agent %q but no executable agent plugin is configured", roleName, agentName)
		}
	}

	reviewGateName := strings.TrimSpace(effective.Secretary.ReviewGatePlugin)
	if reviewGateName == "" {
		reviewGateName = defaultReviewGatePlugin
	}
	reviewGateModule, ok := registry.Get(core.SlotReviewGate, reviewGateName)
	if !ok {
		return nil, fmt.Errorf("unknown plugin: slot=%s name=%s", core.SlotReviewGate, reviewGateName)
	}
	reviewGateRaw, err := reviewGateModule.Factory(map[string]any{
		"store": storePlugin.Store(),
		"review_orchestrator_bindings": secretary.ReviewRoleBindingInput{
			Reviewers:  cloneStringMapForFactory(effective.RoleBinds.ReviewOrchestrator.Reviewers),
			Aggregator: effective.RoleBinds.ReviewOrchestrator.Aggregator,
		},
		"role_resolver": roleResolver,
		"max_rounds":    effective.Secretary.ReviewOrchestrator.MaxRounds,
		"github":        effective.GitHub,
	})
	if err != nil {
		return nil, fmt.Errorf("build review gate plugin %q: %w", reviewGateName, err)
	}
	reviewGatePlugin, ok := reviewGateRaw.(core.ReviewGate)
	if !ok {
		return nil, fmt.Errorf("plugin is not a review gate plugin: slot=%s name=%s", core.SlotReviewGate, reviewGateName)
	}

	selectedPlugins := selectTrackerAndSCMPluginNames(effective.GitHub.Enabled, pluginNameOverrides{})

	trackerName := selectedPlugins.Tracker
	trackerModule, ok := registry.Get(core.SlotTracker, trackerName)
	if !ok && trackerName != defaultTrackerPlugin {
		// GitHub tracker is expected to be added in later waves. Fallback keeps current behavior.
		trackerName = defaultTrackerPlugin
		trackerModule, ok = registry.Get(core.SlotTracker, trackerName)
	}
	if !ok {
		return nil, fmt.Errorf("unknown plugin: slot=%s name=%s", core.SlotTracker, trackerName)
	}
	trackerRaw, err := trackerModule.Factory(map[string]any{
		"github": effective.GitHub,
	})
	if err != nil {
		return nil, fmt.Errorf("build tracker plugin %q: %w", trackerName, err)
	}
	trackerPlugin, ok := trackerRaw.(core.Tracker)
	if !ok {
		return nil, fmt.Errorf("plugin is not a tracker plugin: slot=%s name=%s", core.SlotTracker, trackerName)
	}

	scmName := selectedPlugins.SCM
	scmModule, ok := registry.Get(core.SlotSCM, scmName)
	if !ok {
		return nil, fmt.Errorf("unknown plugin: slot=%s name=%s", core.SlotSCM, scmName)
	}

	scmFactoryCfg := map[string]any{}
	if scmName == githubSCMPluginName {
		client, clientErr := githubsvc.NewClient(effective.GitHub)
		if clientErr != nil {
			return nil, fmt.Errorf("build scm plugin %q: %w", scmName, clientErr)
		}
		service, serviceErr := githubsvc.NewGitHubService(client, effective.GitHub.Owner, effective.GitHub.Repo)
		if serviceErr != nil {
			return nil, fmt.Errorf("build scm plugin %q: %w", scmName, serviceErr)
		}
		scmFactoryCfg["github_service"] = service
		scmFactoryCfg["draft"] = effective.GitHub.PR.Draft
		scmFactoryCfg["reviewers"] = append([]string(nil), effective.GitHub.PR.Reviewers...)
	}
	if len(scmFactoryCfg) == 0 {
		scmFactoryCfg = nil
	}

	scmRaw, err := scmModule.Factory(scmFactoryCfg)
	if err != nil {
		return nil, fmt.Errorf("build scm plugin %q: %w", scmName, err)
	}
	scmPlugin, ok := scmRaw.(core.SCM)
	if !ok {
		return nil, fmt.Errorf("plugin is not a scm plugin: slot=%s name=%s", core.SlotSCM, scmName)
	}

	notifierModule, ok := registry.Get(core.SlotNotifier, defaultNotifierPlugin)
	if !ok {
		return nil, fmt.Errorf("unknown plugin: slot=%s name=%s", core.SlotNotifier, defaultNotifierPlugin)
	}
	notifierRaw, err := notifierModule.Factory(nil)
	if err != nil {
		return nil, fmt.Errorf("build notifier plugin %q: %w", defaultNotifierPlugin, err)
	}
	notifierPlugin, ok := notifierRaw.(core.Notifier)
	if !ok {
		return nil, fmt.Errorf("plugin is not a notifier plugin: slot=%s name=%s", core.SlotNotifier, defaultNotifierPlugin)
	}

	specPlugin, err := buildSpecPluginWithPolicy(registry, effective.Spec)
	if err != nil {
		return nil, err
	}

	return &BootstrapSet{
		Agents:       agents,
		RoleResolver: roleResolver,
		Runtime:      runtimePlugin,
		Store:        storePlugin.Store(),
		ReviewGate:   reviewGatePlugin,
		Tracker:      trackerPlugin,
		SCM:          scmPlugin,
		Notifier:     notifierPlugin,
		Spec:         specPlugin,
	}, nil
}

func newDefaultRegistry() (*core.Registry, error) {
	registry := core.NewRegistry()
	modules := []core.PluginModule{
		{
			Name: "claude",
			Slot: core.SlotAgent,
			Factory: func(cfg map[string]any) (core.Plugin, error) {
				binary := stringFromMap(cfg, "binary", "claude")
				return agentclaude.New(binary), nil
			},
		},
		{
			Name: "codex",
			Slot: core.SlotAgent,
			Factory: func(cfg map[string]any) (core.Plugin, error) {
				binary := stringFromMap(cfg, "binary", "codex")
				model := stringFromMap(cfg, "model", "gpt-5.3-codex")
				reasoning := stringFromMap(cfg, "reasoning", "high")
				return agentcodex.New(binary, model, reasoning), nil
			},
		},
		{
			Name: "process",
			Slot: core.SlotRuntime,
			Factory: func(map[string]any) (core.Plugin, error) {
				return runtimeprocess.New(), nil
			},
		},
		{
			Name: "sqlite",
			Slot: core.SlotStore,
			Factory: func(cfg map[string]any) (core.Plugin, error) {
				storePath := expandPath(stringFromMap(cfg, "path", "~/.ai-workflow/data.db"))
				if storePath != ":memory:" {
					if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
						return nil, fmt.Errorf("ensure sqlite dir: %w", err)
					}
				}
				store, err := storesqlite.New(storePath)
				if err != nil {
					return nil, err
				}
				return &storeAdapter{name: "sqlite", store: store}, nil
			},
		},
		{
			Name: defaultReviewGatePlugin,
			Slot: core.SlotReviewGate,
			Factory: func(cfg map[string]any) (core.Plugin, error) {
				if cfg == nil {
					return nil, fmt.Errorf("%s requires store dependency", defaultReviewGatePlugin)
				}
				rawStore, ok := cfg["store"]
				if !ok {
					return nil, fmt.Errorf("%s requires store dependency", defaultReviewGatePlugin)
				}
				store, ok := rawStore.(core.Store)
				if !ok || store == nil {
					return nil, fmt.Errorf("%s requires valid store dependency", defaultReviewGatePlugin)
				}

				panel := secretary.NewDefaultReviewOrchestrator(store)
				if rawBindings, ok := cfg["review_orchestrator_bindings"]; ok {
					bindings, ok := rawBindings.(secretary.ReviewRoleBindingInput)
					if !ok {
						return nil, fmt.Errorf("%s requires valid review_orchestrator_bindings", defaultReviewGatePlugin)
					}
					resolver, _ := cfg["role_resolver"].(*acpclient.RoleResolver)
					resolvedPanel, err := secretary.NewDefaultReviewOrchestratorFromBindings(store, bindings, resolver)
					if err != nil {
						return nil, fmt.Errorf("build review orchestrator from role bindings: %w", err)
					}
					panel = resolvedPanel
				}
				if maxRounds, ok := cfg["max_rounds"].(int); ok && maxRounds > 0 {
					panel.MaxRounds = maxRounds
				}
				return reviewaipanel.New(store, panel), nil
			},
		},
		{
			Name: localReviewGatePlugin,
			Slot: core.SlotReviewGate,
			Factory: func(cfg map[string]any) (core.Plugin, error) {
				if cfg == nil {
					return nil, fmt.Errorf("%s requires store dependency", localReviewGatePlugin)
				}
				rawStore, ok := cfg["store"]
				if !ok {
					return nil, fmt.Errorf("%s requires store dependency", localReviewGatePlugin)
				}
				store, ok := rawStore.(core.Store)
				if !ok || store == nil {
					return nil, fmt.Errorf("%s requires valid store dependency", localReviewGatePlugin)
				}
				return reviewlocal.New(store), nil
			},
		},
		reviewgithubpr.Module(),
		{
			Name: defaultTrackerPlugin,
			Slot: core.SlotTracker,
			Factory: func(map[string]any) (core.Plugin, error) {
				return trackerlocal.New(), nil
			},
		},
		trackergithub.Module(),
		{
			Name: defaultSCMPlugin,
			Slot: core.SlotSCM,
			Factory: func(cfg map[string]any) (core.Plugin, error) {
				repoDir := stringFromMap(cfg, "repo_dir", ".")
				return scmlocalgit.New(repoDir), nil
			},
		},
		scmgithub.Module(),
		{
			Name: defaultNotifierPlugin,
			Slot: core.SlotNotifier,
			Factory: func(map[string]any) (core.Plugin, error) {
				return notifierdesktop.New(), nil
			},
		},
		specnoop.Module(),
	}

	for _, module := range modules {
		if err := registry.Register(module); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func withDefaults(cfg config.Config) config.Config {
	def := config.Defaults()

	if cfg.Agents.Claude == nil {
		cfg.Agents.Claude = def.Agents.Claude
	}
	if cfg.Agents.Codex == nil {
		cfg.Agents.Codex = def.Agents.Codex
	}
	if len(cfg.Roles) == 0 {
		cfg.Roles = append([]config.RoleConfig(nil), def.Roles...)
	}
	if len(cfg.Agents.Profiles) == 0 && len(def.Agents.Profiles) > 0 {
		cfg.Agents.Profiles = append([]config.AgentProfileConfig(nil), def.Agents.Profiles...)
	}
	if isRoleBindingsEmpty(cfg.RoleBinds) {
		cfg.RoleBinds = def.RoleBinds
	}
	if cfg.Runtime.Driver == "" {
		cfg.Runtime.Driver = def.Runtime.Driver
	}
	if cfg.Store.Driver == "" {
		cfg.Store.Driver = def.Store.Driver
	}
	if cfg.Store.Path == "" {
		cfg.Store.Path = def.Store.Path
	}
	if strings.TrimSpace(cfg.Spec.Provider) == "" {
		cfg.Spec.Provider = def.Spec.Provider
	}
	if strings.TrimSpace(cfg.Spec.OnFailure) == "" {
		cfg.Spec.OnFailure = def.Spec.OnFailure
	}
	if strings.TrimSpace(cfg.Spec.OpenSpec.Binary) == "" {
		cfg.Spec.OpenSpec.Binary = def.Spec.OpenSpec.Binary
	}
	return cfg
}

func isRoleBindingsEmpty(binds config.RoleBindings) bool {
	return strings.TrimSpace(binds.Secretary.Role) == "" &&
		strings.TrimSpace(binds.PlanParser.Role) == "" &&
		strings.TrimSpace(binds.ReviewOrchestrator.Aggregator) == "" &&
		len(binds.Pipeline.StageRoles) == 0 &&
		len(binds.ReviewOrchestrator.Reviewers) == 0
}

func buildRoleResolver(cfg config.Config) (*acpclient.RoleResolver, error) {
	agentProfiles := cfg.EffectiveAgentProfiles()
	agents := make([]acpclient.AgentProfile, 0, len(agentProfiles))
	for _, agent := range agentProfiles {
		agentID := strings.TrimSpace(agent.Name)
		agents = append(agents, acpclient.AgentProfile{
			ID:            agentID,
			LaunchCommand: agent.LaunchCommand,
			LaunchArgs:    append([]string(nil), agent.LaunchArgs...),
			Env:           cloneStringMapForFactory(agent.Env),
			CapabilitiesMax: acpclient.ClientCapabilities{
				FSRead:   agent.CapabilitiesMax.FSRead,
				FSWrite:  agent.CapabilitiesMax.FSWrite,
				Terminal: agent.CapabilitiesMax.Terminal,
			},
		})
	}

	roles := make([]acpclient.RoleProfile, 0, len(cfg.Roles))
	for _, role := range cfg.Roles {
		roleID := strings.TrimSpace(role.Name)
		agentID := strings.TrimSpace(role.Agent)
		roles = append(roles, acpclient.RoleProfile{
			ID:             roleID,
			AgentID:        agentID,
			PromptTemplate: role.PromptTemplate,
			SessionPolicy: acpclient.SessionPolicy{
				Reuse:             role.Session.Reuse,
				PreferLoadSession: role.Session.PreferLoadSession,
				ResetPrompt:       role.Session.ResetPrompt,
				MaxTurns:          role.Session.MaxTurns,
			},
			Capabilities: acpclient.ClientCapabilities{
				FSRead:   role.Capabilities.FSRead,
				FSWrite:  role.Capabilities.FSWrite,
				Terminal: role.Capabilities.Terminal,
			},
			PermissionPolicy: toACPPermissionRules(role.PermissionPolicy),
			MCPTools:         append([]string(nil), role.MCP.Tools...),
		})
	}

	resolver := acpclient.NewRoleResolver(agents, roles)
	for _, role := range roles {
		if _, _, err := resolver.Resolve(role.ID); err != nil {
			return nil, err
		}
	}
	return resolver, nil
}

func cloneStringMapForFactory(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func toACPPermissionRules(in []config.PermissionRule) []acpclient.PermissionRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]acpclient.PermissionRule, len(in))
	for i := range in {
		out[i] = acpclient.PermissionRule{
			Pattern: in[i].Pattern,
			Action:  in[i].Action,
			Scope:   in[i].Scope,
		}
	}
	return out
}

func agentConfigToMap(agent *config.AgentConfig) map[string]any {
	out := map[string]any{}
	if agent == nil {
		return out
	}
	if agent.Binary != nil {
		out["binary"] = *agent.Binary
	}
	if agent.Model != nil {
		out["model"] = *agent.Model
	}
	if agent.Reasoning != nil {
		out["reasoning"] = *agent.Reasoning
	}
	return out
}

func stringFromMap(cfg map[string]any, key, fallback string) string {
	if cfg != nil {
		if raw, ok := cfg[key]; ok {
			if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return fallback
}

func expandPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return trimmed
	}
	if trimmed == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
		return trimmed
	}
	if strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return trimmed
		}
		suffix := strings.TrimPrefix(strings.TrimPrefix(trimmed, "~/"), "~\\")
		return filepath.Join(home, filepath.FromSlash(strings.ReplaceAll(suffix, "\\", "/")))
	}
	return trimmed
}

func buildSpecPluginWithPolicy(registry *core.Registry, cfg config.SpecConfig) (core.SpecPlugin, error) {
	fallbackNoop := func() (core.SpecPlugin, error) {
		return buildAndInitSpecPlugin(registry, "noop", cfg)
	}

	if !cfg.Enabled {
		return fallbackNoop()
	}

	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" || strings.EqualFold(provider, "noop") {
		return fallbackNoop()
	}

	plugin, err := buildAndInitSpecPlugin(registry, provider, cfg)
	if err == nil {
		return plugin, nil
	}

	if normalizeSpecOnFailure(cfg.OnFailure) == "fail" {
		return nil, fmt.Errorf("spec provider %q: %w", provider, err)
	}

	fmt.Fprintf(os.Stderr, "warning: spec provider %q unavailable, fallback to noop: %v\n", provider, err)
	return fallbackNoop()
}

func buildAndInitSpecPlugin(registry *core.Registry, provider string, cfg config.SpecConfig) (core.SpecPlugin, error) {
	module, ok := registry.Get(core.SlotSpec, provider)
	if !ok {
		return nil, fmt.Errorf("unknown plugin: slot=%s name=%s", core.SlotSpec, provider)
	}

	raw, err := module.Factory(map[string]any{
		"provider":        provider,
		"openspec_binary": cfg.OpenSpec.Binary,
	})
	if err != nil {
		return nil, fmt.Errorf("build spec plugin %q: %w", provider, err)
	}

	plugin, ok := raw.(core.SpecPlugin)
	if !ok {
		return nil, fmt.Errorf("plugin is not a spec plugin: slot=%s name=%s", core.SlotSpec, provider)
	}

	if err := plugin.Init(context.Background()); err != nil {
		_ = plugin.Close()
		return nil, fmt.Errorf("init spec plugin %q: %w", provider, err)
	}
	return plugin, nil
}

func normalizeSpecOnFailure(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fail":
		return "fail"
	default:
		return "warn"
	}
}
