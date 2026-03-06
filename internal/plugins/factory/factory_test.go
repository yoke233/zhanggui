package factory

import (
	"os"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

func TestFactoryNoCoreCommunicationSlotDependency(t *testing.T) {
	content, err := os.ReadFile("factory.go")
	if err != nil {
		t.Fatalf("read factory.go: %v", err)
	}

	src := string(content)
	for _, legacy := range []string{
		"core.SlotAgent",
		"core.SlotRuntime",
	} {
		if strings.Contains(src, legacy) {
			t.Fatalf("legacy reference still exists in factory: %s", legacy)
		}
	}
}

func TestFactoryBuildKnownPlugin(t *testing.T) {
	cfg := config.Defaults()
	cfg.A2A.Token = "test-token"
	cfg.Store.Path = ":memory:"

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig returned error: %v", err)
	}
	defer set.Store.Close()

	if set.Store == nil {
		t.Fatal("expected store to be initialized")
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
	if set.Workspace == nil {
		t.Fatal("expected workspace plugin to be initialized")
	}
	if set.Workspace.Name() != "workspace-worktree" {
		t.Fatalf("expected workspace plugin name workspace-worktree, got %q", set.Workspace.Name())
	}
	if set.RoleResolver == nil {
		t.Fatal("expected role resolver to be initialized")
	}
}

func TestFactoryBuildsRoleResolver(t *testing.T) {
	cfg := config.Defaults()
	cfg.A2A.Token = "test-token"
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

func TestFactoryBuildsRoleResolver_TeamLeaderMCPEnabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.A2A.Token = "test-token"
	cfg.Store.Path = ":memory:"

	set, err := BuildFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildFromConfig returned error: %v", err)
	}
	defer set.Store.Close()

	_, role, err := set.RoleResolver.Resolve("team_leader")
	if err != nil {
		t.Fatalf("resolve team_leader failed: %v", err)
	}
	if !role.MCPEnabled {
		t.Fatal("expected team_leader role to have MCPEnabled=true from Defaults()")
	}

	// Verify MCPToolsFromRoleConfig produces non-nil servers.
	mcpEnv := teamleader.MCPEnvConfig{
		DBPath:     "/tmp/test.db",
		ServerAddr: "http://127.0.0.1:8080",
	}
	servers := teamleader.MCPToolsFromRoleConfig(role, mcpEnv, true)
	if len(servers) == 0 {
		t.Fatal("expected MCPToolsFromRoleConfig to return at least 1 McpServer for team_leader")
	}
	if servers[0].Sse == nil {
		t.Fatal("expected SSE mode McpServer when ServerAddr is set")
	}

	// Without SSE support, should fallback to stdio.
	stdioServers := teamleader.MCPToolsFromRoleConfig(role, mcpEnv, false)
	if len(stdioServers) == 0 {
		t.Fatal("expected MCPToolsFromRoleConfig to return at least 1 McpServer for team_leader (stdio fallback)")
	}
	if stdioServers[0].Stdio == nil {
		t.Fatal("expected stdio fallback when SSE not supported")
	}
}

func TestFactoryBuildsRoleResolver_TrimmedNamesResolve(t *testing.T) {
	cfg := config.Defaults()
	cfg.A2A.Token = "test-token"
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
		TeamLeader: config.SingleRoleBinding{
			Role: "worker",
		},
		Run: config.RunRoleBindings{
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
	cfg.A2A.Token = "test-token"
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

func TestFactoryBuildReviewGateCanSwitchToLocal(t *testing.T) {
	cfg := config.Defaults()
	cfg.A2A.Token = "test-token"
	cfg.Store.Path = ":memory:"
	cfg.TeamLeader.ReviewGatePlugin = "review-local"

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
	cfg.A2A.Token = "test-token"
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
