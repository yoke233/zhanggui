package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	natsprobe "github.com/yoke233/ai-workflow/internal/adapters/probe/nats"
	runtimeapp "github.com/yoke233/ai-workflow/internal/application/runtime"
	"github.com/yoke233/ai-workflow/internal/core"
)

// NATSSessionManagerConfig configures the NATS-backed session manager.
type NATSSessionManagerConfig struct {
	// NATSConn is an already-connected NATS connection.
	NATSConn *nats.Conn

	// StreamPrefix is the JetStream stream name prefix (default: "aiworkflow").
	StreamPrefix string

	// ServerID uniquely identifies this server in multi-server setups.
	// Used as a prefix in invocation IDs to avoid collisions across servers.
	// Auto-generated from hostname + PID if empty.
	ServerID string

	// Store is used for persisting execution metadata.
	Store core.Store
}

// NATSSessionManager implements SessionManager using NATS JetStream.
// Executions are published as messages, executors consume them via queue groups,
// results and events are streamed back through dedicated subjects.
//
// Subject layout:
//
//	{prefix}.invocation.submit.{agent_type}  — execution submission (consumed by executors)
//	{prefix}.invocation.result.{invocation_id}   — final result
//	{prefix}.invocation.events.{invocation_id}   — streaming events during execution
//	{prefix}.executor.register           — executor heartbeat/registration
type NATSSessionManager struct {
	nc       *nats.Conn
	js       jetstream.JetStream
	prefix   string
	serverID string
	store    core.Store

	mu      sync.Mutex
	handles map[string]*natsHandle
	nextID  int64

	activeCount atomic.Int32
	drainWg     sync.WaitGroup
}

type natsHandle struct {
	id        string
	sessionIn runtimeapp.SessionAcquireInput
}

// natsInvocationMessage is the payload published to the execution submission subject.
type natsInvocationMessage struct {
	InvocationID string                         `json:"invocation_id"`
	HandleID     string                         `json:"handle_id"`
	Text         string                         `json:"text"`
	Input        runtimeapp.SessionAcquireInput `json:"-"` // serialized separately
	FlowID       int64                          `json:"flow_id"`
	StepID       int64                          `json:"step_id"`
	ExecID       int64                          `json:"execution_id"`
	AgentID      string                         `json:"agent_id"`
	ProfileID    string                         `json:"profile_id"`
	WorkDir      string                         `json:"work_dir"`
}

// natsInvocationResult is the payload published to the result subject.
type natsInvocationResult struct {
	InvocationID   string `json:"invocation_id"`
	Text           string `json:"text"`
	StopReason     string `json:"stop_reason"`
	InputTokens    int64  `json:"input_tokens"`
	OutputTokens   int64  `json:"output_tokens"`
	AgentContextID *int64 `json:"agent_context_id,omitempty"`
	Error          string `json:"error,omitempty"`
}

// natsEventMessage wraps a streaming event for NATS transport.
type natsEventMessage struct {
	InvocationID string                  `json:"invocation_id"`
	Seq          int64                   `json:"seq"`
	Update       acpclient.SessionUpdate `json:"update"`
}

// NewNATSSessionManager creates a NATS-backed session manager.
func NewNATSSessionManager(cfg NATSSessionManagerConfig) (*NATSSessionManager, error) {
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

	serverID := strings.TrimSpace(cfg.ServerID)
	if serverID == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "unknown"
		}
		serverID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}

	m := &NATSSessionManager{
		nc:       cfg.NATSConn,
		js:       js,
		prefix:   prefix,
		serverID: serverID,
		store:    cfg.Store,
		handles:  make(map[string]*natsHandle),
	}

	if err := m.ensureStreams(context.Background()); err != nil {
		return nil, fmt.Errorf("ensure JetStream streams: %w", err)
	}

	return m, nil
}

func (m *NATSSessionManager) ensureStreams(ctx context.Context) error {
	// Execution submission stream — consumed by executor workers.
	_, err := m.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      m.prefix + "_invocations",
		Subjects:  []string{m.prefix + ".invocation.submit.>"},
		Retention: jetstream.WorkQueuePolicy,
		MaxAge:    24 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("create invocations stream: %w", err)
	}

	// Results stream — published by executors, consumed by watchers.
	_, err = m.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      m.prefix + "_results",
		Subjects:  []string{m.prefix + ".invocation.result.>"},
		Retention: jetstream.InterestPolicy,
		MaxAge:    24 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("create results stream: %w", err)
	}

	// Events stream — streaming events during execution.
	_, err = m.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      m.prefix + "_events",
		Subjects:  []string{m.prefix + ".invocation.events.>"},
		Retention: jetstream.InterestPolicy,
		MaxAge:    1 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("create events stream: %w", err)
	}

	return nil
}

// Acquire stores session metadata locally — actual ACP session creation happens on the executor.
func (m *NATSSessionManager) Acquire(_ context.Context, in runtimeapp.SessionAcquireInput) (*runtimeapp.SessionHandle, error) {
	m.mu.Lock()
	m.nextID++
	handleID := fmt.Sprintf("nats-%d", m.nextID)
	nh := &natsHandle{
		id:        handleID,
		sessionIn: in,
	}
	m.handles[handleID] = nh
	m.mu.Unlock()

	return &runtimeapp.SessionHandle{ID: handleID}, nil
}

// StartExecution publishes the execution request to JetStream for remote execution.
func (m *NATSSessionManager) StartExecution(ctx context.Context, handle *runtimeapp.SessionHandle, text string) (string, error) {
	m.mu.Lock()
	nh, ok := m.handles[handle.ID]
	if !ok {
		m.mu.Unlock()
		return "", fmt.Errorf("session handle %q not found", handle.ID)
	}
	m.nextID++
	invocationID := fmt.Sprintf("ni-%s-%d-%d", m.serverID, time.Now().UnixNano(), m.nextID)
	m.mu.Unlock()

	agentType := "default"
	if nh.sessionIn.Driver != nil {
		agentType = nh.sessionIn.Driver.ID
	}

	msg := natsInvocationMessage{
		InvocationID: invocationID,
		HandleID:     handle.ID,
		Text:         text,
		FlowID:       nh.sessionIn.FlowID,
		StepID:       nh.sessionIn.StepID,
		ExecID:       nh.sessionIn.ExecID,
		AgentID:      agentType,
		WorkDir:      nh.sessionIn.WorkDir,
	}
	if nh.sessionIn.Profile != nil {
		msg.ProfileID = strings.TrimSpace(nh.sessionIn.Profile.ID)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshal execution message: %w", err)
	}

	subject := fmt.Sprintf("%s.invocation.submit.%s", m.prefix, agentType)
	_, err = m.js.Publish(ctx, subject, data)
	if err != nil {
		return "", fmt.Errorf("publish execution to NATS: %w", err)
	}

	m.activeCount.Add(1)
	m.drainWg.Add(1)

	slog.Info("nats session manager: execution dispatched",
		"exec_id", msg.ExecID, "agent", agentType, "flow_id", msg.FlowID)

	return invocationID, nil
}

// WatchExecution subscribes to the result and event subjects for a given invocation.
// It blocks until the result is received or ctx is cancelled.
func (m *NATSSessionManager) WatchExecution(ctx context.Context, invocationID string, lastEventSeq int64, sink runtimeapp.EventSink) (*runtimeapp.ExecutionResult, error) {
	defer func() {
		m.activeCount.Add(-1)
		m.drainWg.Done()
	}()

	// Subscribe to events stream for this invocation.
	eventSubject := fmt.Sprintf("%s.invocation.events.%s", m.prefix, invocationID)
	resultSubject := fmt.Sprintf("%s.invocation.result.%s", m.prefix, invocationID)

	// Create ephemeral consumer for events.
	if sink != nil {
		eventConsumer, err := m.js.CreateOrUpdateConsumer(ctx, m.prefix+"_events", jetstream.ConsumerConfig{
			FilterSubject: eventSubject,
			DeliverPolicy: jetstream.DeliverAllPolicy,
			AckPolicy:     jetstream.AckExplicitPolicy,
		})
		if err != nil {
			slog.Warn("nats watch: failed to create event consumer", "invocation_id", invocationID, "error", err)
		} else {
			go m.consumeEvents(ctx, eventConsumer, lastEventSeq, sink)
		}
	}

	// Create consumer for result.
	resultConsumer, err := m.js.CreateOrUpdateConsumer(ctx, m.prefix+"_results", jetstream.ConsumerConfig{
		FilterSubject: resultSubject,
		DeliverPolicy: jetstream.DeliverLastPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("create result consumer: %w", err)
	}

	// Block until result message arrives.
	for {
		msgs, err := resultConsumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		for msg := range msgs.Messages() {
			var result natsInvocationResult
			if err := json.Unmarshal(msg.Data(), &result); err != nil {
				_ = msg.Nak()
				return nil, fmt.Errorf("unmarshal result: %w", err)
			}
			_ = msg.Ack()

			if result.Error != "" {
				return nil, fmt.Errorf("remote execution failed: %s", result.Error)
			}

			return &runtimeapp.ExecutionResult{
				Text:           result.Text,
				StopReason:     result.StopReason,
				InputTokens:    result.InputTokens,
				OutputTokens:   result.OutputTokens,
				AgentContextID: result.AgentContextID,
			}, nil
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
}

func (m *NATSSessionManager) consumeEvents(ctx context.Context, consumer jetstream.Consumer, lastEventSeq int64, sink runtimeapp.EventSink) {
	for {
		msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		for msg := range msgs.Messages() {
			var ev natsEventMessage
			if err := json.Unmarshal(msg.Data(), &ev); err != nil {
				_ = msg.Nak()
				continue
			}
			_ = msg.Ack()
			if ev.Seq <= lastEventSeq {
				continue
			}
			_ = sink.HandleSessionUpdate(ctx, ev.Update)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// RecoverExecutions queries NATS for executions that may have been in-flight during a restart.
func (m *NATSSessionManager) RecoverExecutions(ctx context.Context, since time.Time) ([]runtimeapp.ExecutionRuntimeStatus, error) {
	// In NATS mode, executions that were published but not yet consumed are still in the stream.
	// Executions that were being executed will have their results published by the executor.
	// We return an empty list here — the executor worker handles recovery by re-publishing results.
	slog.Info("nats session manager: recovery check", "since", since)
	return nil, nil
}

// ProbeExecution routes a probe request to the owning remote worker over NATS request-reply.
func (m *NATSSessionManager) ProbeExecution(ctx context.Context, req runtimeapp.ExecutionProbeRuntimeRequest) (*runtimeapp.ExecutionProbeRuntimeResult, error) {
	if strings.TrimSpace(req.OwnerID) == "" {
		return &runtimeapp.ExecutionProbeRuntimeResult{
			Reachable:  false,
			Error:      "missing execution owner",
			ObservedAt: time.Now().UTC(),
		}, nil
	}

	payload, err := json.Marshal(natsprobe.Request{
		ExecutionID:  req.ExecutionID,
		SessionID:    req.SessionID,
		InvocationID: req.InvocationID,
		Question:     req.Question,
		TimeoutMS:    req.Timeout.Milliseconds(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal probe request: %w", err)
	}

	subject := fmt.Sprintf("%s.probe.request.%s", m.prefix, req.OwnerID)
	replyCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		replyCtx, cancel = context.WithTimeout(ctx, req.Timeout+(2*time.Second))
	}
	defer cancel()

	msg, err := m.nc.RequestWithContext(replyCtx, subject, payload)
	if err != nil {
		return &runtimeapp.ExecutionProbeRuntimeResult{
			Reachable:  false,
			Error:      fmt.Sprintf("probe owner unreachable: %v", err),
			ObservedAt: time.Now().UTC(),
		}, nil
	}

	var res natsprobe.Response
	if err := json.Unmarshal(msg.Data, &res); err != nil {
		return nil, fmt.Errorf("unmarshal probe response: %w", err)
	}

	return &runtimeapp.ExecutionProbeRuntimeResult{
		Reachable:  res.Reachable,
		Answered:   res.Answered,
		ReplyText:  res.ReplyText,
		Error:      res.Error,
		ObservedAt: res.ObservedAt,
	}, nil
}

// Release is a no-op in NATS mode — sessions are managed by executors.
func (m *NATSSessionManager) Release(_ context.Context, handle *runtimeapp.SessionHandle) error {
	m.mu.Lock()
	delete(m.handles, handle.ID)
	m.mu.Unlock()
	return nil
}

// CleanupFlow is a no-op in NATS mode — executor workers manage their own sessions.
func (m *NATSSessionManager) CleanupFlow(_ int64) {}

// DrainActive blocks until all in-flight executions complete.
func (m *NATSSessionManager) DrainActive(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		m.drainWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ActiveCount returns the number of invocations being watched.
func (m *NATSSessionManager) ActiveCount() int {
	return int(m.activeCount.Load())
}

// Close drains the NATS connection.
func (m *NATSSessionManager) Close() {
	if m.nc != nil {
		m.nc.Drain()
	}
}
