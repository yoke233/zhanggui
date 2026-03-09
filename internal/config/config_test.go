package config

import (
	"slices"
	"strings"
	"testing"
	"time"
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
	if cfg.Store.Path != ".ai-workflow/data.db" {
		t.Errorf("expected default store path .ai-workflow/data.db, got %s", cfg.Store.Path)
	}
	if cfg.Log.File != ".ai-workflow/logs/app.log" {
		t.Errorf("expected default log file .ai-workflow/logs/app.log, got %s", cfg.Log.File)
	}
	if !cfg.Scheduler.Watchdog.Enabled {
		t.Fatal("expected scheduler.watchdog.enabled default true, got false")
	}
	if got := cfg.Scheduler.Watchdog.Interval.Duration; got != 5*time.Minute {
		t.Fatalf("expected scheduler.watchdog.interval 5m, got %s", got)
	}
	if got := cfg.Scheduler.Watchdog.StuckRunTTL.Duration; got != 30*time.Minute {
		t.Fatalf("expected scheduler.watchdog.stuck_run_ttl 30m, got %s", got)
	}
	if got := cfg.Scheduler.Watchdog.StuckMergeTTL.Duration; got != 15*time.Minute {
		t.Fatalf("expected scheduler.watchdog.stuck_merge_ttl 15m, got %s", got)
	}
	if got := cfg.Scheduler.Watchdog.QueueStaleTTL.Duration; got != 60*time.Minute {
		t.Fatalf("expected scheduler.watchdog.queue_stale_ttl 60m, got %s", got)
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
	if !slices.Equal(claude.LaunchArgs, []string{"-y", "@zed-industries/claude-agent-acp"}) {
		t.Fatalf("unexpected claude launch args: %#v", claude.LaunchArgs)
	}

	codex, ok := byName["codex"]
	if !ok {
		t.Fatal("expected default codex agent profile")
	}
	if codex.LaunchCommand != "npx" {
		t.Fatalf("expected codex launch command npx, got %q", codex.LaunchCommand)
	}
	if !slices.Equal(codex.LaunchArgs, []string{"-y", "@zed-industries/codex-acp"}) {
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

func TestA2ADefaults_EnabledByDefault(t *testing.T) {
	cfg := Defaults()
	if !cfg.A2A.Enabled {
		t.Fatal("expected a2a.enabled default true, got false")
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
[a2a]
token = "a2a-token"
`)
	if err != nil {
		t.Fatalf("loadAndValidate failed: %v", err)
	}
	if cfg.A2A.Token != "a2a-token" {
		t.Fatalf("expected a2a.token loaded, got %q", cfg.A2A.Token)
	}
}

func TestA2AEnabledWithoutToken_PassesValidation(t *testing.T) {
	cfg, err := loadAndValidate(t, `
[a2a]
enabled = true
token = ""
`)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if !cfg.A2A.Enabled {
		t.Fatal("expected a2a.enabled=true")
	}
}

func TestEnsureSecrets_AutoGeneratesAdminToken(t *testing.T) {
	s := &Secrets{Tokens: map[string]TokenEntry{}}
	changed := EnsureSecrets(s)
	if !changed {
		t.Fatal("expected changed=true")
	}
	token := s.AdminToken()
	if token == "" || len(token) != 32 {
		t.Fatalf("expected 32-char hex admin token, got %q", token)
	}
	entry := s.Tokens["admin"]
	if len(entry.Scopes) != 1 || entry.Scopes[0] != "*" {
		t.Fatalf("expected admin scopes=[*], got %v", entry.Scopes)
	}
}

func TestEnsureSecrets_DoesNotOverwriteExisting(t *testing.T) {
	s := &Secrets{Tokens: map[string]TokenEntry{
		"admin": {Token: "existing-token", Scopes: []string{"*"}},
	}}
	changed := EnsureSecrets(s)
	if changed {
		t.Fatal("expected changed=false when admin token already set")
	}
	if s.AdminToken() != "existing-token" {
		t.Fatalf("expected token unchanged, got %q", s.AdminToken())
	}
}

func TestApplySecrets_GitHubSecretsToConfig(t *testing.T) {
	cfg := Defaults()
	s := &Secrets{
		Tokens: map[string]TokenEntry{
			"admin": {Token: "my-token", Scopes: []string{"*"}},
		},
		GitHub: GitHubSecrets{Token: "gh-token"},
	}
	ApplySecrets(&cfg, s)
	if cfg.GitHub.Token != "gh-token" {
		t.Fatalf("expected github.token = gh-token, got %q", cfg.GitHub.Token)
	}
}

func TestRoleDrivenConfigLoadTeamLeaderBinding(t *testing.T) {
	cfg := loadTestConfig(t, `
[[agents.profiles]]
name = "claude"
launch_command = "claude-agent-acp"
launch_args = []
[agents.profiles.env]
[agents.profiles.capabilities_max]
fs_read = true
fs_write = true
terminal = true

[[roles]]
name = "worker"
agent = "claude"
prompt_template = "implement"
[roles.capabilities]
fs_read = true
fs_write = true
terminal = true
[roles.session]
reuse = true

[role_bindings.team_leader]
role = "worker"
[role_bindings.run.stage_roles]
implement = "worker"
[role_bindings.review_orchestrator]
aggregator = "worker"
[role_bindings.review_orchestrator.reviewers]
[role_bindings.plan_parser]
role = "worker"
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
[[agents.profiles]]
name = "claude"
launch_command = "claude-agent-acp"
[agents.profiles.capabilities_maxx]
fs_read = true
`
	layer, err := loadLayerFromBytes([]byte(raw))
	if err == nil || layer != nil {
		t.Fatalf("expected strict unknown-field error, got layer=%v err=%v", layer, err)
	}
}

func TestRoleDrivenConfigCapabilityOverflowFailFast(t *testing.T) {
	_, err := loadAndValidate(t, `
[[agents.profiles]]
name = "claude"
launch_command = "claude-agent-acp"
[agents.profiles.capabilities_max]
fs_read = true
fs_write = false
terminal = false

[[roles]]
name = "worker"
agent = "claude"
[roles.capabilities]
fs_read = true
fs_write = true
terminal = false
`)
	if err == nil {
		t.Fatal("expected capability overflow error, got nil")
	}
}

func TestApplyConfigLayer_RoleBindingsPartialOverrideKeepsOtherBindings(t *testing.T) {
	cfg := Defaults()
	layer, err := loadLayerFromBytes([]byte(`
[role_bindings.run.stage_roles]
implement = "reviewer"
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

func TestApplyConfigLayer_SchedulerWatchdogOverride(t *testing.T) {
	cfg := Defaults()
	layer, err := loadLayerFromBytes([]byte(`
[scheduler.watchdog]
enabled = false
interval = "2m"
stuck_run_ttl = "45m"
stuck_merge_ttl = "20m"
queue_stale_ttl = "90m"
`))
	if err != nil {
		t.Fatalf("load layer failed: %v", err)
	}

	ApplyConfigLayer(&cfg, layer)

	if cfg.Scheduler.Watchdog.Enabled {
		t.Fatal("expected scheduler.watchdog.enabled override false, got true")
	}
	if got := cfg.Scheduler.Watchdog.Interval.Duration; got != 2*time.Minute {
		t.Fatalf("expected interval 2m, got %s", got)
	}
	if got := cfg.Scheduler.Watchdog.StuckRunTTL.Duration; got != 45*time.Minute {
		t.Fatalf("expected stuck_run_ttl 45m, got %s", got)
	}
	if got := cfg.Scheduler.Watchdog.StuckMergeTTL.Duration; got != 20*time.Minute {
		t.Fatalf("expected stuck_merge_ttl 20m, got %s", got)
	}
	if got := cfg.Scheduler.Watchdog.QueueStaleTTL.Duration; got != 90*time.Minute {
		t.Fatalf("expected queue_stale_ttl 90m, got %s", got)
	}
}

func TestMergeForRun_SchedulerWatchdogOverride(t *testing.T) {
	global := Defaults()
	global.A2A.Token = "test-default-token"

	merged, err := MergeForRun(&global, nil, map[string]any{
		"scheduler": map[string]any{
			"watchdog": map[string]any{
				"enabled":         false,
				"interval":        "2m",
				"stuck_run_ttl":   "45m",
				"stuck_merge_ttl": "20m",
				"queue_stale_ttl": "90m",
			},
		},
	})
	if err != nil {
		t.Fatalf("MergeForRun() error = %v", err)
	}

	if merged.Scheduler.Watchdog.Enabled {
		t.Fatal("expected merged watchdog.enabled false, got true")
	}
	if got := merged.Scheduler.Watchdog.Interval.Duration; got != 2*time.Minute {
		t.Fatalf("expected merged interval 2m, got %s", got)
	}
	if got := merged.Scheduler.Watchdog.StuckRunTTL.Duration; got != 45*time.Minute {
		t.Fatalf("expected merged stuck_run_ttl 45m, got %s", got)
	}
	if got := merged.Scheduler.Watchdog.StuckMergeTTL.Duration; got != 20*time.Minute {
		t.Fatalf("expected merged stuck_merge_ttl 20m, got %s", got)
	}
	if got := merged.Scheduler.Watchdog.QueueStaleTTL.Duration; got != 90*time.Minute {
		t.Fatalf("expected merged queue_stale_ttl 90m, got %s", got)
	}
}

func TestLoadLayerFromYAML_SchedulerWatchdog(t *testing.T) {
	layer, err := loadLayerFromYAML([]byte(`
scheduler:
  watchdog:
    enabled: false
    interval: 2m
    stuck_run_ttl: 45m
    stuck_merge_ttl: 20m
    queue_stale_ttl: 90m
`))
	if err != nil {
		t.Fatalf("loadLayerFromYAML() error = %v", err)
	}
	if layer.Scheduler == nil || layer.Scheduler.Watchdog == nil {
		t.Fatal("expected scheduler.watchdog layer to be loaded")
	}
	if layer.Scheduler.Watchdog.Enabled == nil || *layer.Scheduler.Watchdog.Enabled {
		t.Fatal("expected scheduler.watchdog.enabled false")
	}
	if layer.Scheduler.Watchdog.Interval == nil {
		t.Fatal("expected scheduler.watchdog.interval to be loaded")
	}
	if got := layer.Scheduler.Watchdog.Interval.Duration; got != 2*time.Minute {
		t.Fatalf("expected scheduler.watchdog.interval 2m, got %s", got)
	}
}

func TestLoadLayerFromYAML_SchedulerWatchdogInvalidDuration(t *testing.T) {
	_, err := loadLayerFromYAML([]byte(`
scheduler:
  watchdog:
    interval: nope
`))
	if err == nil {
		t.Fatal("expected invalid duration error, got nil")
	}
	if !strings.Contains(err.Error(), "parse duration") {
		t.Fatalf("expected parse duration error, got %v", err)
	}
}

func TestValidateConfig_WatchdogRejectsNonPositiveDurations(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name: "interval zero",
			raw: `
[scheduler.watchdog]
enabled = true
interval = "0s"
`,
			wantErr: "scheduler.watchdog.interval must be > 0",
		},
		{
			name: "stuck run ttl negative",
			raw: `
[scheduler.watchdog]
enabled = true
stuck_run_ttl = "-1m"
`,
			wantErr: "scheduler.watchdog.stuck_run_ttl must be > 0",
		},
		{
			name: "stuck merge ttl zero",
			raw: `
[scheduler.watchdog]
enabled = true
stuck_merge_ttl = "0s"
`,
			wantErr: "scheduler.watchdog.stuck_merge_ttl must be > 0",
		},
		{
			name: "queue stale ttl negative",
			raw: `
[scheduler.watchdog]
enabled = true
queue_stale_ttl = "-5m"
`,
			wantErr: "scheduler.watchdog.queue_stale_ttl must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadAndValidate(t, tt.raw)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestRoleDrivenConfigLegacyUnknownFieldFailFast(t *testing.T) {
	const raw = `
[role_bindings.TeamLeader]
role = "worker"
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
	// Provide a default token so tests don't fail on the a2a.token check
	// unless they explicitly test that scenario.
	if cfg.A2A.Token == "" {
		cfg.A2A.Token = "test-default-token"
	}
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
