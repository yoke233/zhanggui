package runtimeapp

import (
	"context"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
	probeapp "github.com/yoke233/zhanggui/internal/application/probe"
	"github.com/yoke233/zhanggui/internal/core"
)

// SessionManager abstracts ACP agent session lifecycle management for runs.
//
// Two modes:
//   - Local (default): in-process, wraps ACPSessionPool. No external dependencies.
//   - NATS (opt-in): async run dispatch via NATS JetStream. Agents survive
//     server restarts. Supports multiple remote executors with queue-group load balancing.
type SessionManager interface {
	// Acquire gets or creates an ACP session for the given agent+work item.
	Acquire(ctx context.Context, in SessionAcquireInput) (*SessionHandle, error)

	// StartRun dispatches text to the acquired session. Returns an invocation ID.
	// In local mode this executes synchronously and the result is available immediately.
	// In NATS mode this publishes to JetStream and returns before run execution starts.
	StartRun(ctx context.Context, handle *SessionHandle, text string) (string, error)

	// WatchRun subscribes to events for a dispatched invocation. Blocks until the
	// run completes or ctx is cancelled. Events are forwarded to sink.
	// Can reconnect with lastEventSeq to resume from where we left off.
	WatchRun(ctx context.Context, invocationID string, lastEventSeq int64, sink EventSink) (*RunResult, error)

	// RecoverRuns returns the status of all runs that were active or completed
	// since the given timestamp. Called after server restart to resume tracking.
	RecoverRuns(ctx context.Context, since time.Time) ([]RunRuntimeStatus, error)

	// ProbeRun sends a side-channel diagnostic question to a running run.
	ProbeRun(ctx context.Context, req probeapp.RunProbeRuntimeRequest) (*probeapp.RunProbeRuntimeResult, error)

	// Release marks a session handle as no longer active.
	Release(ctx context.Context, handle *SessionHandle) error

	// CleanupWorkItem releases all sessions for a completed/failed work item.
	CleanupWorkItem(workItemID int64)

	// DrainActive blocks until all in-flight runs complete (for graceful upgrade).
	DrainActive(ctx context.Context) error

	// ActiveCount returns the number of invocations with runs in flight.
	ActiveCount() int

	// Close shuts down all managed sessions.
	Close()
}

// EventSink receives streaming events during a run.
type EventSink interface {
	HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error
}

// SessionAcquireInput contains everything needed to acquire an agent session.
type SessionAcquireInput struct {
	Profile *core.AgentProfile
	Launch  acpclient.LaunchConfig
	Caps    acpclient.ClientCapabilities
	WorkDir string

	// MCPFactory resolves MCP servers after connecting to the agent.
	// Local mode only; remote executors use their own MCP resolver.
	MCPFactory func(agentSupportsSSE bool) []acpproto.McpServer

	WorkItemID int64
	ActionID   int64
	RunID      int64

	Reuse    bool
	IdleTTL  time.Duration
	MaxTurns int

	// ExtraSkills are dynamically injected skill names (e.g. "action-signal")
	// that should be linked alongside Profile.Skills in the sandbox.
	ExtraSkills []string

	// EphemeralSkills maps skill names to pre-built directories on disk.
	// These are linked directly into the agent's skills dir, bypassing the
	// global skillsRoot. Used for per-run materials (e.g. action-context).
	EphemeralSkills map[string]string
}

// SessionHandle is an opaque reference to an acquired session.
type SessionHandle struct {
	ID             string // opaque handle identifier
	AgentContextID *int64 // persisted context ID (for run record)
	HasPriorTurns  bool   // whether session had prior runs
}

// RunResult contains the outcome of a run.
type RunResult struct {
	Text             string
	StopReason       string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	ModelID          string
	AgentContextID   *int64
}

// RunRuntimeStatus represents the state of a run invocation for recovery.
type RunRuntimeStatus struct {
	InvocationID string
	RunID        int64
	WorkItemID   int64
	ActionID     int64
	Status       RunRuntimeState
	Result       *RunResult
	Error        string
	CreatedAt    time.Time
}

// RunRuntimeState is the lifecycle state of a dispatched run invocation.
type RunRuntimeState string

const (
	RunPending RunRuntimeState = "pending"
	RunRunning RunRuntimeState = "running"
	RunDone    RunRuntimeState = "done"
	RunFailed  RunRuntimeState = "failed"
)

type RunProbeRuntimeRequest = probeapp.RunProbeRuntimeRequest

type RunProbeRuntimeResult = probeapp.RunProbeRuntimeResult
