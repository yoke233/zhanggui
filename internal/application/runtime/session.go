package runtimeapp

import (
	"context"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	"github.com/yoke233/ai-workflow/internal/core"
)

// SessionManager abstracts ACP agent session lifecycle management for executions.
//
// Two modes:
//   - Local (default): in-process, wraps ACPSessionPool. No external dependencies.
//   - NATS (opt-in): async execution dispatch via NATS JetStream. Agents survive
//     server restarts. Supports multiple remote executors with queue-group load balancing.
type SessionManager interface {
	// Acquire gets or creates an ACP session for the given agent+issue.
	Acquire(ctx context.Context, in SessionAcquireInput) (*SessionHandle, error)

	// StartExecution dispatches text to the acquired session. Returns an invocation ID.
	// In local mode this executes synchronously and the result is available immediately.
	// In NATS mode this publishes to JetStream and returns before execution starts.
	StartExecution(ctx context.Context, handle *SessionHandle, text string) (string, error)

	// WatchExecution subscribes to events for a dispatched invocation. Blocks until the
	// execution completes or ctx is cancelled. Events are forwarded to sink.
	// Can reconnect with lastEventSeq to resume from where we left off.
	WatchExecution(ctx context.Context, invocationID string, lastEventSeq int64, sink EventSink) (*ExecutionResult, error)

	// RecoverExecutions returns the status of all executions that were active or completed
	// since the given timestamp. Called after server restart to resume tracking.
	RecoverExecutions(ctx context.Context, since time.Time) ([]ExecutionRuntimeStatus, error)

	// ProbeRun sends a side-channel diagnostic question to a running run.
	ProbeRun(ctx context.Context, req probeapp.RunProbeRuntimeRequest) (*probeapp.RunProbeRuntimeResult, error)

	// Release marks a session handle as no longer active.
	Release(ctx context.Context, handle *SessionHandle) error

	// CleanupIssue releases all sessions for a completed/failed issue.
	CleanupIssue(issueID int64)

	// DrainActive blocks until all in-flight executions complete (for graceful upgrade).
	DrainActive(ctx context.Context) error

	// ActiveCount returns the number of currently executing invocations.
	ActiveCount() int

	// Close shuts down all managed sessions.
	Close()
}

// EventSink receives streaming events during execution.
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

	IssueID int64
	StepID  int64
	ExecID  int64

	Reuse    bool
	IdleTTL  time.Duration
	MaxTurns int

	// ExtraSkills are dynamically injected skill names (e.g. "step-signal")
	// that should be linked alongside Profile.Skills in the sandbox.
	ExtraSkills []string

	// EphemeralSkills maps skill names to pre-built directories on disk.
	// These are linked directly into the agent's skills dir, bypassing the
	// global skillsRoot. Used for per-execution materials (e.g. step-context).
	EphemeralSkills map[string]string
}

// SessionHandle is an opaque reference to an acquired session.
type SessionHandle struct {
	ID             string // opaque handle identifier
	AgentContextID *int64 // persisted context ID (for execution record)
	HasPriorTurns  bool   // whether session had prior executions
}

// ExecutionResult contains the outcome of an execution.
type ExecutionResult struct {
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

// ExecutionRuntimeStatus represents the state of an execution invocation for recovery.
type ExecutionRuntimeStatus struct {
	InvocationID string
	ExecID       int64
	IssueID      int64
	StepID       int64
	Status       ExecutionRuntimeState
	Result       *ExecutionResult
	Error        string
	CreatedAt    time.Time
}

// ExecutionRuntimeState is the lifecycle state of a dispatched execution invocation.
type ExecutionRuntimeState string

const (
	ExecutionPending ExecutionRuntimeState = "pending"
	ExecutionRunning ExecutionRuntimeState = "running"
	ExecutionDone    ExecutionRuntimeState = "done"
	ExecutionFailed  ExecutionRuntimeState = "failed"
)

type RunProbeRuntimeRequest = probeapp.RunProbeRuntimeRequest

type RunProbeRuntimeResult = probeapp.RunProbeRuntimeResult
