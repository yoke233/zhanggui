package config

import (
	"slices"
	"testing"
)

func TestMergeAgentConfig(t *testing.T) {
	global := &AgentConfig{Binary: ptr("claude"), MaxTurns: ptr(30)}
	project := &AgentConfig{MaxTurns: ptr(50)}

	merged := MergeAgentConfig(global, project)

	if *merged.Binary != "claude" {
		t.Errorf("expected binary claude, got %s", *merged.Binary)
	}
	if *merged.MaxTurns != 50 {
		t.Errorf("expected max_turns 50, got %d", *merged.MaxTurns)
	}
}

func TestMergeAgentConfig_CapabilitiesMaxOverride(t *testing.T) {
	baseCaps := &CapabilitiesConfig{FSRead: true, FSWrite: false, Terminal: false}
	overrideCaps := &CapabilitiesConfig{FSRead: true, FSWrite: true, Terminal: true}
	global := &AgentConfig{CapabilitiesMax: baseCaps}
	project := &AgentConfig{CapabilitiesMax: overrideCaps}

	merged := MergeAgentConfig(global, project)
	if merged.CapabilitiesMax == nil {
		t.Fatal("expected capabilities_max to be merged")
	}
	if !merged.CapabilitiesMax.FSRead || !merged.CapabilitiesMax.FSWrite || !merged.CapabilitiesMax.Terminal {
		t.Fatalf("expected merged capabilities_max from override, got %+v", *merged.CapabilitiesMax)
	}

	// Ensure merged config holds its own copy.
	overrideCaps.FSWrite = false
	if !merged.CapabilitiesMax.FSWrite {
		t.Fatal("expected merged capabilities_max to be independent copy")
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Run.DefaultTemplate != "standard" {
		t.Errorf("expected default template standard, got %s", cfg.Run.DefaultTemplate)
	}
	if cfg.Scheduler.MaxGlobalAgents != 3 {
		t.Errorf("expected max_global_agents 3, got %d", cfg.Scheduler.MaxGlobalAgents)
	}
	if cfg.TeamLeader.ReviewGatePlugin != "review-ai-panel" {
		t.Errorf("expected team_leader.review_gate_plugin review-ai-panel, got %s", cfg.TeamLeader.ReviewGatePlugin)
	}
	if cfg.TeamLeader.ReviewOrchestrator.MaxRounds != 2 {
		t.Errorf("expected team_leader.review_orchestrator.max_rounds 2, got %d", cfg.TeamLeader.ReviewOrchestrator.MaxRounds)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected server host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected server port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoadDefaults_UsesACPAgentProfiles(t *testing.T) {
	cfg := Defaults()
	agents := cfg.EffectiveAgentProfiles()
	if len(agents) == 0 {
		t.Fatal("expected non-empty effective agent profiles")
	}

	byName := make(map[string]AgentProfileConfig, len(agents))
	for _, agent := range agents {
		byName[agent.Name] = agent
	}

	claude, ok := byName["claude"]
	if !ok {
		t.Fatal("expected default claude agent profile")
	}
	if claude.LaunchCommand != "npx" {
		t.Fatalf("expected claude launch command npx, got %q", claude.LaunchCommand)
	}
	if !slices.Equal(claude.LaunchArgs, []string{"-y", "@zed-industries/claude-agent-acp@latest"}) {
		t.Fatalf("unexpected claude launch args: %#v", claude.LaunchArgs)
	}

	codex, ok := byName["codex"]
	if !ok {
		t.Fatal("expected default codex agent profile")
	}
	if codex.LaunchCommand != "npx" {
		t.Fatalf("expected codex launch command npx, got %q", codex.LaunchCommand)
	}
	if !slices.Equal(codex.LaunchArgs, []string{"-y", "@zed-industries/codex-acp@latest"}) {
		t.Fatalf("unexpected codex launch args: %#v", codex.LaunchArgs)
	}
}

func TestLoadDefaults_TeamLeaderRoleUsesClaude(t *testing.T) {
	cfg := Defaults()
	teamLeaderAgent := ""
	for _, role := range cfg.Roles {
		if role.Name == "team_leader" {
			teamLeaderAgent = role.Agent
			break
		}
	}
	if teamLeaderAgent != "claude" {
		t.Fatalf("expected team_leader role bind to claude agent, got %q", teamLeaderAgent)
	}
}

func TestLoadDefaults_TeamLeaderRoleBindingDefault(t *testing.T) {
	cfg := Defaults()
	if got := cfg.RoleBinds.TeamLeader.Role; got != "team_leader" {
		t.Fatalf("expected role_bindings.team_leader default to team_leader role, got %q", got)
	}
}

func TestConfig_Defaults_GitHub(t *testing.T) {
	cfg := Defaults()
	if cfg.GitHub.Enabled {
		t.Fatalf("expected github.enabled default false, got true")
	}
	if cfg.GitHub.Token != "" {
		t.Fatalf("expected github.token default empty, got %q", cfg.GitHub.Token)
	}
	if cfg.GitHub.AppID != 0 {
		t.Fatalf("expected github.app_id default 0, got %d", cfg.GitHub.AppID)
	}
	if cfg.GitHub.PrivateKeyPath != "" {
		t.Fatalf("expected github.private_key_path default empty, got %q", cfg.GitHub.PrivateKeyPath)
	}
	if cfg.GitHub.InstallationID != 0 {
		t.Fatalf("expected github.installation_id default 0, got %d", cfg.GitHub.InstallationID)
	}
	if cfg.GitHub.WebhookSecret != "" {
		t.Fatalf("expected github.webhook_secret default empty, got %q", cfg.GitHub.WebhookSecret)
	}
}

func TestA2ADefaults_DisabledByDefault(t *testing.T) {
	cfg := Defaults()
	if cfg.A2A.Enabled {
		t.Fatal("expected a2a.enabled default false, got true")
	}
	if cfg.A2A.Token != "" {
		t.Fatalf("expected a2a.token default empty, got %q", cfg.A2A.Token)
	}
	if cfg.A2A.Version != "0.3" {
		t.Fatalf("expected a2a.version default 0.3, got %q", cfg.A2A.Version)
	}
}

func TestA2AToken_CanBeReadFromConfig(t *testing.T) {
	cfg, err := loadAndValidate(t, `
a2a:
  token: "a2a-token"
`)
	if err != nil {
		t.Fatalf("loadAndValidate failed: %v", err)
	}
	if cfg.A2A.Token != "a2a-token" {
		t.Fatalf("expected a2a.token loaded, got %q", cfg.A2A.Token)
	}
}

func TestA2AEnabledWithoutToken_FailFast(t *testing.T) {
	_, err := loadAndValidate(t, `
a2a:
  enabled: true
  token: ""
`)
	if err == nil {
		t.Fatal("expected enabled a2a with empty token to fail")
	}
}

func TestA2AEnabledWithoutToken_NoServerAuthFallback(t *testing.T) {
	_, err := loadAndValidate(t, `
server:
  auth_token: "legacy-token"
a2a:
  enabled: true
  token: ""
`)
	if err == nil {
		t.Fatal("expected enabled a2a with empty token to fail even when server.auth_token is set")
	}
}

func TestRoleDrivenConfigLoadTeamLeaderBinding(t *testing.T) {
	cfg := loadTestConfig(t, `
agents:
  - name: claude
    launch_command: claude-agent-acp
    launch_args: []
    env: {}
    capabilities_max:
      fs_read: true
      fs_write: true
      terminal: true
roles:
  - name: worker
    agent: claude
    prompt_template: implement
    capabilities:
      fs_read: true
      fs_write: true
      terminal: true
    session:
      reuse: true
role_bindings:
  team_leader:
    role: worker
  Run:
    stage_roles:
      implement: worker
  review_orchestrator:
    reviewers: {}
    aggregator: worker
  plan_parser:
    role: worker
`)

	agents := cfg.EffectiveAgentProfiles()
	if got := len(agents); got != 1 {
		t.Fatalf("expected one role-driven agent, got %d", got)
	}
	if got := agents[0].LaunchCommand; got != "claude-agent-acp" {
		t.Fatalf("expected launch command claude-agent-acp, got %q", got)
	}
	if got := cfg.RoleBinds.Run.StageRoles["implement"]; got != "worker" {
		t.Fatalf("expected stage role implement=worker, got %q", got)
	}
	if got := cfg.RoleBinds.TeamLeader.Role; got != "worker" {
		t.Fatalf("expected role_bindings.team_leader.role=worker, got %q", got)
	}
}

func TestRoleDrivenConfigUnknownFieldFailFast(t *testing.T) {
	const raw = `
agents:
  - name: claude
    launch_command: claude-agent-acp
    capabilities_maxx:
      fs_read: true
`
	layer, err := loadLayerFromBytes([]byte(raw))
	if err == nil || layer != nil {
		t.Fatalf("expected strict unknown-field error, got layer=%v err=%v", layer, err)
	}
}

func TestRoleDrivenConfigCapabilityOverflowFailFast(t *testing.T) {
	_, err := loadAndValidate(t, `
agents:
  - name: claude
    launch_command: claude-agent-acp
    capabilities_max:
      fs_read: true
      fs_write: false
      terminal: false
roles:
  - name: worker
    agent: claude
    capabilities:
      fs_read: true
      fs_write: true
      terminal: false
`)
	if err == nil {
		t.Fatal("expected capability overflow error, got nil")
	}
}

func TestApplyConfigLayer_RoleBindingsPartialOverrideKeepsOtherBindings(t *testing.T) {
	cfg := Defaults()
	layer, err := loadLayerFromBytes([]byte(`
role_bindings:
  Run:
    stage_roles:
      implement: reviewer
`))
	if err != nil {
		t.Fatalf("load layer failed: %v", err)
	}

	ApplyConfigLayer(&cfg, layer)

	if got := cfg.RoleBinds.TeamLeader.Role; got != "team_leader" {
		t.Fatalf("expected team_leader role binding kept, got %q", got)
	}
	if got := cfg.RoleBinds.PlanParser.Role; got != "plan_parser" {
		t.Fatalf("expected plan_parser role binding kept, got %q", got)
	}
	if got := cfg.RoleBinds.ReviewOrchestrator.Aggregator; got != "aggregator" {
		t.Fatalf("expected review aggregator binding kept, got %q", got)
	}
	if got := cfg.RoleBinds.Run.StageRoles["implement"]; got != "reviewer" {
		t.Fatalf("expected Run implement role overwritten to reviewer, got %q", got)
	}
}

func TestRoleDrivenConfigLegacySecretaryBindingFailFast(t *testing.T) {
	const raw = `
role_bindings:
  TeamLeader:
    role: worker
`
	layer, err := loadLayerFromBytes([]byte(raw))
	if err == nil || layer != nil {
		t.Fatalf("expected strict unknown-field error for legacy role_bindings.TeamLeader, got layer=%v err=%v", layer, err)
	}
}

func loadTestConfig(t *testing.T, raw string) Config {
	t.Helper()
	cfg, err := loadAndValidate(t, raw)
	if err != nil {
		t.Fatalf("loadAndValidate failed: %v", err)
	}
	return cfg
}

func loadAndValidate(t *testing.T, raw string) (Config, error) {
	t.Helper()
	cfg := Defaults()
	layer, err := loadLayerFromBytes([]byte(raw))
	if err != nil {
		return Config{}, err
	}
	ApplyConfigLayer(&cfg, layer)
	if err := validateConfig(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ptr[T any](v T) *T { return &v }
