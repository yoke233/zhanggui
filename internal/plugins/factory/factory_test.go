package factory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/user/ai-workflow/internal/config"
	"github.com/user/ai-workflow/internal/core"
)

func TestFactoryBuildKnownPlugin(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig returned error: %v", err)
	}
	defer set.Store.Close()

	if set.Store == nil {
		t.Fatal("expected store to be initialized")
	}
	if set.Runtime == nil {
		t.Fatal("expected runtime to be initialized")
	}
	if _, ok := set.Agents["claude"]; !ok {
		t.Fatal("expected claude agent to be initialized")
	}
	if _, ok := set.Agents["codex"]; !ok {
		t.Fatal("expected codex agent to be initialized")
	}
	if set.ReviewGate == nil {
		t.Fatal("expected review gate to be initialized")
	}
	if set.ReviewGate.Name() != "ai-panel" {
		t.Fatalf("expected review gate name ai-panel, got %q", set.ReviewGate.Name())
	}
	if set.Tracker == nil {
		t.Fatal("expected tracker to be initialized")
	}
	if set.Tracker.Name() != "local" {
		t.Fatalf("expected tracker name local, got %q", set.Tracker.Name())
	}
	if set.SCM == nil {
		t.Fatal("expected scm to be initialized")
	}
	if set.SCM.Name() != "local-git" {
		t.Fatalf("expected scm name local-git, got %q", set.SCM.Name())
	}
	if set.Notifier == nil {
		t.Fatal("expected notifier to be initialized")
	}
	if set.Notifier.Name() != "desktop" {
		t.Fatalf("expected notifier name desktop, got %q", set.Notifier.Name())
	}
	if set.Spec == nil {
		t.Fatal("expected spec plugin to be initialized")
	}
	if set.RoleResolver == nil {
		t.Fatal("expected role resolver to be initialized")
	}
}

func TestFactoryBuildsRoleResolver(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig returned error: %v", err)
	}
	defer set.Store.Close()

	if set.RoleResolver == nil {
		t.Fatal("expected bootstrap set role resolver")
	}
	agent, role, err := set.RoleResolver.Resolve("worker")
	if err != nil {
		t.Fatalf("resolve worker failed: %v", err)
	}
	if agent.ID == "" || role.ID != "worker" {
		t.Fatalf("unexpected resolver output agent=%q role=%q", agent.ID, role.ID)
	}
}

func TestFactoryBuildsRoleResolver_TrimmedNamesResolve(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Roles = []config.RoleConfig{
		{
			Name:  " worker ",
			Agent: " codex ",
			Capabilities: config.CapabilitiesConfig{
				FSRead:   true,
				FSWrite:  true,
				Terminal: true,
			},
		},
	}
	cfg.RoleBinds = config.RoleBindings{
		Secretary: config.SingleRoleBinding{
			Role: "worker",
		},
		Pipeline: config.PipelineRoleBindings{
			StageRoles: map[string]string{
				"implement": "worker",
			},
		},
		ReviewOrchestrator: config.ReviewRoleBindings{
			Reviewers: map[string]string{
				"completeness": "worker",
				"dependency":   "worker",
				"feasibility":  "worker",
			},
			Aggregator: "worker",
		},
		PlanParser: config.SingleRoleBinding{
			Role: "worker",
		},
	}

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig returned error: %v", err)
	}
	defer set.Store.Close()

	agent, role, err := set.RoleResolver.Resolve("worker")
	if err != nil {
		t.Fatalf("resolve trimmed worker failed: %v", err)
	}
	if agent.ID != "codex" {
		t.Fatalf("expected resolved agent codex, got %q", agent.ID)
	}
	if role.ID != "worker" {
		t.Fatalf("expected resolved role worker, got %q", role.ID)
	}
}

func TestFactoryBuildUnknownPlugin(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Driver = "unknown-driver"
	cfg.Store.Path = ":memory:"

	_, err := BuildFromConfig(cfg)
	if err == nil {
		t.Fatal("expected BuildFromConfig to fail for unknown plugin")
	}
	if !strings.Contains(err.Error(), "unknown plugin") {
		t.Fatalf("expected unknown plugin error, got %v", err)
	}
}

func TestFactoryBuildUnknownRuntimePlugin(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Runtime.Driver = "unknown-runtime"

	_, err := BuildFromConfig(cfg)
	if err == nil {
		t.Fatal("expected BuildFromConfig to fail for unknown runtime plugin")
	}
	if !strings.Contains(err.Error(), "unknown plugin") {
		t.Fatalf("expected unknown plugin error, got %v", err)
	}
}

func TestFactoryBuildReviewGateCanSwitchToLocal(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Secretary.ReviewGatePlugin = "review-local"

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig returned error: %v", err)
	}
	defer set.Store.Close()

	if set.ReviewGate == nil {
		t.Fatal("expected review gate to be initialized")
	}
	if set.ReviewGate.Name() != "local" {
		t.Fatalf("expected review gate name local, got %q", set.ReviewGate.Name())
	}
}

func TestFactoryBuildUnknownAgentPlugin(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Agents.Codex.Plugin = stringPtr("unknown-agent")

	_, err := BuildFromConfig(cfg)
	if err == nil {
		t.Fatal("expected BuildFromConfig to fail for unknown agent plugin")
	}
	if !strings.Contains(err.Error(), "unknown plugin") {
		t.Fatalf("expected unknown plugin error, got %v", err)
	}
}

func TestFactoryBuildRoleAgentMustBeExecutable(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Agents.Profiles = []config.AgentProfileConfig{
		{
			Name:          "ghost",
			LaunchCommand: "ghost-agent",
			CapabilitiesMax: config.CapabilitiesConfig{
				FSRead:   true,
				FSWrite:  true,
				Terminal: true,
			},
		},
	}
	cfg.Roles = []config.RoleConfig{
		{
			Name:  "worker",
			Agent: "ghost",
			Capabilities: config.CapabilitiesConfig{
				FSRead:   true,
				FSWrite:  true,
				Terminal: true,
			},
		},
	}
	cfg.RoleBinds = config.RoleBindings{
		Secretary: config.SingleRoleBinding{
			Role: "worker",
		},
		Pipeline: config.PipelineRoleBindings{
			StageRoles: map[string]string{
				"implement": "worker",
			},
		},
		ReviewOrchestrator: config.ReviewRoleBindings{
			Reviewers: map[string]string{
				"completeness": "worker",
				"dependency":   "worker",
				"feasibility":  "worker",
			},
			Aggregator: "worker",
		},
		PlanParser: config.SingleRoleBinding{
			Role: "worker",
		},
	}

	_, err := BuildFromConfig(cfg)
	if err == nil {
		t.Fatal("expected BuildFromConfig to fail when role resolves to non-executable agent plugin")
	}
	if !strings.Contains(err.Error(), "no executable agent plugin is configured") {
		t.Fatalf("expected executable agent plugin error, got %v", err)
	}
}

func TestBootstrapSet_ContainsSpecPlugin(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig returned error: %v", err)
	}
	defer set.Store.Close()

	if set.Spec == nil {
		t.Fatal("expected bootstrap set to contain spec plugin")
	}
}

func TestBuildWithRegistry_LoadsSpecWhenEnabled(t *testing.T) {
	registry, err := newDefaultRegistry()
	if err != nil {
		t.Fatalf("newDefaultRegistry() error = %v", err)
	}
	if err := registry.Register(core.PluginModule{
		Name: "fake-spec",
		Slot: core.SlotSpec,
		Factory: func(map[string]any) (core.Plugin, error) {
			return &stubSpecPlugin{name: "fake-spec"}, nil
		},
	}); err != nil {
		t.Fatalf("register fake-spec module: %v", err)
	}

	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Spec.Enabled = true
	cfg.Spec.Provider = "fake-spec"
	cfg.Spec.OnFailure = "fail"

	set, err := buildWithRegistry(registry, cfg)
	if err != nil {
		t.Fatalf("buildWithRegistry() error = %v", err)
	}
	defer set.Store.Close()

	if set.Spec == nil {
		t.Fatal("expected spec plugin")
	}
	if got := set.Spec.Name(); got != "fake-spec" {
		t.Fatalf("spec provider = %q, want fake-spec", got)
	}
	if !set.Spec.IsInitialized() {
		t.Fatal("expected spec provider initialized")
	}
}

func TestBuildWithRegistry_UsesNoopSpecWhenDisabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Spec.Enabled = false
	cfg.Spec.Provider = "missing-provider"
	cfg.Spec.OnFailure = "fail"

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig() error = %v", err)
	}
	defer set.Store.Close()

	if set.Spec == nil {
		t.Fatal("expected fallback noop spec plugin")
	}
	if got := set.Spec.Name(); got != "spec-noop" {
		t.Fatalf("spec provider = %q, want spec-noop", got)
	}
}

func TestBuildWithRegistry_SpecProviderMissing_OnFailureWarn_FallbackNoop(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Spec.Enabled = true
	cfg.Spec.Provider = "missing-provider"
	cfg.Spec.OnFailure = "warn"

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig() error = %v", err)
	}
	defer set.Store.Close()

	if set.Spec == nil {
		t.Fatal("expected fallback noop spec plugin")
	}
	if got := set.Spec.Name(); got != "spec-noop" {
		t.Fatalf("spec provider = %q, want spec-noop", got)
	}
}

func TestBuildWithRegistry_SpecProviderMissing_OnFailureFail_ReturnsError(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Spec.Enabled = true
	cfg.Spec.Provider = "missing-provider"
	cfg.Spec.OnFailure = "fail"

	_, err := BuildFromConfig(cfg)
	if err == nil {
		t.Fatal("expected missing spec provider to fail when on_failure=fail")
	}
	if !strings.Contains(err.Error(), "spec provider") {
		t.Fatalf("expected spec provider error, got %v", err)
	}
}

func TestBuildWithRegistry_SpecInitError_OnFailureWarn_FallbackNoop(t *testing.T) {
	registry, err := newDefaultRegistry()
	if err != nil {
		t.Fatalf("newDefaultRegistry() error = %v", err)
	}
	if err := registry.Register(core.PluginModule{
		Name: "broken-spec",
		Slot: core.SlotSpec,
		Factory: func(map[string]any) (core.Plugin, error) {
			return &stubSpecPlugin{name: "broken-spec", initErr: errors.New("init failed")}, nil
		},
	}); err != nil {
		t.Fatalf("register broken-spec module: %v", err)
	}

	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Spec.Enabled = true
	cfg.Spec.Provider = "broken-spec"
	cfg.Spec.OnFailure = "warn"

	set, err := buildWithRegistry(registry, cfg)
	if err != nil {
		t.Fatalf("buildWithRegistry() error = %v", err)
	}
	defer set.Store.Close()

	if set.Spec == nil {
		t.Fatal("expected fallback noop spec plugin")
	}
	if got := set.Spec.Name(); got != "spec-noop" {
		t.Fatalf("spec provider = %q, want spec-noop", got)
	}
}

func TestBuildWithRegistry_SpecInitError_OnFailureFail_ReturnsError(t *testing.T) {
	registry, err := newDefaultRegistry()
	if err != nil {
		t.Fatalf("newDefaultRegistry() error = %v", err)
	}
	if err := registry.Register(core.PluginModule{
		Name: "broken-spec",
		Slot: core.SlotSpec,
		Factory: func(map[string]any) (core.Plugin, error) {
			return &stubSpecPlugin{name: "broken-spec", initErr: errors.New("init failed")}, nil
		},
	}); err != nil {
		t.Fatalf("register broken-spec module: %v", err)
	}

	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.Spec.Enabled = true
	cfg.Spec.Provider = "broken-spec"
	cfg.Spec.OnFailure = "fail"

	_, err = buildWithRegistry(registry, cfg)
	if err == nil {
		t.Fatal("expected spec init failure when on_failure=fail")
	}
	if !strings.Contains(err.Error(), "init failed") {
		t.Fatalf("expected init failure error, got %v", err)
	}
}

func TestFactory_GitHubEnabled_SelectsTrackerAndSCM(t *testing.T) {
	selected := selectTrackerAndSCMPluginNames(true, pluginNameOverrides{})

	if selected.Tracker != githubTrackerPluginName {
		t.Fatalf("expected tracker plugin %q when github.enabled=true, got %q", githubTrackerPluginName, selected.Tracker)
	}
	if selected.SCM != githubSCMPluginName {
		t.Fatalf("expected scm plugin %q when github.enabled=true, got %q", githubSCMPluginName, selected.SCM)
	}
}

func TestFactory_GitHubExplicitOverride_Wins(t *testing.T) {
	overrideSelected := selectTrackerAndSCMPluginNames(true, pluginNameOverrides{
		Tracker: "tracker-local",
		SCM:     "local-git",
	})

	if overrideSelected.Tracker != "tracker-local" {
		t.Fatalf("expected explicit tracker override to win, got %q", overrideSelected.Tracker)
	}
	if overrideSelected.SCM != "local-git" {
		t.Fatalf("expected explicit scm override to win, got %q", overrideSelected.SCM)
	}
}

func TestFactory_GitHubDisabled_UsesLocalDefaults(t *testing.T) {
	selected := selectTrackerAndSCMPluginNames(false, pluginNameOverrides{})

	if selected.Tracker != defaultTrackerPlugin {
		t.Fatalf("expected local tracker plugin %q when github.enabled=false, got %q", defaultTrackerPlugin, selected.Tracker)
	}
	if selected.SCM != defaultSCMPlugin {
		t.Fatalf("expected local scm plugin %q when github.enabled=false, got %q", defaultSCMPlugin, selected.SCM)
	}
}

func TestFactory_GitHubEnabled_BuildFromConfigSelectsTrackerAndSCMPlugins(t *testing.T) {
	cfg := config.Defaults()
	cfg.Store.Path = ":memory:"
	cfg.GitHub.Enabled = true
	cfg.GitHub.Token = "ghp_test_token"
	cfg.GitHub.AllowPATFallback = true
	cfg.GitHub.Owner = "acme"
	cfg.GitHub.Repo = "ai-workflow"
	cfg.GitHub.WebhookSecret = "secret"

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig() error = %v", err)
	}
	defer set.Store.Close()

	if set.Tracker == nil {
		t.Fatal("expected github tracker plugin")
	}
	if set.Tracker.Name() != githubTrackerPluginName {
		t.Fatalf("expected tracker %q, got %q", githubTrackerPluginName, set.Tracker.Name())
	}
	if set.SCM == nil {
		t.Fatal("expected github scm plugin")
	}
	if set.SCM.Name() != githubSCMPluginName {
		t.Fatalf("expected scm %q, got %q", githubSCMPluginName, set.SCM.Name())
	}
}

func stringPtr(v string) *string { return &v }

type stubSpecPlugin struct {
	name        string
	initialized bool
	initErr     error
}

func (s *stubSpecPlugin) Name() string {
	if strings.TrimSpace(s.name) == "" {
		return "stub-spec"
	}
	return s.name
}

func (s *stubSpecPlugin) Init(context.Context) error {
	if s.initErr != nil {
		return s.initErr
	}
	s.initialized = true
	return nil
}

func (s *stubSpecPlugin) Close() error { return nil }

func (s *stubSpecPlugin) IsInitialized() bool { return s.initialized }

func (s *stubSpecPlugin) GetContext(context.Context, core.SpecContextRequest) (core.SpecContext, error) {
	return core.SpecContext{}, nil
}
