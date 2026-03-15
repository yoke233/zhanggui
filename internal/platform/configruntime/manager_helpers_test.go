package configruntime

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func TestValidationErrorHelpers(t *testing.T) {
	var nilErr *ValidationError
	if got := nilErr.Error(); got != ErrInvalidConfig.Error() {
		t.Fatalf("nil ValidationError.Error() = %q", got)
	}
	if got := nilErr.Unwrap(); got != nil {
		t.Fatalf("nil ValidationError.Unwrap() = %v, want nil", got)
	}

	base := errors.New("bad config")
	ve := &ValidationError{Err: base}
	if got := ve.Error(); got != "bad config" {
		t.Fatalf("ValidationError.Error() = %q", got)
	}
	if !errors.Is(ve, base) {
		t.Fatalf("ValidationError should unwrap to base error")
	}
}

func TestBuildAgentsAndResolveDriverConfig(t *testing.T) {
	if got := BuildAgents(nil); got != nil {
		t.Fatalf("BuildAgents(nil) = %#v, want nil", got)
	}

	cfg := &config.Config{}
	cfg.Runtime.Agents = config.RuntimeAgentsConfig{
		Drivers: []config.RuntimeDriverConfig{{
			ID:            "codex",
			LaunchCommand: " codex ",
			LaunchArgs:    []string{"exec"},
			Env:           map[string]string{"MODE": "test"},
			CapabilitiesMax: config.CapabilitiesConfig{
				FSRead:   true,
				FSWrite:  true,
				Terminal: true,
			},
		}},
		Profiles: []config.RuntimeProfileConfig{{
			ID:             "worker",
			Name:           "Worker",
			Driver:         "codex",
			Role:           "worker",
			Capabilities:   []string{"backend"},
			ActionsAllowed: []string{"read_context", "terminal"},
			PromptTemplate: "worker",
			Skills:         []string{"skill-a"},
			Session: config.RuntimeSessionConfig{
				Reuse:              true,
				MaxTurns:           3,
				IdleTTL:            config.Duration{Duration: time.Minute},
				ThreadBootTemplate: "boot",
				MaxContextTokens:   99,
				ContextWarnRatio:   0.75,
			},
			MCP: config.MCPConfig{
				Enabled: true,
				Tools:   []string{"shell_command"},
			},
		}},
	}

	profiles := BuildAgents(cfg)
	if len(profiles) != 1 || profiles[0].Driver.LaunchCommand != " codex " {
		t.Fatalf("BuildAgents() = %#v", profiles)
	}
	profiles[0].Capabilities[0] = "frontend"
	profiles[0].Driver.LaunchArgs[0] = "run"
	profiles[0].Driver.Env["MODE"] = "prod"
	if cfg.Runtime.Agents.Profiles[0].Capabilities[0] != "backend" {
		t.Fatalf("BuildAgents should deep copy profile capabilities")
	}
	if cfg.Runtime.Agents.Drivers[0].LaunchArgs[0] != "exec" || cfg.Runtime.Agents.Drivers[0].Env["MODE"] != "test" {
		t.Fatalf("BuildAgents should deep copy driver config")
	}

	manager := &Manager{logger: slog.Default()}
	manager.current.Store(&Snapshot{Config: cfg})

	driverCfg, err := manager.ResolveDriverConfig(" codex ")
	if err != nil {
		t.Fatalf("ResolveDriverConfig() error = %v", err)
	}
	driverCfg.LaunchArgs[0] = "changed"
	driverCfg.Env["MODE"] = "changed"
	if cfg.Runtime.Agents.Drivers[0].LaunchArgs[0] != "exec" || cfg.Runtime.Agents.Drivers[0].Env["MODE"] != "test" {
		t.Fatalf("ResolveDriverConfig should clone slices/maps")
	}
	if _, err := manager.ResolveDriverConfig(""); err == nil {
		t.Fatalf("ResolveDriverConfig(empty) should fail")
	}
	if _, err := manager.ResolveDriverConfig("missing"); err == nil {
		t.Fatalf("ResolveDriverConfig(missing) should fail")
	}
}

func TestManagerStartCloseAndWriteRawValidation(t *testing.T) {
	manager, _ := newManagerForTest(t)
	if err := (&Manager{}).Close(); err != nil {
		t.Fatalf("Close(nil watcher) error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if manager.watcher == nil {
		t.Fatal("Start() should initialize watcher")
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close(active watcher) error = %v", err)
	}

	if _, err := manager.WriteRaw(context.Background(), "  "); err == nil {
		t.Fatalf("WriteRaw(empty) should fail")
	}
}

func TestManagerRuntimeAndDriverErrorBranches(t *testing.T) {
	empty := &Manager{logger: slog.Default()}
	if got := empty.GetRuntime(); got.Sandbox.Provider != "" || got.LLM.DefaultConfigID != "" || len(got.Agents.Drivers) != 0 || len(got.MCP.Servers) != 0 {
		t.Fatalf("GetRuntime() = %#v, want zero value", got)
	}
	if got := empty.ListDriverConfigs(); got != nil {
		t.Fatalf("ListDriverConfigs() = %#v, want nil", got)
	}
	if _, _, ok := empty.CurrentConfig(); ok {
		t.Fatalf("CurrentConfig() ok = true, want false")
	}

	manager, _ := newManagerForTest(t)
	if _, err := manager.CreateDriverConfig(context.Background(), config.RuntimeDriverConfig{}); err == nil {
		t.Fatalf("CreateDriverConfig(empty id) should fail")
	}
	if _, err := manager.CreateDriverConfig(context.Background(), config.RuntimeDriverConfig{ID: "codex"}); err != nil {
		t.Fatalf("CreateDriverConfig(codex) error = %v", err)
	}
	if _, err := manager.CreateDriverConfig(context.Background(), config.RuntimeDriverConfig{ID: "codex"}); err == nil {
		t.Fatalf("CreateDriverConfig(duplicate) should fail")
	}

	if _, err := manager.UpdateDriverConfig(context.Background(), "", config.RuntimeDriverConfig{}); err == nil {
		t.Fatalf("UpdateDriverConfig(empty id) should fail")
	}
	if _, err := manager.UpdateDriverConfig(context.Background(), "missing", config.RuntimeDriverConfig{}); err == nil {
		t.Fatalf("UpdateDriverConfig(missing) should fail")
	}
	if _, err := manager.DeleteDriverConfig(context.Background(), ""); err == nil {
		t.Fatalf("DeleteDriverConfig(empty id) should fail")
	}
	if _, err := manager.DeleteDriverConfig(context.Background(), "missing"); err == nil {
		t.Fatalf("DeleteDriverConfig(missing) should fail")
	}
}

func TestManagerWriteRawRollbackAndReloadErrors(t *testing.T) {
	manager, initialRaw := newManagerForTest(t)
	calls := 0
	manager.onReload = func(context.Context, *Snapshot) error {
		calls++
		if calls == 1 {
			return errors.New("reload failed")
		}
		return nil
	}

	_, err := manager.WriteRaw(context.Background(), "[runtime]\n")
	if err == nil || err.Error() != "reload failed" {
		t.Fatalf("WriteRaw() error = %v, want reload failed", err)
	}
	raw, readErr := manager.ReadRawString()
	if readErr != nil {
		t.Fatalf("ReadRawString() error = %v", readErr)
	}
	if raw != initialRaw {
		t.Fatalf("WriteRaw() should rollback raw content, got %q want %q", raw, initialRaw)
	}

	manager.onReload = nil
	if _, err := manager.Reload(context.Background(), "manual"); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	manager.configPath = filepath.Join(t.TempDir(), "missing", "config.toml")
	if _, err := manager.reloadLocked(context.Background(), "invalid-path"); err == nil {
		t.Fatalf("reloadLocked(invalid path) should fail")
	}
}

func TestManagerStartInvalidDirFails(t *testing.T) {
	manager := &Manager{
		configPath: filepath.Join(t.TempDir(), "missing", "config.toml"),
		logger:     slog.Default(),
	}
	if err := manager.Start(context.Background()); err == nil {
		t.Fatalf("Start(invalid dir) should fail")
	}
}

func TestStructTomlHelpers(t *testing.T) {
	raw, err := StructToTomlMap(config.RuntimeDriverConfig{
		ID:            "codex",
		LaunchCommand: "run",
	})
	if err != nil {
		t.Fatalf("StructToTomlMap() error = %v", err)
	}
	if raw["id"] != "codex" {
		t.Fatalf("StructToTomlMap() = %#v", raw)
	}

	var decoded config.RuntimeDriverConfig
	if err := TomlMapToStruct(map[string]any{
		"id":             "worker",
		"launch_command": "npx",
	}, &decoded); err != nil {
		t.Fatalf("TomlMapToStruct() error = %v", err)
	}
	if decoded.ID != "worker" || decoded.LaunchCommand != "npx" {
		t.Fatalf("TomlMapToStruct() = %#v", decoded)
	}

	if err := TomlMapToStruct(map[string]any{"unknown": "field"}, &decoded); err == nil {
		t.Fatalf("TomlMapToStruct(unknown field) should fail")
	}
	if _, err := StructToTomlMap(make(chan int)); err == nil {
		t.Fatalf("StructToTomlMap(unmarshalable) should fail")
	}
	if err := TomlMapToStruct(map[string]any{"bad": make(chan int)}, &decoded); err == nil {
		t.Fatalf("TomlMapToStruct(unmarshalable map) should fail")
	}
}

func TestResolveSecretRefAndHelpers(t *testing.T) {
	if _, err := resolveSecretRef(nil, "tokens.api"); err == nil {
		t.Fatalf("resolveSecretRef(nil, ref) should fail")
	}
	if got, err := resolveSecretRef(&config.Secrets{}, ""); err != nil || got != "" {
		t.Fatalf("resolveSecretRef(empty) = %q, %v", got, err)
	}
	if _, err := resolveSecretRef(&config.Secrets{}, "github.token"); err == nil {
		t.Fatalf("resolveSecretRef(unsupported) should fail")
	}
	if _, err := resolveSecretRef(&config.Secrets{Tokens: map[string]config.TokenEntry{}}, "tokens.api"); err == nil {
		t.Fatalf("resolveSecretRef(missing token) should fail")
	}
	got, err := resolveSecretRef(&config.Secrets{
		Tokens: map[string]config.TokenEntry{
			"api": {Token: " secret-token "},
		},
	}, "tokens.api")
	if err != nil || got != "secret-token" {
		t.Fatalf("resolveSecretRef(found) = %q, %v", got, err)
	}

	keys := mapKeys(map[string][]int{"a": {1}, "b": {2}})
	if len(keys) != 2 {
		t.Fatalf("mapKeys() = %#v", keys)
	}
	if cloneStringMap(nil) != nil {
		t.Fatalf("cloneStringMap(nil) should return nil")
	}
	cloned := cloneStringMap(map[string]string{"A": "1"})
	cloned["A"] = "2"
	if cloned["A"] != "2" {
		t.Fatalf("cloneStringMap() clone not writable")
	}
}

func TestBuildMCPServersBindingsAndResolve(t *testing.T) {
	cfg := &config.Config{}
	cfg.Runtime.MCP = config.RuntimeMCPConfig{
		Servers: []config.RuntimeMCPServerConfig{
			{
				ID:            "sse-server",
				Transport:     "sse",
				Endpoint:      "https://example.com/mcp",
				AuthSecretRef: "tokens.api",
				Enabled:       true,
			},
			{
				ID:        "stdio-server",
				Name:      "stdio-name",
				Transport: "stdio",
				Command:   "node",
				Args:      []string{"mcp.js"},
				Env:       map[string]string{"MODE": "test"},
				Enabled:   true,
			},
		},
		ProfileBindings: []config.RuntimeMCPProfileBindingConfig{{
			Profile:  "worker",
			Server:   "sse-server",
			Enabled:  true,
			ToolMode: "all",
		}, {
			Profile:  "worker",
			Server:   "stdio-server",
			Enabled:  true,
			ToolMode: "allow_list",
			Tools:    []string{"shell_command"},
		}},
	}
	servers, err := buildMCPServers(cfg, &config.Secrets{
		Tokens: map[string]config.TokenEntry{
			"api": {Token: "api-token"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildMCPServers() error = %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("buildMCPServers() = %#v", servers)
	}
	if servers["sse-server"].Name != "sse-server" {
		t.Fatalf("expected blank name to fall back to ID, got %#v", servers["sse-server"])
	}
	if len(servers["sse-server"].Headers) != 1 || servers["sse-server"].Headers[0].Value != "Bearer api-token" {
		t.Fatalf("sse headers = %#v", servers["sse-server"].Headers)
	}

	legacyServers, err := buildMCPServers(&config.Config{}, &config.Secrets{}, []*core.AgentProfile{{
		ID:  "worker",
		MCP: core.ProfileMCP{Enabled: true},
	}})
	if err != nil {
		t.Fatalf("buildMCPServers(legacy) error = %v", err)
	}
	if _, ok := legacyServers[defaultInternalMCPServerID]; !ok {
		t.Fatalf("legacy internal server missing: %#v", legacyServers)
	}

	bindings := buildBindings(cfg, nil)
	if len(bindings["worker"]) != 2 || bindings["worker"][1].ToolMode != "allow_list" {
		t.Fatalf("buildBindings(explicit) = %#v", bindings)
	}

	legacyBindings := buildBindings(&config.Config{}, []*core.AgentProfile{{
		ID:  "worker",
		MCP: core.ProfileMCP{Enabled: true, Tools: []string{"shell_command"}},
	}})
	if len(legacyBindings["worker"]) != 1 || legacyBindings["worker"][0].ServerID != defaultInternalMCPServerID {
		t.Fatalf("buildBindings(legacy) = %#v", legacyBindings)
	}

	manager := &Manager{
		logger: slog.Default(),
		mcpEnv: MCPEnvConfig{
			ServerAddr: "http://127.0.0.1:8080/",
			AuthToken:  "secret",
		},
	}
	manager.current.Store(&Snapshot{
		MCPServersByID:       servers,
		MCPBindingsByProfile: bindings,
	})
	resolved := manager.ResolveMCPServers("worker", true)
	if len(resolved) != 2 || resolved[0].Sse == nil || resolved[1].Stdio == nil {
		t.Fatalf("ResolveMCPServers() = %#v", resolved)
	}
	if resolved[0].Sse.Headers[0].Value != "Bearer api-token" {
		t.Fatalf("resolved sse headers = %#v", resolved[0].Sse.Headers)
	}

	emptyManager := &Manager{logger: slog.Default()}
	if got := emptyManager.ResolveMCPServers("worker", true); got != nil {
		t.Fatalf("ResolveMCPServers(nil snapshot) = %#v, want nil", got)
	}
	emptyManager.current.Store(&Snapshot{
		MCPServersByID:       servers,
		MCPBindingsByProfile: map[string][]MCPProfileBinding{},
	})
	if got := emptyManager.ResolveMCPServers("worker", true); got != nil {
		t.Fatalf("ResolveMCPServers(no bindings) = %#v, want nil", got)
	}

	disabledManager := &Manager{logger: slog.Default()}
	disabledManager.current.Store(&Snapshot{
		MCPServersByID: map[string]MCPServer{
			"disabled": {ID: "disabled", Enabled: false, Transport: "sse", Endpoint: "https://example.com"},
		},
		MCPBindingsByProfile: map[string][]MCPProfileBinding{
			"worker": {
				{ProfileID: "worker", ServerID: "disabled-bind", Enabled: false},
				{ProfileID: "worker", ServerID: "disabled", Enabled: true},
			},
		},
	})
	if got := disabledManager.ResolveMCPServers("worker", true); len(got) != 0 {
		t.Fatalf("ResolveMCPServers(disabled) = %#v, want empty", got)
	}
}

func TestBuildInternalServerVariants(t *testing.T) {
	server := MCPServer{ID: "internal", Name: "query"}

	sse := buildInternalServer(server, MCPEnvConfig{
		ServerAddr: "http://localhost:9000/",
		AuthToken:  "token",
	}, true)
	if len(sse) != 1 || sse[0].Sse == nil || sse[0].Sse.Url != "http://localhost:9000/api/v1/mcp" {
		t.Fatalf("buildInternalServer(sse) = %#v", sse)
	}

	stdio := buildInternalServer(server, MCPEnvConfig{
		DBPath:     "probe.db",
		DevMode:    true,
		SourceRoot: "D:/src",
		ServerAddr: "http://localhost:9000",
	}, false)
	if len(stdio) != 1 || stdio[0].Stdio == nil {
		t.Fatalf("buildInternalServer(stdio) = %#v", stdio)
	}
	if stdio[0].Stdio.Args[0] != "mcp-serve" {
		t.Fatalf("stdio args = %#v", stdio[0].Stdio.Args)
	}
	if len(stdio[0].Stdio.Env) < 2 {
		t.Fatalf("stdio env = %#v", stdio[0].Stdio.Env)
	}

	if got := buildInternalServer(server, MCPEnvConfig{}, false); got != nil {
		t.Fatalf("buildInternalServer(no db path) = %#v, want nil", got)
	}
}

type syncStoreStub struct {
	list   []*core.AgentProfile
	upsert []string
	delete []string

	listErr   error
	upsertErr error
	deleteErr error
}

func (s *syncStoreStub) ListProfiles(context.Context) ([]*core.AgentProfile, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.list, nil
}

func (s *syncStoreStub) UpsertProfile(_ context.Context, p *core.AgentProfile) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.upsert = append(s.upsert, p.ID)
	return nil
}

func (s *syncStoreStub) DeleteProfile(_ context.Context, id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.delete = append(s.delete, id)
	return nil
}

func TestSyncRegistry(t *testing.T) {
	if err := SyncRegistry(context.Background(), nil, nil); err != nil {
		t.Fatalf("SyncRegistry(nil,nil) error = %v", err)
	}

	store := &syncStoreStub{
		list: []*core.AgentProfile{
			{ID: "keep"},
			{ID: "stale"},
		},
	}
	snap := &Snapshot{
		Profiles: []*core.AgentProfile{
			{ID: "keep"},
			{ID: "new"},
		},
	}
	if err := SyncRegistry(context.Background(), store, snap); err != nil {
		t.Fatalf("SyncRegistry() error = %v", err)
	}
	if len(store.upsert) != 2 || len(store.delete) != 1 || store.delete[0] != "stale" {
		t.Fatalf("SyncRegistry operations: upsert=%v delete=%v", store.upsert, store.delete)
	}

	if err := SyncRegistry(context.Background(), &syncStoreStub{listErr: errors.New("list failed")}, snap); err == nil {
		t.Fatalf("SyncRegistry(list error) should fail")
	}
	if err := SyncRegistry(context.Background(), &syncStoreStub{list: []*core.AgentProfile{}, upsertErr: errors.New("upsert failed")}, snap); err == nil {
		t.Fatalf("SyncRegistry(upsert error) should fail")
	}
	if err := SyncRegistry(context.Background(), &syncStoreStub{list: []*core.AgentProfile{{ID: "stale"}}, deleteErr: errors.New("delete failed")}, &Snapshot{}); err == nil {
		t.Fatalf("SyncRegistry(delete error) should fail")
	}
}

func TestReadRawStringAndBuildSnapshotErrors(t *testing.T) {
	manager := &Manager{
		configPath: filepath.Join(t.TempDir(), "missing.toml"),
		logger:     slog.Default(),
	}
	if _, err := manager.ReadRaw(); err == nil {
		t.Fatalf("ReadRaw(missing file) should fail")
	}
	if _, err := manager.ReadRawString(); err == nil {
		t.Fatalf("ReadRawString(missing file) should fail")
	}

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	secretsPath := filepath.Join(t.TempDir(), "secrets.toml")
	if err := os.WriteFile(cfgPath, []byte("runtime = ["), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(secretsPath, []byte(""), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	manager.configPath = cfgPath
	manager.secretsPath = secretsPath
	if _, err := manager.buildSnapshotFromPaths(cfgPath, secretsPath); err == nil {
		t.Fatalf("buildSnapshotFromPaths(invalid config) should fail")
	}
}

func TestReadLayerAndCloneHelpers(t *testing.T) {
	manager, _ := newManagerForTest(t)
	if err := os.WriteFile(manager.configPath, []byte("runtime = ["), 0o600); err != nil {
		t.Fatalf("write invalid raw: %v", err)
	}
	if _, err := manager.readLayer(); err == nil {
		t.Fatalf("readLayer(invalid raw) should fail")
	}

	if cloneRuntimeDrivers(nil) != nil {
		t.Fatalf("cloneRuntimeDrivers(nil) should return nil")
	}
	if cloneRuntimeProfiles(nil) != nil {
		t.Fatalf("cloneRuntimeProfiles(nil) should return nil")
	}
	drivers := []config.RuntimeDriverConfig{{ID: "a"}}
	driverPtr := cloneRuntimeDrivers(drivers)
	(*driverPtr)[0].ID = "b"
	if drivers[0].ID != "a" {
		t.Fatalf("cloneRuntimeDrivers should copy slice")
	}
	profiles := []config.RuntimeProfileConfig{{ID: "p1"}}
	profilePtr := cloneRuntimeProfiles(profiles)
	(*profilePtr)[0].ID = "p2"
	if profiles[0].ID != "p1" {
		t.Fatalf("cloneRuntimeProfiles should copy slice")
	}
}

var _ = acpproto.McpServer{}
