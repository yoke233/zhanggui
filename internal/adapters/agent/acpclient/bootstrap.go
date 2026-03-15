package acpclient

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/core"
)

// BootstrapConfig configures the unified ACP client bootstrap sequence.
//
// The bootstrap handles the full lifecycle from profile to ready client:
//  1. Build LaunchConfig from profile (command, args, workDir, env clone).
//  2. Merge context-specific environment variables.
//  3. Create ACP client with handler + event handler.
//  4. Initialize the ACP protocol (capabilities negotiation).
//  5. (Optional) Create or load a session.
//
// Sandbox wrapping is the caller's responsibility — apply it to the
// LaunchConfig before passing it as LaunchOverride, or to the result
// of PrepareLaunch before creating the client.
type BootstrapConfig struct {
	// Profile is the agent profile providing driver config and capabilities. Required.
	Profile *core.AgentProfile

	// WorkDir is the working directory for the agent process. Required.
	WorkDir string

	// ExtraEnv contains context-specific env vars merged on top of profile.Driver.Env.
	// Examples: AI_WORKFLOW_API_TOKEN, AI_WORKFLOW_SERVER_ADDR, etc.
	// Ignored when LaunchOverride is set.
	ExtraEnv map[string]string

	// LaunchOverride, if non-nil, replaces the LaunchConfig that would be
	// built from Profile.Driver + WorkDir + ExtraEnv. Use when the launch
	// config has already been composed upstream (e.g. pre-sandboxed by
	// SessionManager). Profile is still used for capabilities negotiation.
	LaunchOverride *LaunchConfig

	// Handler is the ACP protocol handler (acpproto.Client interface).
	// If nil, NopHandler is used.
	Handler acpproto.Client

	// EventHandler receives session update events.
	// If nil, no event handler is attached.
	EventHandler EventHandler

	// ClientOpts are additional Option values (trace recorder, close hook, etc.).
	ClientOpts []Option

	// Session controls whether and how to create a session.
	// If nil, no session is created and BootstrapResult.Session will be zero.
	Session *BootstrapSessionConfig

	// InitTimeout is the timeout for the Initialize call. Defaults to 30s.
	InitTimeout time.Duration
}

// BootstrapSessionConfig controls session creation or resumption.
type BootstrapSessionConfig struct {
	// PriorSessionID enables session resumption. If non-empty, LoadSession is
	// attempted first; on failure, falls back to NewSession.
	PriorSessionID string

	// MCPServers are attached to the created/loaded session.
	MCPServers []acpproto.McpServer

	// MCPFactory resolves MCP servers after initialization (receives SupportsSSEMCP result).
	// Takes precedence over MCPServers if non-nil.
	MCPFactory func(supportsSSE bool) []acpproto.McpServer
}

// BootstrapResult contains the bootstrapped ACP client and (optionally) session.
type BootstrapResult struct {
	// Client is the initialised ACP client. Caller owns closing it.
	Client *Client

	// Launch is the final LaunchConfig used.
	Launch LaunchConfig

	// Session is set when BootstrapConfig.Session is non-nil.
	Session BootstrapSessionOutput

	// SupportsSSEMCP reflects the agent's MCP capability after Initialize.
	SupportsSSEMCP bool
}

// BootstrapSessionOutput contains session creation results.
type BootstrapSessionOutput struct {
	ID            acpproto.SessionId
	ConfigOptions []acpproto.SessionConfigOptionSelect
	Modes         *acpproto.SessionModeState
	Loaded        bool // true if a prior session was successfully loaded
}

// Bootstrap launches an ACP agent process, initialises the protocol,
// and optionally creates or loads a session. It is the single entry
// point for all ACP agent lifecycle starts in this codebase.
func Bootstrap(ctx context.Context, cfg BootstrapConfig) (*BootstrapResult, error) {
	if cfg.Profile == nil {
		return nil, fmt.Errorf("acpclient: bootstrap: profile is required")
	}
	if strings.TrimSpace(cfg.WorkDir) == "" {
		return nil, fmt.Errorf("acpclient: bootstrap: work_dir is required")
	}

	// ── Steps 1-2: build launch config, merge env ──
	launch, err := PrepareLaunch(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// ── 3. Create ACP client ──
	handler := cfg.Handler
	if handler == nil {
		handler = &NopHandler{}
	}

	opts := make([]Option, 0, len(cfg.ClientOpts)+1)
	if cfg.EventHandler != nil {
		opts = append(opts, WithEventHandler(cfg.EventHandler))
	}
	opts = append(opts, cfg.ClientOpts...)

	slog.Info("acpclient: bootstrap: launching agent process",
		"profile", cfg.Profile.ID,
		"command", launch.Command,
		"args", launch.Args,
		"work_dir", launch.WorkDir)

	client, err := New(launch, handler, opts...)
	if err != nil {
		return nil, fmt.Errorf("acpclient: bootstrap: launch agent %q: %w", cfg.Profile.ID, err)
	}

	// ── 4. Initialize (capabilities negotiation) ──
	initTimeout := cfg.InitTimeout
	if initTimeout == 0 {
		initTimeout = 30 * time.Second
	}
	initCtx, initCancel := context.WithTimeout(ctx, initTimeout)
	defer initCancel()

	if err := client.Initialize(initCtx, InitCapabilities(cfg.Profile)); err != nil {
		_ = client.Close(context.Background())
		return nil, fmt.Errorf("acpclient: bootstrap: initialize agent %q: %w", cfg.Profile.ID, err)
	}

	slog.Info("acpclient: bootstrap: agent initialized",
		"profile", cfg.Profile.ID,
		"supports_sse_mcp", client.SupportsSSEMCP())

	result := &BootstrapResult{
		Client:         client,
		Launch:         launch,
		SupportsSSEMCP: client.SupportsSSEMCP(),
	}

	// ── 5. Create or load session ──
	if cfg.Session != nil {
		sessOut, err := createOrLoadSession(initCtx, client, cfg.WorkDir, cfg.Session)
		if err != nil {
			_ = client.Close(context.Background())
			return nil, err
		}
		result.Session = sessOut
	}

	return result, nil
}

// PrepareLaunch builds a LaunchConfig from profile and merges extra env.
// Use this when you need only the launch config (e.g. because the client
// is created by a pluggable factory, or you need to apply sandbox before
// passing the config to Bootstrap via LaunchOverride).
func PrepareLaunch(_ context.Context, cfg BootstrapConfig) (LaunchConfig, error) {
	if cfg.Profile == nil {
		return LaunchConfig{}, fmt.Errorf("acpclient: prepare launch: profile is required")
	}

	if cfg.LaunchOverride != nil {
		return *cfg.LaunchOverride, nil
	}

	launch := LaunchConfig{
		Command: cfg.Profile.Driver.LaunchCommand,
		Args:    cfg.Profile.Driver.LaunchArgs,
		WorkDir: cfg.WorkDir,
		Env:     CloneEnv(cfg.Profile.Driver.Env),
	}
	for k, v := range cfg.ExtraEnv {
		launch.Env[k] = v
	}

	return launch, nil
}

// InitCapabilities returns the ClientCapabilities derived from a profile.
func InitCapabilities(profile *core.AgentProfile) ClientCapabilities {
	caps := profile.EffectiveCapabilities()
	return ClientCapabilities{
		FSRead:   caps.FSRead,
		FSWrite:  caps.FSWrite,
		Terminal: caps.Terminal,
	}
}

// CreateOrLoadSession creates a new session or loads a prior one.
// Use this when you need session creation with a custom client factory
// (e.g. lead chat tests) rather than the full Bootstrap.
func CreateOrLoadSession(ctx context.Context, client *Client, workDir string, sc *BootstrapSessionConfig) (BootstrapSessionOutput, error) {
	return createOrLoadSession(ctx, client, workDir, sc)
}

func createOrLoadSession(ctx context.Context, client *Client, workDir string, sc *BootstrapSessionConfig) (BootstrapSessionOutput, error) {
	var mcpServers []acpproto.McpServer
	if sc.MCPFactory != nil {
		mcpServers = sc.MCPFactory(client.SupportsSSEMCP())
	} else {
		mcpServers = sc.MCPServers
	}

	if prior := strings.TrimSpace(sc.PriorSessionID); prior != "" {
		sr, err := client.LoadSessionResult(ctx, acpproto.LoadSessionRequest{
			SessionId:  acpproto.SessionId(prior),
			Cwd:        workDir,
			McpServers: mcpServers,
		})
		if err == nil && strings.TrimSpace(string(sr.SessionID)) != "" {
			slog.Info("acpclient: loaded prior session", "session_id", string(sr.SessionID))
			return BootstrapSessionOutput{
				ID:            sr.SessionID,
				ConfigOptions: sr.ConfigOptions,
				Modes:         sr.Modes,
				Loaded:        true,
			}, nil
		}
		if err != nil {
			slog.Warn("acpclient: load prior session failed, creating new", "error", err)
		}
	}

	sr, err := client.NewSessionResult(ctx, acpproto.NewSessionRequest{
		Cwd:        workDir,
		McpServers: mcpServers,
	})
	if err != nil {
		return BootstrapSessionOutput{}, fmt.Errorf("acpclient: create session: %w", err)
	}
	if strings.TrimSpace(string(sr.SessionID)) == "" {
		return BootstrapSessionOutput{}, fmt.Errorf("acpclient: session/new returned empty session id")
	}

	slog.Info("acpclient: new session created", "session_id", string(sr.SessionID))
	return BootstrapSessionOutput{
		ID:            sr.SessionID,
		ConfigOptions: sr.ConfigOptions,
		Modes:         sr.Modes,
	}, nil
}

// CloneEnv returns a shallow copy of the map, never nil.
func CloneEnv(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
