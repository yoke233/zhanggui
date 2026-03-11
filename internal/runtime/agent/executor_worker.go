package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	probeacp "github.com/yoke233/ai-workflow/internal/adapters/probe/acp"
	natsprobe "github.com/yoke233/ai-workflow/internal/adapters/probe/nats"
	v2sandbox "github.com/yoke233/ai-workflow/internal/adapters/sandbox"
	runtimeapp "github.com/yoke233/ai-workflow/internal/application/runtime"
	"github.com/yoke233/ai-workflow/internal/core"
)

// ExecutorWorkerConfig configures a remote executor worker.
type ExecutorWorkerConfig struct {
	// NATSConn is an already-connected NATS connection.
	NATSConn *nats.Conn

	// StreamPrefix is the JetStream stream name prefix (default: "aiworkflow").
	StreamPrefix string

	// WorkerID uniquely identifies this worker for probe routing.
	WorkerID string

	// AgentTypes are the agent driver IDs this worker can handle (e.g., ["claude", "codex"]).
	// If empty, the worker consumes from all agent types ("*").
	AgentTypes []string

	// Store is used for agent context persistence.
	Store core.Store

	// Registry resolves agent profiles and drivers.
	Registry core.AgentRegistry

	// Sandbox provides optional per-process isolation.
	Sandbox v2sandbox.Sandbox

	// MCPResolver resolves MCP servers for an agent profile.
	MCPResolver func(profileID string, agentSupportsSSE bool) []acpproto.McpServer

	// DefaultWorkDir is the fallback working directory.
	DefaultWorkDir string

	// MaxConcurrent limits parallel execution. Default: 2.
	MaxConcurrent int
}

type activeExecutionProbeTarget struct {
	ExecutionID    int64
	AgentContextID *int64
	SessionID      acpproto.SessionId
	Launch         acpclient.LaunchConfig
	Caps           acpclient.ClientCapabilities
	WorkDir        string
	MCPServers     []acpproto.McpServer
}

// ExecutorWorker consumes execution messages from NATS JetStream, executes them locally
// via ACP agents, and publishes results + events back to NATS.
type ExecutorWorker struct {
	cfg    ExecutorWorkerConfig
	js     jetstream.JetStream
	prefix string
	pool   *ACPSessionPool

	mu               sync.Mutex
	running          int
	cancel           context.CancelFunc
	activeExecutions map[int64]*activeExecutionProbeTarget
	probeSub         *nats.Subscription
}

// NewExecutorWorker creates a new remote executor worker.
func NewExecutorWorker(cfg ExecutorWorkerConfig) (*ExecutorWorker, error) {
	if cfg.NATSConn == nil {
		return nil, fmt.Errorf("NATS connection is required")
	}

	prefix := strings.TrimSpace(cfg.StreamPrefix)
	if prefix == "" {
		prefix = "aiworkflow"
	}

	js, err := jetstream.New(cfg.NATSConn)
	if err != nil {
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}

	workerID := strings.TrimSpace(cfg.WorkerID)
	if workerID == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "unknown"
		}
		workerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}
	cfg.WorkerID = workerID

	maxConc := cfg.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 2
	}
	cfg.MaxConcurrent = maxConc

	var pool *ACPSessionPool
	if cfg.Store != nil {
		pool = NewACPSessionPool(cfg.Store, nil)
	}

	return &ExecutorWorker{
		cfg:              cfg,
		js:               js,
		prefix:           prefix,
		pool:             pool,
		activeExecutions: make(map[int64]*activeExecutionProbeTarget),
	}, nil
}

// Start begins consuming execution messages. Blocks until ctx is cancelled.
func (w *ExecutorWorker) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	defer cancel()

	probeSubject := fmt.Sprintf("%s.probe.request.%s", w.prefix, w.cfg.WorkerID)
	probeSub, err := w.cfg.NATSConn.Subscribe(probeSubject, w.handleProbeRequest)
	if err != nil {
		return fmt.Errorf("subscribe probe subject: %w", err)
	}
	w.probeSub = probeSub
	defer probeSub.Unsubscribe()

	// Determine subjects to consume.
	subjects := w.buildSubjects()
	slog.Info("executor worker: starting",
		"worker_id", w.cfg.WorkerID,
		"subjects", subjects,
		"max_concurrent", w.cfg.MaxConcurrent)

	// Create durable consumer with queue group for load balancing.
	consumer, err := w.js.CreateOrUpdateConsumer(ctx, w.prefix+"_invocations", buildExecutorConsumerConfig(w.prefix, w.cfg.MaxConcurrent, subjects))
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}

	// Consume messages with concurrency control.
	sem := make(chan struct{}, w.cfg.MaxConcurrent)
	for {
		msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		for msg := range msgs.Messages() {
			sem <- struct{}{}
			go func(m jetstream.Msg) {
				defer func() { <-sem }()
				w.handleMessage(ctx, m)
			}(msg)
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (w *ExecutorWorker) buildSubjects() []string {
	if len(w.cfg.AgentTypes) == 0 {
		return []string{fmt.Sprintf("%s.invocation.submit.>", w.prefix)}
	}
	subjects := make([]string, 0, len(w.cfg.AgentTypes))
	for _, at := range w.cfg.AgentTypes {
		subjects = append(subjects, fmt.Sprintf("%s.invocation.submit.%s", w.prefix, at))
	}
	return subjects
}

func buildExecutorConsumerConfig(prefix string, maxConcurrent int, subjects []string) jetstream.ConsumerConfig {
	cfg := jetstream.ConsumerConfig{
		Durable:       prefix + "_executor",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxAckPending: maxConcurrent,
		AckWait:       10 * time.Minute, // allow up to 10 min for execution
	}
	if len(subjects) == 1 {
		cfg.FilterSubject = subjects[0]
	} else {
		cfg.FilterSubjects = append([]string(nil), subjects...)
	}
	return cfg
}

func (w *ExecutorWorker) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var invocation natsInvocationMessage
	if err := json.Unmarshal(msg.Data(), &invocation); err != nil {
		slog.Error("executor worker: invalid execution message", "error", err)
		_ = msg.Nak()
		return
	}

	slog.Info("executor worker: executing execution",
		"exec_id", invocation.ExecID, "agent", invocation.AgentID, "flow_id", invocation.FlowID)

	// Create event forwarder that publishes to NATS.
	eventSeq := int64(0)
	eventSubject := fmt.Sprintf("%s.invocation.events.%s", w.prefix, invocation.InvocationID)
	eventForwarder := &natsEventForwarder{
		js:      w.js,
		subject: eventSubject,
		seq:     &eventSeq,
		id:      invocation.InvocationID,
	}

	result, execErr := w.executeExecution(ctx, &invocation, eventForwarder)

	// Publish result.
	resultMsg := natsInvocationResult{
		InvocationID: invocation.InvocationID,
	}
	if execErr != nil {
		resultMsg.Error = execErr.Error()
	} else if result != nil {
		resultMsg.Text = result.Text
		resultMsg.StopReason = result.StopReason
		resultMsg.InputTokens = result.InputTokens
		resultMsg.OutputTokens = result.OutputTokens
		resultMsg.CacheReadTokens = result.CacheReadTokens
		resultMsg.CacheWriteTokens = result.CacheWriteTokens
		resultMsg.ReasoningTokens = result.ReasoningTokens
		resultMsg.ModelID = result.ModelID
		resultMsg.AgentContextID = result.AgentContextID
	}

	resultData, _ := json.Marshal(resultMsg)
	resultSubject := fmt.Sprintf("%s.invocation.result.%s", w.prefix, invocation.InvocationID)
	if _, err := w.js.Publish(ctx, resultSubject, resultData); err != nil {
		slog.Error("executor worker: failed to publish result",
			"exec_id", invocation.ExecID, "error", err)
	}

	_ = msg.Ack()

	if execErr != nil {
		slog.Error("executor worker: execution failed",
			"exec_id", invocation.ExecID, "error", execErr)
	} else {
		slog.Info("executor worker: execution completed",
			"exec_id", invocation.ExecID, "output_len", len(resultMsg.Text))
	}
}

func (w *ExecutorWorker) executeExecution(ctx context.Context, invocation *natsInvocationMessage, eventHandler acpclient.EventHandler) (*runtimeapp.ExecutionResult, error) {
	workDir := invocation.WorkDir
	if workDir == "" {
		workDir = w.cfg.DefaultWorkDir
	}

	profile, driver, err := resolveExecutionProfile(ctx, w.cfg.Registry, invocation)
	if err != nil {
		return nil, err
	}

	launchCfg := acpclient.LaunchConfig{
		Command: driver.LaunchCommand,
		Args:    driver.LaunchArgs,
		WorkDir: workDir,
		Env:     cloneEnv(driver.Env),
	}

	sb := w.cfg.Sandbox
	if sb == nil {
		sb = v2sandbox.NoopSandbox{}
	}
	sandboxedLaunch, err := sb.Prepare(ctx, v2sandbox.PrepareInput{
		Profile: profile,
		Driver:  driver,
		Launch:  launchCfg,
		Scope:   fmt.Sprintf("flow-%d-exec-%d", invocation.FlowID, invocation.ExecID),
	})
	if err != nil {
		return nil, fmt.Errorf("prepare sandbox: %w", err)
	}

	caps := profile.EffectiveCapabilities()
	acpCaps := acpclient.ClientCapabilities{
		FSRead:   caps.FSRead,
		FSWrite:  caps.FSWrite,
		Terminal: caps.Terminal,
	}

	acquireInput := acpSessionAcquireInput{
		Profile: profile,
		Driver:  driver,
		Launch:  sandboxedLaunch,
		Caps:    acpCaps,
		WorkDir: workDir,
		FlowID:  invocation.FlowID,
		StepID:  invocation.StepID,
		ExecID:  invocation.ExecID,
	}
	if w.cfg.MCPResolver != nil {
		acquireInput.MCPFactory = func(agentSupportsSSE bool) []acpproto.McpServer {
			return w.cfg.MCPResolver(profile.ID, agentSupportsSSE)
		}
	}

	if w.pool == nil {
		return nil, fmt.Errorf("execution pool is not configured")
	}

	session, agentCtx, err := w.pool.Acquire(ctx, acquireInput)
	if err != nil {
		return nil, err
	}

	var mcpServers []acpproto.McpServer
	if acquireInput.MCPFactory != nil && session.client != nil {
		mcpServers = acquireInput.MCPFactory(session.client.SupportsSSEMCP())
	}

	w.registerActiveExecution(invocation.ExecID, &activeExecutionProbeTarget{
		ExecutionID: invocation.ExecID,
		SessionID:   session.sessionID,
		Launch:      sandboxedLaunch,
		Caps:        acpCaps,
		WorkDir:     workDir,
		MCPServers:  append([]acpproto.McpServer(nil), mcpServers...),
	})
	defer w.unregisterActiveExecution(invocation.ExecID)

	if err := w.persistExecutionRoute(ctx, invocation.ExecID, agentCtx); err != nil {
		slog.Warn("executor worker: persist execution route failed", "exec_id", invocation.ExecID, "error", err)
	}

	collector := session.events
	if collector != nil {
		collector.Set(eventHandler)
		defer collector.Set(nil)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	result, err := session.client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: session.sessionID,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: invocation.Text}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ACP execution failed: %w", err)
	}

	w.pool.NoteTurn(ctx, agentCtx, session)
	if agentCtx != nil {
		w.touchExecutionOwner(ctx, agentCtx)
	}

	out := &runtimeapp.ExecutionResult{
		Text:       strings.TrimSpace(result.Text),
		StopReason: string(result.StopReason),
	}
	if result.Usage != nil {
		out.InputTokens = int64(result.Usage.InputTokens)
		out.OutputTokens = int64(result.Usage.OutputTokens)
		if result.Usage.CachedReadTokens != nil {
			out.CacheReadTokens = int64(*result.Usage.CachedReadTokens)
		}
		if result.Usage.CachedWriteTokens != nil {
			out.CacheWriteTokens = int64(*result.Usage.CachedWriteTokens)
		}
		if result.Usage.ThoughtTokens != nil {
			out.ReasoningTokens = int64(*result.Usage.ThoughtTokens)
		}
	}
	if agentCtx != nil && agentCtx.ID > 0 {
		id := agentCtx.ID
		out.AgentContextID = &id
	}
	return out, nil
}

func (w *ExecutorWorker) persistExecutionRoute(ctx context.Context, executionID int64, agentCtx *core.AgentContext) error {
	if w.cfg.Store == nil || agentCtx == nil {
		return nil
	}
	w.touchExecutionOwner(ctx, agentCtx)
	execRec, err := w.cfg.Store.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}
	execRec.AgentContextID = &agentCtx.ID
	return w.cfg.Store.UpdateExecution(ctx, execRec)
}

func (w *ExecutorWorker) touchExecutionOwner(ctx context.Context, agentCtx *core.AgentContext) {
	if agentCtx == nil || w.cfg.Store == nil {
		return
	}
	now := time.Now().UTC()
	agentCtx.WorkerID = w.cfg.WorkerID
	agentCtx.WorkerLastSeenAt = &now
	_ = w.cfg.Store.UpdateAgentContext(ctx, agentCtx)
}

func (w *ExecutorWorker) registerActiveExecution(executionID int64, target *activeExecutionProbeTarget) {
	w.mu.Lock()
	w.activeExecutions[executionID] = target
	w.mu.Unlock()
}

func (w *ExecutorWorker) unregisterActiveExecution(executionID int64) {
	w.mu.Lock()
	delete(w.activeExecutions, executionID)
	w.mu.Unlock()
}

func (w *ExecutorWorker) handleProbeRequest(msg *nats.Msg) {
	var req natsprobe.Request
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		w.respondProbe(msg, natsprobe.Response{
			Reachable:  false,
			Answered:   false,
			Error:      fmt.Sprintf("invalid probe request: %v", err),
			ObservedAt: time.Now().UTC(),
		})
		return
	}

	w.mu.Lock()
	target := w.activeExecutions[req.ExecutionID]
	w.mu.Unlock()
	if target == nil {
		w.respondProbe(msg, natsprobe.Response{
			Reachable:  false,
			Answered:   false,
			Error:      "execution is not active on worker",
			ObservedAt: time.Now().UTC(),
		})
		return
	}
	if sessionID := strings.TrimSpace(req.SessionID); sessionID != "" && sessionID != strings.TrimSpace(string(target.SessionID)) {
		w.respondProbe(msg, natsprobe.Response{
			Reachable:  false,
			Answered:   false,
			Error:      "execution session route mismatch",
			ObservedAt: time.Now().UTC(),
		})
		return
	}

	res, err := probeacp.Run(context.Background(), probeacp.Target{
		Launch:     target.Launch,
		Caps:       target.Caps,
		WorkDir:    target.WorkDir,
		MCPServers: target.MCPServers,
		SessionID:  target.SessionID,
		Question:   req.Question,
		Timeout:    time.Duration(req.TimeoutMS) * time.Millisecond,
	})
	if err != nil {
		w.respondProbe(msg, natsprobe.Response{
			Reachable:  false,
			Answered:   false,
			Error:      err.Error(),
			ObservedAt: time.Now().UTC(),
		})
		return
	}

	w.respondProbe(msg, natsprobe.Response{
		Reachable:  res.Reachable,
		Answered:   res.Answered,
		ReplyText:  res.ReplyText,
		Error:      res.Error,
		ObservedAt: res.ObservedAt,
	})
}

func (w *ExecutorWorker) respondProbe(msg *nats.Msg, res natsprobe.Response) {
	data, err := json.Marshal(res)
	if err != nil {
		slog.Error("executor worker: marshal probe response failed", "error", err)
		return
	}
	if err := msg.Respond(data); err != nil {
		slog.Error("executor worker: respond probe failed", "error", err)
	}
}

func resolveExecutionProfile(ctx context.Context, registry core.AgentRegistry, invocation *natsInvocationMessage) (*core.AgentProfile, *core.AgentDriver, error) {
	if registry == nil {
		return nil, nil, fmt.Errorf("registry is required")
	}
	profileID := strings.TrimSpace(invocation.ProfileID)
	if profileID == "" {
		profileID = strings.TrimSpace(invocation.AgentID)
	}
	profile, driver, err := registry.ResolveByID(ctx, profileID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve profile %q: %w", profileID, err)
	}
	return profile, driver, nil
}

// Stop gracefully shuts down the executor worker.
func (w *ExecutorWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.probeSub != nil {
		_ = w.probeSub.Unsubscribe()
	}
	if w.pool != nil {
		w.pool.Close()
	}
}

// natsEventForwarder publishes ACP events to NATS JetStream.
type natsEventForwarder struct {
	js      jetstream.JetStream
	subject string
	seq     *int64
	id      string
}

func (f *natsEventForwarder) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	*f.seq++
	msg := natsEventMessage{
		InvocationID: f.id,
		Seq:          *f.seq,
		Update:       update,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = f.js.Publish(ctx, f.subject, data)
	return err
}
