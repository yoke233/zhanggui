# acpclient

ACP (Agent Communication Protocol) client library. Handles the full agent lifecycle: process launch, protocol negotiation, session management, and message exchange over JSON-RPC stdio transport.

## Package Structure

| File | Description |
|---|---|
| `client.go` | `Client` — process launch, Initialize, Prompt, Session CRUD, Close |
| `bootstrap.go` | `Bootstrap` — unified entry point: profile → launch → init → session |
| `protocol.go` | Data types: `LaunchConfig`, `ClientCapabilities`, `SessionResult`, `SessionUpdate` |
| `handler.go` | `EventHandler` interface, `NopHandler` default implementation |
| `transport.go` | `Transport` — bidirectional JSON-RPC 2.0 over stdio |
| `trace.go` | `TraceRecorder` — optional protocol tracing |
| `role_resolver.go` | `RoleResolver` — agent/role profile lookup and capability validation |

## Usage

### Quick Start — Bootstrap

`Bootstrap` is the recommended way to launch an ACP agent. It handles everything from profile to ready client+session in one call:

```go
result, err := acpclient.Bootstrap(ctx, acpclient.BootstrapConfig{
    Profile:      profile,       // *core.AgentProfile (driver command, capabilities)
    WorkDir:      "/workspace",
    ExtraEnv:     map[string]string{"AI_WORKFLOW_API_TOKEN": token},
    Handler:      handler,       // acpproto.Client implementation (e.g. acphandler.NewACPHandler)
    EventHandler: eventBridge,   // receives session update events
    Session:      &acpclient.BootstrapSessionConfig{},  // create a new session
})
if err != nil {
    return err
}
defer result.Client.Close(ctx)

// Send a prompt
reply, err := result.Client.PromptText(ctx, result.Session.ID, "Hello")
```

### With Pre-built LaunchConfig (pre-sandboxed)

When the launch config is already composed upstream (e.g. by SessionManager with sandbox):

```go
result, err := acpclient.Bootstrap(ctx, acpclient.BootstrapConfig{
    Profile:        profile,
    WorkDir:        workDir,
    LaunchOverride: &sandboxedLaunch,  // skip profile-based build
    Handler:        handler,
    EventHandler:   switcher,
    Session: &acpclient.BootstrapSessionConfig{
        PriorSessionID: priorID,       // try to resume
        MCPFactory:     mcpFactory,    // lazy MCP server resolution
    },
})
```

### With Session Resumption

```go
Session: &acpclient.BootstrapSessionConfig{
    PriorSessionID: "session-abc-123",  // attempts LoadSession first
    MCPServers:     servers,            // fallback creates new session with these
}
```

If `PriorSessionID` is set, `LoadSession` is attempted first. On failure, falls back to `NewSession`. Check `result.Session.Loaded` to see which path was taken.

### PrepareLaunch Only

When you need the launch config but create the client yourself (e.g. with a custom factory for testing):

```go
launchCfg, err := acpclient.PrepareLaunch(ctx, acpclient.BootstrapConfig{
    Profile: profile,
    WorkDir: workDir,
})
// optionally apply sandbox here:
// launchCfg, err = sandbox.Prepare(ctx, sandbox.PrepareInput{Launch: launchCfg, ...})

client, err := customFactory(launchCfg, handler, opts...)
client.Initialize(ctx, acpclient.InitCapabilities(profile))
```

### Low-level Client

Direct client creation without Bootstrap:

```go
client, err := acpclient.New(launchCfg, handler,
    acpclient.WithEventHandler(eventHandler),
    acpclient.WithTraceRecorder(recorder),
)
if err != nil { ... }
defer client.Close(ctx)

err = client.Initialize(ctx, acpclient.ClientCapabilities{
    FSRead: true, FSWrite: true, Terminal: true,
})

sessionID, err := client.NewSession(ctx, acpproto.NewSessionRequest{
    Cwd: "/workspace",
})

// Simple text prompt (convenience helper)
result, err := client.PromptText(ctx, sessionID, "Hello")

// Multi-block prompt (e.g. with attachments)
result, err := client.Prompt(ctx, acpproto.PromptRequest{
    SessionId: sessionID,
    Prompt:    promptBlocks,
})
```

## Key Types

### BootstrapConfig

| Field | Type | Description |
|---|---|---|
| `Profile` | `*core.AgentProfile` | Required. Driver command, capabilities |
| `WorkDir` | `string` | Required. Agent working directory |
| `ExtraEnv` | `map[string]string` | Context-specific env vars (e.g. API tokens) |
| `LaunchOverride` | `*LaunchConfig` | Pre-built launch config, skips profile-based build |
| `Handler` | `acpproto.Client` | Protocol handler (nil = NopHandler) |
| `EventHandler` | `EventHandler` | Session event receiver (nil = none) |
| `ClientOpts` | `[]Option` | Trace recorder, close hooks, etc. |
| `Session` | `*BootstrapSessionConfig` | Session config (nil = skip session creation) |
| `InitTimeout` | `time.Duration` | Initialize timeout (default 30s) |

### Helper Functions

| Function | Description |
|---|---|
| `Bootstrap(ctx, cfg)` | Full lifecycle: profile → client → init → session |
| `PrepareLaunch(ctx, cfg)` | Build LaunchConfig from profile + env merge |
| `InitCapabilities(profile)` | Convert profile to ClientCapabilities |
| `CreateOrLoadSession(ctx, client, workDir, cfg)` | Session creation with load-or-new fallback |
| `CloneEnv(env)` | Shallow copy of env map (never nil) |
| `client.PromptText(ctx, sid, text)` | Send a single text prompt (convenience wrapper) |

## Architecture

```
Caller
  │
  ├─ Bootstrap()           ← recommended entry point
  │   ├─ PrepareLaunch()   ← LaunchConfig from profile + env
  │   ├─ New()             ← start process, set up transport
  │   ├─ Initialize()      ← ACP protocol handshake
  │   └─ NewSession/Load   ← optional session creation
  │
  └─ Client
      ├─ Transport         ← JSON-RPC 2.0 over stdio
      │   ├─ Call()        ← send request, wait for response
      │   └─ Notify()      ← fire-and-forget notification
      ├─ EventHandler      ← session update events
      └─ TraceRecorder     ← optional protocol tracing
```
