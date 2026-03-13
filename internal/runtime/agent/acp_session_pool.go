package agentruntime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acp"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

type acpSessionKey struct {
	issueID int64
	agentID string // runtime AgentProfile.ID
}

type pooledACPSession struct {
	key acpSessionKey

	client    *acpclient.Client
	sessionID acpproto.SessionId
	events    *switchingEventHandler

	mu           sync.Mutex // serialize prompts
	statsMu      sync.RWMutex
	lastUsed     time.Time
	turns        int
	inputTokens  int64 // cumulative input tokens in this session
	outputTokens int64 // cumulative output tokens in this session
}

type acpSessionFlight struct {
	wg   sync.WaitGroup
	sess *pooledACPSession
	ac   *core.AgentContext
	err  error
}

// switchingEventHandler forwards ACP events to the currently active handler.
// This allows a pooled session to reuse the same ACP process while emitting events
// scoped to the current (issue, step, exec) prompt.
type switchingEventHandler struct {
	mu sync.RWMutex
	h  acpclient.EventHandler
}

func (s *switchingEventHandler) Set(h acpclient.EventHandler) {
	s.mu.Lock()
	s.h = h
	s.mu.Unlock()
}

func (s *switchingEventHandler) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	s.mu.RLock()
	h := s.h
	s.mu.RUnlock()
	if h == nil {
		return nil
	}
	return h.HandleSessionUpdate(ctx, update)
}

// ACPSessionPool caches ACP processes + sessions per (issue, agent profile).
// It enables session reuse (prompt caching + conversational continuity).
type ACPSessionPool struct {
	store core.Store
	bus   core.EventBus

	mu              sync.Mutex
	sessions        map[acpSessionKey]*pooledACPSession
	inflight        map[acpSessionKey]*acpSessionFlight
	createSessionFn func(context.Context, acpSessionKey, acpSessionAcquireInput) (*pooledACPSession, *core.AgentContext, error)

	sub *core.Subscription
}

func NewACPSessionPool(store core.Store, bus core.EventBus) *ACPSessionPool {
	p := &ACPSessionPool{
		store:    store,
		bus:      bus,
		sessions: make(map[acpSessionKey]*pooledACPSession),
		inflight: make(map[acpSessionKey]*acpSessionFlight),
	}

	if bus != nil {
		p.sub = bus.Subscribe(core.SubscribeOpts{
			Types: []core.EventType{
				core.EventIssueCompleted,
				core.EventIssueFailed,
				core.EventIssueCancelled,
			},
			BufferSize: 64,
		})
		go p.watchIssueLifecycle()
	}

	return p
}

func (p *ACPSessionPool) Close() {
	if p == nil {
		return
	}
	if p.sub != nil && p.sub.Cancel != nil {
		p.sub.Cancel()
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for k, s := range p.sessions {
		delete(p.sessions, k)
		if s != nil && s.client != nil {
			_ = s.client.Close(context.Background())
		}
	}
}

func (p *ACPSessionPool) watchIssueLifecycle() {
	if p == nil || p.sub == nil {
		return
	}
	for ev := range p.sub.C {
		issueID := ev.IssueID
		if issueID == 0 {
			continue
		}
		p.CleanupIssue(issueID)
	}
}

func (p *ACPSessionPool) CleanupIssue(issueID int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, s := range p.sessions {
		if k.issueID != issueID {
			continue
		}
		delete(p.sessions, k)
		if s != nil && s.client != nil {
			_ = s.client.Close(context.Background())
		}
	}
}

type acpSessionAcquireInput struct {
	Profile *core.AgentProfile
	Driver  *core.AgentDriver

	Launch     acpclient.LaunchConfig
	Caps       acpclient.ClientCapabilities
	WorkDir    string
	MCPFactory func(agentSupportsSSE bool) []acpproto.McpServer
	IssueID    int64
	StepID     int64
	ExecID     int64
	IdleTTL    time.Duration
	MaxTurns   int
}

func (p *ACPSessionPool) Acquire(ctx context.Context, in acpSessionAcquireInput) (*pooledACPSession, *core.AgentContext, error) {
	if p == nil {
		return nil, nil, fmt.Errorf("nil session pool")
	}
	if in.Profile == nil || in.Driver == nil {
		return nil, nil, fmt.Errorf("profile/driver required")
	}

	key := acpSessionKey{issueID: in.IssueID, agentID: in.Profile.ID}

	// Fast path: existing session, evict if idle/max-turns exceeded.
	p.mu.Lock()
	if existing := p.sessions[key]; existing != nil {
		now := time.Now().UTC()
		lastUsed, turns, _, _ := existing.statsSnapshot()
		if in.IdleTTL > 0 && !lastUsed.IsZero() && now.Sub(lastUsed) > in.IdleTTL {
			delete(p.sessions, key)
			p.mu.Unlock()
			_ = existing.client.Close(context.Background())
		} else if in.MaxTurns > 0 && turns >= in.MaxTurns {
			delete(p.sessions, key)
			p.mu.Unlock()
			_ = existing.client.Close(context.Background())
		} else {
			p.mu.Unlock()
			ac, _ := p.findAgentContext(ctx, in.Profile.ID, in.IssueID)
			return existing, ac, nil
		}
	}
	if flight := p.inflight[key]; flight != nil {
		p.mu.Unlock()
		flight.wg.Wait()
		if flight.err != nil {
			return nil, flight.ac, flight.err
		}
		return flight.sess, flight.ac, nil
	}
	flight := &acpSessionFlight{}
	flight.wg.Add(1)
	p.inflight[key] = flight
	p.mu.Unlock()
	defer func() {
		flight.wg.Done()
		p.mu.Lock()
		delete(p.inflight, key)
		p.mu.Unlock()
	}()

	createFn := p.createSession
	if p.createSessionFn != nil {
		createFn = p.createSessionFn
	}
	sess, ac, err := createFn(ctx, key, in)
	flight.sess = sess
	flight.ac = ac
	flight.err = err
	if err != nil {
		return nil, ac, err
	}

	p.mu.Lock()
	if existing := p.sessions[key]; existing != nil {
		p.mu.Unlock()
		if sess != nil && sess.client != nil {
			_ = sess.client.Close(context.Background())
		}
		return existing, ac, nil
	}
	p.sessions[key] = sess
	p.mu.Unlock()

	slog.Info("runtime acp pool: session acquired",
		"issue_id", in.IssueID, "agent", in.Profile.ID,
		"loaded", ac != nil && strings.TrimSpace(ac.SessionID) == strings.TrimSpace(string(sess.sessionID)))

	return sess, ac, nil
}

func (p *ACPSessionPool) createSession(ctx context.Context, key acpSessionKey, in acpSessionAcquireInput) (*pooledACPSession, *core.AgentContext, error) {
	// Ensure AgentContext row exists (best-effort).
	ac, err := p.findAgentContext(ctx, in.Profile.ID, in.IssueID)
	if err == core.ErrNotFound {
		if p.store != nil {
			ac = &core.AgentContext{
				AgentID:   in.Profile.ID,
				IssueID:   in.IssueID,
				TurnCount: 0,
			}
			id, cErr := p.store.CreateAgentContext(ctx, ac)
			if cErr == nil {
				ac.ID = id
			} else {
				slog.Warn("runtime acp pool: create agent context failed", "agent", in.Profile.ID, "issue_id", in.IssueID, "error", cErr)
				ac = nil
			}
		} else {
			ac = nil
		}
	} else if err != nil {
		// Non-fatal; proceed without persistence.
		slog.Warn("runtime acp pool: find agent context failed", "agent", in.Profile.ID, "issue_id", in.IssueID, "error", err)
		ac = nil
	}

	// Create a new ACP process + session (or try to load a prior session id).
	slog.Info("runtime acp pool: launching agent process",
		"driver", in.Driver.ID, "command", in.Launch.Command,
		"args", in.Launch.Args, "work_dir", in.WorkDir,
		"issue_id", in.IssueID, "step_id", in.StepID)
	switcher := &switchingEventHandler{}
	handler := acphandler.NewACPHandler(in.WorkDir, "", nil)
	handler.SetSuppressEvents(true)
	client, err := acpclient.New(in.Launch, handler, acpclient.WithEventHandler(switcher))
	if err != nil {
		return nil, ac, fmt.Errorf("launch ACP agent %q: %w", in.Driver.ID, err)
	}
	slog.Info("runtime acp pool: agent process started, initializing",
		"driver", in.Driver.ID, "caps", fmt.Sprintf("%+v", in.Caps))
	if err := client.Initialize(ctx, in.Caps); err != nil {
		_ = client.Close(context.Background())
		return nil, ac, fmt.Errorf("initialize ACP agent %q: %w", in.Driver.ID, err)
	}
	slog.Info("runtime acp pool: agent initialized successfully",
		"driver", in.Driver.ID, "supports_sse_mcp", client.SupportsSSEMCP())

	var mcpServers []acpproto.McpServer
	if in.MCPFactory != nil {
		mcpServers = in.MCPFactory(client.SupportsSSEMCP())
	}
	slog.Info("runtime acp pool: creating session",
		"driver", in.Driver.ID, "mcp_server_count", len(mcpServers),
		"work_dir", in.WorkDir)

	var sessionID acpproto.SessionId
	loaded := false
	if ac != nil && strings.TrimSpace(ac.SessionID) != "" {
		slog.Info("runtime acp pool: attempting to load prior session",
			"session_id", ac.SessionID)
		sid, lErr := client.LoadSession(ctx, acpproto.LoadSessionRequest{
			SessionId:  acpproto.SessionId(strings.TrimSpace(ac.SessionID)),
			Cwd:        in.WorkDir,
			McpServers: mcpServers,
		})
		if lErr == nil && strings.TrimSpace(string(sid)) != "" {
			sessionID = sid
			loaded = true
			slog.Info("runtime acp pool: loaded prior session", "session_id", string(sid))
		} else if lErr != nil {
			slog.Warn("runtime acp pool: load prior session failed, will create new",
				"error", lErr)
		}
	}
	if !loaded {
		slog.Info("runtime acp pool: creating new session",
			"driver", in.Driver.ID, "work_dir", in.WorkDir)
		sid, nErr := client.NewSession(ctx, acpproto.NewSessionRequest{
			Cwd:        in.WorkDir,
			McpServers: mcpServers,
		})
		if nErr != nil {
			slog.Error("runtime acp pool: session/new failed",
				"driver", in.Driver.ID, "error", nErr,
				"work_dir", in.WorkDir, "mcp_count", len(mcpServers))
			_ = client.Close(context.Background())
			return nil, ac, fmt.Errorf("create ACP session: %w", nErr)
		}
		sessionID = sid
		slog.Info("runtime acp pool: new session created", "session_id", string(sid))
	}
	handler.SetSessionID(string(sessionID))

	sess := &pooledACPSession{
		key:       key,
		client:    client,
		sessionID: sessionID,
		events:    switcher,
		lastUsed:  time.Now().UTC(),
		turns:     0,
	}

	// Persist session id (best-effort).
	if ac != nil && strings.TrimSpace(string(sessionID)) != "" {
		ac.SessionID = strings.TrimSpace(string(sessionID))
		_ = p.store.UpdateAgentContext(ctx, ac)
	}

	return sess, ac, nil
}

func (p *ACPSessionPool) findAgentContext(ctx context.Context, agentID string, issueID int64) (*core.AgentContext, error) {
	if p == nil || p.store == nil {
		return nil, core.ErrNotFound
	}
	return p.store.FindAgentContext(ctx, agentID, issueID)
}

func (p *ACPSessionPool) NoteTurn(ctx context.Context, ac *core.AgentContext, sess *pooledACPSession) {
	if p == nil || sess == nil {
		return
	}
	now := time.Now().UTC()
	sess.statsMu.Lock()
	sess.lastUsed = now
	sess.turns++
	sess.statsMu.Unlock()

	if ac != nil {
		ac.TurnCount++
		ac.UpdatedAt = now
		_ = p.store.UpdateAgentContext(ctx, ac)
	}
}

// NoteTokens records token usage from the latest prompt.
func (p *ACPSessionPool) NoteTokens(sess *pooledACPSession, input, output int64) {
	if sess == nil {
		return
	}
	sess.statsMu.Lock()
	sess.inputTokens += input
	sess.outputTokens += output
	sess.statsMu.Unlock()
}

// TokenBudgetStatus describes the result of a token budget check.
type TokenBudgetStatus int

const (
	TokenBudgetOK       TokenBudgetStatus = iota // under warning threshold
	TokenBudgetWarning                           // above warning threshold but under hard limit
	TokenBudgetExceeded                          // at or above hard limit
)

// CheckTokenBudget evaluates whether the session's cumulative token usage
// is within the profile's configured budget. Returns OK if no budget is configured.
func (p *ACPSessionPool) CheckTokenBudget(sess *pooledACPSession, profile *core.AgentProfile) TokenBudgetStatus {
	if sess == nil || profile == nil {
		return TokenBudgetOK
	}
	maxTokens := profile.Session.MaxContextTokens
	if maxTokens <= 0 {
		return TokenBudgetOK
	}

	_, _, inputTokens, outputTokens := sess.statsSnapshot()
	totalUsed := inputTokens + outputTokens

	if totalUsed >= maxTokens {
		return TokenBudgetExceeded
	}

	warnRatio := profile.Session.ContextWarnRatio
	if warnRatio <= 0 {
		warnRatio = 0.8
	}
	if totalUsed >= int64(float64(maxTokens)*warnRatio) {
		return TokenBudgetWarning
	}

	return TokenBudgetOK
}

// SessionTokenUsage returns the cumulative token usage for a pooled session.
func (p *ACPSessionPool) SessionTokenUsage(sess *pooledACPSession) (input, output int64) {
	if sess == nil {
		return 0, 0
	}
	_, _, input, output = sess.statsSnapshot()
	return input, output
}

func (s *pooledACPSession) statsSnapshot() (time.Time, int, int64, int64) {
	if s == nil {
		return time.Time{}, 0, 0, 0
	}
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	return s.lastUsed, s.turns, s.inputTokens, s.outputTokens
}
