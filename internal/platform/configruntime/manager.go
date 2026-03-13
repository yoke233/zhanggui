package configruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/fsnotify/fsnotify"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

var ErrInvalidConfig = errors.New("invalid config")

type Snapshot struct {
	Version              int64
	LoadedAt             time.Time
	Config               *config.Config
	Drivers              []*core.AgentDriver
	Profiles             []*core.AgentProfile
	MCPServersByID       map[string]MCPServer
	MCPBindingsByProfile map[string][]MCPProfileBinding
}

type MCPServer struct {
	ID        string
	Name      string
	Kind      string
	Transport string
	Endpoint  string
	Command   string
	Args      []string
	Env       map[string]string
	Headers   []acpproto.HttpHeader
	Enabled   bool
}

type MCPProfileBinding struct {
	ProfileID string
	ServerID  string
	Enabled   bool
	ToolMode  string
	Tools     []string
}

type ReloadStatus struct {
	ActiveVersion int64     `json:"active_version"`
	LastSuccessAt time.Time `json:"last_success_at"`
	LastError     string    `json:"last_error"`
	LastErrorAt   time.Time `json:"last_error_at"`
}

type ValidationError struct {
	Err error
}

func (e *ValidationError) Error() string {
	if e == nil || e.Err == nil {
		return ErrInvalidConfig.Error()
	}
	return e.Err.Error()
}

func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type RuntimeConfig struct {
	Sandbox config.RuntimeSandboxConfig `json:"sandbox"`
	Agents  config.RuntimeAgentsConfig  `json:"agents"`
	MCP     config.RuntimeMCPConfig     `json:"mcp"`
	Prompts config.RuntimePromptsConfig `json:"prompts"`
}

type MCPEnvConfig struct {
	DBPath     string
	DevMode    bool
	SourceRoot string
	ServerAddr string
	AuthToken  string
}

type Manager struct {
	configPath  string
	secretsPath string
	mcpEnv      MCPEnvConfig
	logger      *slog.Logger
	onReload    func(context.Context, *Snapshot) error

	nextVersion atomic.Int64
	current     atomic.Pointer[Snapshot]

	statusMu sync.RWMutex
	status   ReloadStatus

	reloadMu sync.Mutex
	watcher  *fsnotify.Watcher
}

func DisabledMCPEnv() MCPEnvConfig {
	return MCPEnvConfig{}
}

func NewManager(configPath string, secretsPath string, mcpEnv MCPEnvConfig, logger *slog.Logger, onReload func(context.Context, *Snapshot) error) (*Manager, error) {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{
		configPath:  configPath,
		secretsPath: secretsPath,
		mcpEnv:      mcpEnv,
		logger:      logger,
		onReload:    onReload,
	}
	if _, err := m.Reload(context.Background(), "startup"); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Current() *Snapshot {
	return m.current.Load()
}

func (m *Manager) Status() ReloadStatus {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	return m.status
}

func (m *Manager) ReadRaw() ([]byte, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("read config runtime raw: %w", err)
	}
	return data, nil
}

func (m *Manager) ReadRawString() (string, error) {
	data, err := m.ReadRaw()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *Manager) GetRuntime() RuntimeConfig {
	snap := m.Current()
	if snap == nil || snap.Config == nil {
		return RuntimeConfig{}
	}
	return RuntimeConfig{
		Sandbox: snap.Config.Runtime.Sandbox,
		Agents:  snap.Config.Runtime.Agents,
		MCP:     snap.Config.Runtime.MCP,
		Prompts: snap.Config.Runtime.Prompts,
	}
}

func (m *Manager) CurrentConfig() (config.RuntimeAgentsConfig, config.RuntimeMCPConfig, bool) {
	current := m.GetRuntime()
	snap := m.Current()
	return current.Agents, current.MCP, snap != nil && snap.Config != nil
}

func (m *Manager) Reload(ctx context.Context, reason string) (*Snapshot, error) {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()
	return m.reloadLocked(ctx, reason)
}

func (m *Manager) WriteRaw(ctx context.Context, raw string) (*Snapshot, error) {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	content := normalizeRaw(raw)
	if len(bytes.TrimSpace(content)) == 0 {
		err := &ValidationError{Err: errors.New("config.toml content is empty")}
		m.setError(err)
		return nil, err
	}
	if err := m.validateRaw(content); err != nil {
		m.setError(err)
		return nil, err
	}

	previous, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("read current config before write: %w", err)
	}
	if err := writeFileKeepingMode(m.configPath, content); err != nil {
		return nil, fmt.Errorf("write config runtime raw: %w", err)
	}

	snap, err := m.reloadLocked(ctx, "api")
	if err == nil {
		return snap, nil
	}

	if restoreErr := writeFileKeepingMode(m.configPath, previous); restoreErr != nil {
		return nil, fmt.Errorf("reload config runtime failed: %w (rollback write failed: %v)", err, restoreErr)
	}
	if _, rollbackErr := m.reloadLocked(context.Background(), "rollback"); rollbackErr != nil {
		return nil, fmt.Errorf("reload config runtime failed: %w (rollback reload failed: %v)", err, rollbackErr)
	}
	return nil, err
}

func (m *Manager) UpdateRuntime(ctx context.Context, next RuntimeConfig) (*Snapshot, error) {
	layer, err := m.readLayer()
	if err != nil {
		return nil, err
	}
	if layer.Runtime == nil {
		layer.Runtime = &config.RuntimeLayer{}
	}
	layer.Runtime.Sandbox = buildRuntimeSandboxLayer(next.Sandbox)
	layer.Runtime.Agents = &config.RuntimeAgentsLayerCfg{
		Drivers:  cloneRuntimeDrivers(next.Agents.Drivers),
		Profiles: cloneRuntimeProfiles(next.Agents.Profiles),
	}
	layer.Runtime.MCP = &config.RuntimeMCPLayer{
		Servers:         cloneRuntimeMCPServers(next.MCP.Servers),
		ProfileBindings: cloneRuntimeMCPBindings(next.MCP.ProfileBindings),
	}
	layer.Runtime.Prompts = &config.RuntimePromptsLayer{
		ReworkFollowup:        stringPtr(next.Prompts.ReworkFollowup),
		ContinueFollowup:      stringPtr(next.Prompts.ContinueFollowup),
		PRImplementObjective:  stringPtr(next.Prompts.PRImplementObjective),
		PRGateObjective:       stringPtr(next.Prompts.PRGateObjective),
		PRMergeReworkFeedback: stringPtr(next.Prompts.PRMergeReworkFeedback),
		PRProviders: &config.RuntimePRPromptProvidersLayer{
			GitHub: buildPRProviderPromptLayer(next.Prompts.PRProviders.GitHub),
			CodeUp: buildPRProviderPromptLayer(next.Prompts.PRProviders.CodeUp),
			GitLab: buildPRProviderPromptLayer(next.Prompts.PRProviders.GitLab),
		},
	}

	raw, err := toml.Marshal(layer)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime runtime config: %w", err)
	}
	return m.WriteRaw(ctx, string(raw))
}

func buildRuntimeSandboxLayer(in config.RuntimeSandboxConfig) *config.RuntimeSandboxLayer {
	return &config.RuntimeSandboxLayer{
		Enabled:  boolPtr(in.Enabled),
		Provider: stringPtr(in.Provider),
		LiteBox: &config.RuntimeLiteBoxLayer{
			BridgeCommand: stringPtr(in.LiteBox.BridgeCommand),
			BridgeArgs:    cloneStringSlicePtr(in.LiteBox.BridgeArgs),
			RunnerPath:    stringPtr(in.LiteBox.RunnerPath),
			RunnerArgs:    cloneStringSlicePtr(in.LiteBox.RunnerArgs),
		},
		BoxLite: &config.RuntimeBoxLiteLayer{
			Command: stringPtr(in.BoxLite.Command),
			Image:   stringPtr(in.BoxLite.Image),
			RunArgs: cloneStringSlicePtr(in.BoxLite.RunArgs),
			CPUs:    stringPtr(in.BoxLite.CPUs),
			Memory:  stringPtr(in.BoxLite.Memory),
			Network: stringPtr(in.BoxLite.Network),
		},
		Docker: &config.RuntimeDockerLayer{
			Command:        stringPtr(in.Docker.Command),
			Image:          stringPtr(in.Docker.Image),
			RunArgs:        cloneStringSlicePtr(in.Docker.RunArgs),
			CPUs:           stringPtr(in.Docker.CPUs),
			Memory:         stringPtr(in.Docker.Memory),
			MemorySwap:     stringPtr(in.Docker.MemorySwap),
			PidsLimit:      stringPtr(in.Docker.PidsLimit),
			Network:        stringPtr(in.Docker.Network),
			ReadOnlyRootFS: boolPtr(in.Docker.ReadOnlyRootFS),
			Tmpfs:          cloneStringSlicePtr(in.Docker.Tmpfs),
		},
	}
}

func buildPRProviderPromptLayer(in config.RuntimePRProviderPromptConfig) *config.RuntimePRProviderPromptLayer {
	return &config.RuntimePRProviderPromptLayer{
		ImplementObjective:  stringPtr(in.ImplementObjective),
		GateObjective:       stringPtr(in.GateObjective),
		MergeReworkFeedback: stringPtr(in.MergeReworkFeedback),
		MergeStates: &config.RuntimePRMergeStatePromptLayer{
			Default:  stringPtr(in.MergeStates.Default),
			Dirty:    stringPtr(in.MergeStates.Dirty),
			Blocked:  stringPtr(in.MergeStates.Blocked),
			Behind:   stringPtr(in.MergeStates.Behind),
			Unstable: stringPtr(in.MergeStates.Unstable),
			Draft:    stringPtr(in.MergeStates.Draft),
		},
	}
}

func (m *Manager) UpdateConfig(ctx context.Context, agents config.RuntimeAgentsConfig, mcp config.RuntimeMCPConfig) (*Snapshot, error) {
	current := m.GetRuntime()
	current.Agents = agents
	current.MCP = mcp
	return m.UpdateRuntime(ctx, current)
}

func (m *Manager) Start(ctx context.Context) error {
	dir := filepath.Dir(m.configPath)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create config watcher: %w", err)
	}
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return fmt.Errorf("watch config dir %s: %w", dir, err)
	}
	m.watcher = watcher

	go m.watchLoop(ctx, dir)
	return nil
}

func (m *Manager) Close() error {
	if m.watcher != nil {
		return m.watcher.Close()
	}
	return nil
}

func (m *Manager) ResolveMCPServers(profileID string, agentSupportsSSE bool) []acpproto.McpServer {
	snap := m.Current()
	if snap == nil {
		m.logger.Warn("mcp: resolve servers — no snapshot available", "profile", profileID)
		return nil
	}
	bindings := snap.MCPBindingsByProfile[strings.TrimSpace(profileID)]
	if len(bindings) == 0 {
		m.logger.Info("mcp: no bindings for profile", "profile", profileID,
			"all_profiles", mapKeys(snap.MCPBindingsByProfile))
		return nil
	}

	m.logger.Info("mcp: resolving servers", "profile", profileID,
		"bindings_count", len(bindings), "db_path", m.mcpEnv.DBPath)

	var out []acpproto.McpServer
	for _, binding := range bindings {
		if !binding.Enabled {
			m.logger.Info("mcp: binding disabled", "profile", profileID, "server", binding.ServerID)
			continue
		}
		server, ok := snap.MCPServersByID[binding.ServerID]
		if !ok || !server.Enabled {
			m.logger.Warn("mcp: server not found or disabled", "server_id", binding.ServerID)
			continue
		}
		if strings.EqualFold(server.Kind, "internal") {
			servers := buildInternalServer(server, m.mcpEnv, agentSupportsSSE)
			m.logger.Info("mcp: built internal server", "profile", profileID,
				"server_id", binding.ServerID, "result_count", len(servers))
			out = append(out, servers...)
			continue
		}
		switch strings.ToLower(strings.TrimSpace(server.Transport)) {
		case "sse":
			out = append(out, acpproto.McpServer{
				Sse: &acpproto.McpServerSseInline{
					Name:    server.Name,
					Type:    "sse",
					Url:     server.Endpoint,
					Headers: server.Headers,
				},
			})
		case "stdio":
			env := make([]acpproto.EnvVariable, 0, len(server.Env))
			for k, v := range server.Env {
				env = append(env, acpproto.EnvVariable{Name: k, Value: v})
			}
			out = append(out, acpproto.McpServer{
				Stdio: &acpproto.McpServerStdio{
					Name:    server.Name,
					Command: server.Command,
					Args:    append([]string(nil), server.Args...),
					Env:     env,
				},
			})
		}
	}
	return out
}

func (m *Manager) reloadLocked(ctx context.Context, reason string) (*Snapshot, error) {
	snap, err := m.buildSnapshotFromPaths(m.configPath, m.secretsPath)
	if err != nil {
		m.setError(err)
		return nil, err
	}
	if m.onReload != nil {
		if err := m.onReload(ctx, snap); err != nil {
			m.setError(err)
			return nil, err
		}
	}
	m.current.Store(snap)
	m.setSuccess(snap)
	if m.logger != nil {
		m.logger.Info("config runtime reloaded", "version", snap.Version, "reason", reason)
	}
	return snap, nil
}

func (m *Manager) buildSnapshotFromPaths(configPath string, secretsPath string) (*Snapshot, error) {
	cfg, err := config.LoadGlobal(configPath, secretsPath)
	if err != nil {
		return nil, fmt.Errorf("load config runtime: %w", err)
	}
	secrets, err := config.LoadSecrets(secretsPath)
	if err != nil {
		return nil, fmt.Errorf("load secrets runtime: %w", err)
	}
	drivers, profiles := BuildAgents(cfg)
	servers, err := buildMCPServers(cfg, secrets, profiles)
	if err != nil {
		return nil, err
	}
	bindings := buildBindings(cfg, profiles)

	return &Snapshot{
		Version:              m.nextVersion.Add(1),
		LoadedAt:             time.Now().UTC(),
		Config:               cfg,
		Drivers:              drivers,
		Profiles:             profiles,
		MCPServersByID:       servers,
		MCPBindingsByProfile: bindings,
	}, nil
}

func (m *Manager) validateRaw(raw []byte) error {
	layer := &config.ConfigLayer{}
	decoder := toml.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(layer); err != nil {
		return &ValidationError{Err: fmt.Errorf("parse config.toml: %w", err)}
	}

	tmp, err := os.CreateTemp(filepath.Dir(m.configPath), "config-runtime-*.toml")
	if err != nil {
		return fmt.Errorf("create config runtime validation file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write config runtime validation file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close config runtime validation file: %w", err)
	}
	if _, err := config.LoadGlobal(tmpPath, m.secretsPath); err != nil {
		return &ValidationError{Err: fmt.Errorf("%w: %v", ErrInvalidConfig, err)}
	}
	return nil
}

func (m *Manager) setSuccess(snap *Snapshot) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	m.status.ActiveVersion = snap.Version
	m.status.LastSuccessAt = snap.LoadedAt
	m.status.LastError = ""
}

func (m *Manager) setError(err error) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	m.status.LastError = err.Error()
	m.status.LastErrorAt = time.Now().UTC()
	if m.logger != nil {
		m.logger.Warn("config runtime reload failed", "error", err)
	}
}

func (m *Manager) readLayer() (*config.ConfigLayer, error) {
	raw, err := m.ReadRaw()
	if err != nil {
		return nil, err
	}
	layer := &config.ConfigLayer{}
	decoder := toml.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(layer); err != nil {
		return nil, fmt.Errorf("decode config layer: %w", err)
	}
	return layer, nil
}

func cloneRuntimeDrivers(items []config.RuntimeDriverConfig) *[]config.RuntimeDriverConfig {
	if items == nil {
		return nil
	}
	out := append([]config.RuntimeDriverConfig(nil), items...)
	return &out
}

func cloneRuntimeProfiles(items []config.RuntimeProfileConfig) *[]config.RuntimeProfileConfig {
	if items == nil {
		return nil
	}
	out := append([]config.RuntimeProfileConfig(nil), items...)
	return &out
}

func cloneRuntimeMCPServers(items []config.RuntimeMCPServerConfig) *[]config.RuntimeMCPServerConfig {
	if items == nil {
		return nil
	}
	out := append([]config.RuntimeMCPServerConfig(nil), items...)
	return &out
}

func cloneRuntimeMCPBindings(items []config.RuntimeMCPProfileBindingConfig) *[]config.RuntimeMCPProfileBindingConfig {
	if items == nil {
		return nil
	}
	out := append([]config.RuntimeMCPProfileBindingConfig(nil), items...)
	return &out
}

func stringPtr(v string) *string {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func cloneStringSlicePtr(items []string) *[]string {
	if items == nil {
		return nil
	}
	out := append([]string(nil), items...)
	return &out
}

func StructToTomlMap(v any) (map[string]any, error) {
	data, err := toml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal toml value: %w", err)
	}
	out := map[string]any{}
	if err := toml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal toml map: %w", err)
	}
	return out, nil
}

func TomlMapToStruct(raw map[string]any, out any) error {
	data, err := toml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal toml map: %w", err)
	}
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode toml map: %w", err)
	}
	return nil
}

func normalizeRaw(raw string) []byte {
	content := []byte(strings.ReplaceAll(raw, "\r\n", "\n"))
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	return content
}

func writeFileKeepingMode(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, data, mode)
}

func (m *Manager) watchLoop(ctx context.Context, dir string) {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}

	schedule := func() {
		timer.Reset(500 * time.Millisecond)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			base := filepath.Base(evt.Name)
			if base != filepath.Base(m.configPath) && base != filepath.Base(m.secretsPath) {
				continue
			}
			if evt.Has(fsnotify.Remove) || evt.Has(fsnotify.Rename) {
				_ = m.watcher.Remove(dir)
				_ = m.watcher.Add(dir)
			}
			schedule()
		case err, ok := <-m.watcher.Errors:
			if ok && m.logger != nil {
				m.logger.Warn("config watcher error", "error", err)
			}
		case <-timer.C:
			if _, err := m.Reload(context.Background(), "fsnotify"); err != nil && m.logger != nil {
				m.logger.Warn("config watcher reload skipped", "error", err)
			}
		}
	}
}

const defaultInternalMCPServerID = "ai-workflow-query"
const legacyMCPEndpointPath = "/api/v1/mcp"

func buildMCPServers(cfg *config.Config, secrets *config.Secrets, profiles []*core.AgentProfile) (map[string]MCPServer, error) {
	out := make(map[string]MCPServer, len(cfg.Runtime.MCP.Servers))
	for _, server := range cfg.Runtime.MCP.Servers {
		headers := []acpproto.HttpHeader{}
		if ref := strings.TrimSpace(server.AuthSecretRef); ref != "" {
			token, err := resolveSecretRef(secrets, ref)
			if err != nil {
				return nil, fmt.Errorf("resolve auth_secret_ref for server %q: %w", server.ID, err)
			}
			if token != "" {
				headers = append(headers, acpproto.HttpHeader{Name: "Authorization", Value: "Bearer " + token})
			}
		}
		name := strings.TrimSpace(server.Name)
		if name == "" {
			name = strings.TrimSpace(server.ID)
		}
		out[strings.TrimSpace(server.ID)] = MCPServer{
			ID:        strings.TrimSpace(server.ID),
			Name:      name,
			Kind:      strings.TrimSpace(server.Kind),
			Transport: strings.TrimSpace(server.Transport),
			Endpoint:  strings.TrimSpace(server.Endpoint),
			Command:   strings.TrimSpace(server.Command),
			Args:      append([]string(nil), server.Args...),
			Env:       cloneStringMap(server.Env),
			Headers:   headers,
			Enabled:   server.Enabled,
		}
	}
	if len(out) == 0 && hasLegacyProfileMCP(profiles) {
		out[defaultInternalMCPServerID] = MCPServer{
			ID:        defaultInternalMCPServerID,
			Name:      defaultInternalMCPServerID,
			Kind:      "internal",
			Transport: "sse",
			Enabled:   true,
		}
	}
	return out, nil
}

func buildBindings(cfg *config.Config, profiles []*core.AgentProfile) map[string][]MCPProfileBinding {
	out := make(map[string][]MCPProfileBinding, len(cfg.Runtime.MCP.ProfileBindings))
	if len(cfg.Runtime.MCP.ProfileBindings) == 0 {
		for _, profile := range profiles {
			if profile == nil || !profile.MCP.Enabled {
				continue
			}
			mode := "all"
			if len(profile.MCP.Tools) > 0 {
				mode = "allow_list"
			}
			out[profile.ID] = append(out[profile.ID], MCPProfileBinding{
				ProfileID: profile.ID,
				ServerID:  defaultInternalMCPServerID,
				Enabled:   true,
				ToolMode:  mode,
				Tools:     append([]string(nil), profile.MCP.Tools...),
			})
		}
		return out
	}
	for _, binding := range cfg.Runtime.MCP.ProfileBindings {
		item := MCPProfileBinding{
			ProfileID: strings.TrimSpace(binding.Profile),
			ServerID:  strings.TrimSpace(binding.Server),
			Enabled:   binding.Enabled,
			ToolMode:  strings.TrimSpace(binding.ToolMode),
			Tools:     append([]string(nil), binding.Tools...),
		}
		out[item.ProfileID] = append(out[item.ProfileID], item)
	}
	return out
}

func hasLegacyProfileMCP(profiles []*core.AgentProfile) bool {
	for _, profile := range profiles {
		if profile != nil && profile.MCP.Enabled {
			return true
		}
	}
	return false
}

func resolveSecretRef(secrets *config.Secrets, ref string) (string, error) {
	if secrets == nil {
		return "", fmt.Errorf("secrets unavailable")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	if !strings.HasPrefix(ref, "tokens.") {
		return "", fmt.Errorf("unsupported secret ref %q", ref)
	}
	name := strings.TrimPrefix(ref, "tokens.")
	entry, ok := secrets.Tokens[name]
	if !ok {
		return "", fmt.Errorf("unknown token %q", name)
	}
	return strings.TrimSpace(entry.Token), nil
}

func buildInternalServer(server MCPServer, env MCPEnvConfig, agentSupportsSSE bool) []acpproto.McpServer {
	name := strings.TrimSpace(server.Name)
	if name == "" {
		name = strings.TrimSpace(server.ID)
	}
	slog.Info("mcp: buildInternalServer", "name", name,
		"db_path", env.DBPath, "server_addr", env.ServerAddr,
		"agent_supports_sse", agentSupportsSSE)
	if addr := strings.TrimSpace(env.ServerAddr); addr != "" && agentSupportsSSE {
		url := strings.TrimRight(addr, "/") + legacyMCPEndpointPath
		headers := []acpproto.HttpHeader{}
		if tok := strings.TrimSpace(env.AuthToken); tok != "" {
			headers = append(headers, acpproto.HttpHeader{Name: "Authorization", Value: "Bearer " + tok})
		}
		return []acpproto.McpServer{{
			Sse: &acpproto.McpServerSseInline{
				Name:    name,
				Type:    "sse",
				Url:     url,
				Headers: headers,
			},
		}}
	}
	if strings.TrimSpace(env.DBPath) == "" {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	stdioEnv := []acpproto.EnvVariable{{Name: "AI_WORKFLOW_DB_PATH", Value: env.DBPath}}
	if env.DevMode {
		stdioEnv = append(stdioEnv,
			acpproto.EnvVariable{Name: "AI_WORKFLOW_DEV_MODE", Value: "true"},
			acpproto.EnvVariable{Name: "AI_WORKFLOW_SOURCE_ROOT", Value: env.SourceRoot},
			acpproto.EnvVariable{Name: "AI_WORKFLOW_SERVER_ADDR", Value: env.ServerAddr},
		)
	}
	return []acpproto.McpServer{{
		Stdio: &acpproto.McpServerStdio{
			Name:    name,
			Command: self,
			Args:    []string{"mcp-serve"},
			Env:     stdioEnv,
		},
	}}
}

func mapKeys[K comparable, V any](m map[K][]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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
