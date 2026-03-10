package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/v2/core"
)

const (
	defaultLeadProfileID   = "lead"
	defaultLeadTimeout     = 120 * time.Second
	defaultSessionIdleTTL  = 30 * time.Minute
)

// LeadAgentConfig configures the LeadAgent.
type LeadAgentConfig struct {
	Registry  core.AgentRegistry
	Bus       core.EventBus
	ProfileID string        // lead profile ID; defaults to "lead"
	Timeout   time.Duration // per-prompt timeout; defaults to 120s
	IdleTTL   time.Duration // session idle timeout; defaults to 30m
}

// ChatRequest is the input for a chat message to the lead agent.
type ChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	WorkDir   string `json:"work_dir,omitempty"`
}

// ChatResponse is the output from a chat message.
type ChatResponse struct {
	SessionID string `json:"session_id"`
	Reply     string `json:"reply"`
}

// LeadAgent manages persistent ACP sessions for direct user conversation.
// Users chat with the lead agent via REST; events stream via WebSocket.
type LeadAgent struct {
	cfg LeadAgentConfig

	mu       sync.Mutex
	sessions map[string]*leadSession

	activeMu   sync.Mutex
	activeRuns map[string]context.CancelFunc
}

type leadSession struct {
	client    ChatACPClient
	sessionID acpproto.SessionId
	workDir   string
	bridge    *EventBridge

	mu        sync.Mutex
	idleTimer *time.Timer
	closed    bool
}

// ChatACPClient is the minimal ACP client interface used by LeadAgent.
type ChatACPClient interface {
	NewSession(ctx context.Context, req acpproto.NewSessionRequest) (acpproto.SessionId, error)
	Prompt(ctx context.Context, req acpproto.PromptRequest) (*acpclient.PromptResult, error)
	Cancel(ctx context.Context, req acpproto.CancelNotification) error
	Close(ctx context.Context) error
}

// NewLeadAgent creates a LeadAgent.
func NewLeadAgent(cfg LeadAgentConfig) *LeadAgent {
	if cfg.ProfileID == "" {
		cfg.ProfileID = defaultLeadProfileID
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultLeadTimeout
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = defaultSessionIdleTTL
	}
	return &LeadAgent{
		cfg:        cfg,
		sessions:   make(map[string]*leadSession),
		activeRuns: make(map[string]context.CancelFunc),
	}
}

// Chat sends a message to the lead agent and returns its reply.
// If SessionID is empty, a new session ID is generated.
// Events are published to the bus for real-time WebSocket streaming.
func (l *LeadAgent) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, errors.New("message is required")
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = fmt.Sprintf("chat-%d", time.Now().UnixNano())
	}

	sess, err := l.getOrCreateSession(ctx, sessionID, strings.TrimSpace(req.WorkDir))
	if err != nil {
		return nil, err
	}
	sess.stopIdleTimer()

	// Register cancel func for abort support.
	promptCtx, promptCancel := context.WithTimeout(ctx, l.cfg.Timeout)
	defer promptCancel()

	l.activeMu.Lock()
	l.activeRuns[sessionID] = promptCancel
	l.activeMu.Unlock()
	defer func() {
		l.activeMu.Lock()
		delete(l.activeRuns, sessionID)
		l.activeMu.Unlock()
	}()

	result, err := sess.client.Prompt(promptCtx, acpproto.PromptRequest{
		SessionId: sess.sessionID,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: message}},
		},
	})

	// Flush any remaining chunks.
	sess.bridge.FlushPending(ctx)

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// Session may still be alive — keep it pooled.
			l.resetSessionIdle(sessionID, sess)
		} else {
			l.removeSession(sessionID)
		}
		return nil, fmt.Errorf("prompt failed: %w", err)
	}
	if result == nil {
		l.removeSession(sessionID)
		return nil, errors.New("empty result from agent")
	}

	reply := strings.TrimSpace(result.Text)
	if reply == "" {
		l.removeSession(sessionID)
		return nil, errors.New("empty reply from agent")
	}

	// Publish done event.
	sess.bridge.PublishData(ctx, map[string]any{
		"type":    "done",
		"content": reply,
	})

	l.resetSessionIdle(sessionID, sess)

	return &ChatResponse{
		SessionID: sessionID,
		Reply:     reply,
	}, nil
}

// CancelChat aborts the current prompt for the given session.
func (l *LeadAgent) CancelChat(sessionID string) error {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return errors.New("session_id is required")
	}

	l.activeMu.Lock()
	cancel, ok := l.activeRuns[id]
	l.activeMu.Unlock()

	if !ok {
		return errors.New("session is not running")
	}
	cancel()

	// Also send ACP cancel notification.
	l.mu.Lock()
	sess := l.sessions[id]
	l.mu.Unlock()
	if sess != nil {
		cancelCtx, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		_ = sess.client.Cancel(cancelCtx, acpproto.CancelNotification{
			SessionId: sess.sessionID,
		})
	}
	return nil
}

// CloseSession closes and removes a specific session.
func (l *LeadAgent) CloseSession(sessionID string) {
	l.removeSession(strings.TrimSpace(sessionID))
}

// Shutdown closes all sessions.
func (l *LeadAgent) Shutdown() {
	l.mu.Lock()
	sessions := make([]*leadSession, 0, len(l.sessions))
	for id, sess := range l.sessions {
		sessions = append(sessions, sess)
		delete(l.sessions, id)
	}
	l.mu.Unlock()

	for _, sess := range sessions {
		sess.close()
	}
}

// IsSessionAlive reports whether a session exists and is not closed.
func (l *LeadAgent) IsSessionAlive(sessionID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	sess, ok := l.sessions[strings.TrimSpace(sessionID)]
	return ok && !sess.isClosed()
}

// IsSessionRunning reports whether a session is currently processing a prompt.
func (l *LeadAgent) IsSessionRunning(sessionID string) bool {
	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	_, ok := l.activeRuns[strings.TrimSpace(sessionID)]
	return ok
}

// --- internal ---

func (l *LeadAgent) getOrCreateSession(ctx context.Context, sessionID, workDir string) (*leadSession, error) {
	l.mu.Lock()
	if sess, ok := l.sessions[sessionID]; ok && !sess.isClosed() {
		l.mu.Unlock()
		return sess, nil
	}
	l.mu.Unlock()

	// Create new session.
	if l.cfg.Registry == nil {
		return nil, errors.New("agent registry is not configured")
	}
	profile, driver, err := l.cfg.Registry.ResolveByID(ctx, l.cfg.ProfileID)
	if err != nil {
		return nil, fmt.Errorf("resolve lead profile %q: %w", l.cfg.ProfileID, err)
	}

	launchCfg := acpclient.LaunchConfig{
		Command: driver.LaunchCommand,
		Args:    driver.LaunchArgs,
		WorkDir: workDir,
		Env:     driver.Env,
	}

	bridge := NewEventBridge(l.cfg.Bus, core.EventChatOutput, EventBridgeScope{
		SessionID: sessionID,
	})

	client, err := acpclient.New(launchCfg, &acpclient.NopHandler{},
		acpclient.WithEventHandler(bridge))
	if err != nil {
		return nil, fmt.Errorf("launch lead agent: %w", err)
	}

	caps := profile.EffectiveCapabilities()
	acpCaps := acpclient.ClientCapabilities{
		FSRead:   caps.FSRead,
		FSWrite:  caps.FSWrite,
		Terminal: caps.Terminal,
	}

	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()

	if err := client.Initialize(initCtx, acpCaps); err != nil {
		_ = client.Close(context.Background())
		return nil, fmt.Errorf("initialize lead agent: %w", err)
	}

	acpSessionID, err := client.NewSession(initCtx, acpproto.NewSessionRequest{
		Cwd:        workDir,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		_ = client.Close(context.Background())
		return nil, fmt.Errorf("create lead session: %w", err)
	}

	sess := &leadSession{
		client:    client,
		sessionID: acpSessionID,
		workDir:   workDir,
		bridge:    bridge,
	}

	l.mu.Lock()
	// Close any stale session with same ID.
	if old, ok := l.sessions[sessionID]; ok {
		go old.close()
	}
	l.sessions[sessionID] = sess
	l.mu.Unlock()

	slog.Info("v2 lead session created", "session_id", sessionID, "profile", profile.ID, "driver", driver.ID)
	return sess, nil
}

func (l *LeadAgent) removeSession(sessionID string) {
	if sessionID == "" {
		return
	}
	l.mu.Lock()
	sess, ok := l.sessions[sessionID]
	if ok {
		delete(l.sessions, sessionID)
	}
	l.mu.Unlock()
	if sess != nil {
		sess.close()
	}
}

func (l *LeadAgent) resetSessionIdle(sessionID string, sess *leadSession) {
	sess.resetIdleTimer(l.cfg.IdleTTL, func() {
		l.removeSession(sessionID)
	})
}

// --- leadSession lifecycle ---

func (s *leadSession) stopIdleTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
}

func (s *leadSession) resetIdleTimer(d time.Duration, onExpire func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.idleTimer = time.AfterFunc(d, onExpire)
}

func (s *leadSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *leadSession) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	client := s.client
	s.mu.Unlock()

	if client != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = client.Close(closeCtx)
	}
}
