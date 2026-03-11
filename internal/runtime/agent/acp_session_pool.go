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
	flowID  int64
	agentID string // runtime AgentProfile.ID
}

type pooledACPSession struct {
	key acpSessionKey

	client    *acpclient.Client
	sessionID acpproto.SessionId
	events    *switchingEventHandler

	mu       sync.Mutex // serialize prompts
	lastUsed time.Time
	turns    int
}

// switchingEventHandler forwards ACP events to the currently active handler.
// This allows a pooled session to reuse the same ACP process while emitting events
// scoped to the current (flow, step, exec) prompt.
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

// ACPSessionPool caches ACP processes + sessions per (flow, agent profile).
// It enables session reuse (prompt caching + conversational continuity).
type ACPSessionPool struct {
	store core.Store
	bus   core.EventBus

	mu       sync.Mutex
	sessions map[acpSessionKey]*pooledACPSession

	sub *core.Subscription
}

func NewACPSessionPool(store core.Store, bus core.EventBus) *ACPSessionPool {
	p := &ACPSessionPool{
		store:    store,
		bus:      bus,
		sessions: make(map[acpSessionKey]*pooledACPSession),
	}

	if bus != nil {
		p.sub = bus.Subscribe(core.SubscribeOpts{
			Types: []core.EventType{
				core.EventFlowCompleted,
				core.EventFlowFailed,
				core.EventFlowCancelled,
			},
			BufferSize: 64,
		})
		go p.watchFlowLifecycle()
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

func (p *ACPSessionPool) watchFlowLifecycle() {
	if p == nil || p.sub == nil {
		return
	}
	for ev := range p.sub.C {
		flowID := ev.FlowID
		if flowID == 0 {
			continue
		}
		p.CleanupFlow(flowID)
	}
}

func (p *ACPSessionPool) CleanupFlow(flowID int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, s := range p.sessions {
		if k.flowID != flowID {
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
	FlowID     int64
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

	key := acpSessionKey{flowID: in.FlowID, agentID: in.Profile.ID}

	// Fast path: existing session, evict if idle/max-turns exceeded.
	p.mu.Lock()
	if existing := p.sessions[key]; existing != nil {
		now := time.Now().UTC()
		if in.IdleTTL > 0 && !existing.lastUsed.IsZero() && now.Sub(existing.lastUsed) > in.IdleTTL {
			delete(p.sessions, key)
			p.mu.Unlock()
			_ = existing.client.Close(context.Background())
		} else if in.MaxTurns > 0 && existing.turns >= in.MaxTurns {
			delete(p.sessions, key)
			p.mu.Unlock()
			_ = existing.client.Close(context.Background())
		} else {
			p.mu.Unlock()
			ac, _ := p.store.FindAgentContext(ctx, in.Profile.ID, in.FlowID)
			return existing, ac, nil
		}
	} else {
		p.mu.Unlock()
	}

	// Ensure AgentContext row exists (best-effort).
	ac, err := p.store.FindAgentContext(ctx, in.Profile.ID, in.FlowID)
	if err == core.ErrNotFound {
		ac = &core.AgentContext{
			AgentID:   in.Profile.ID,
			FlowID:    in.FlowID,
			TurnCount: 0,
		}
		id, cErr := p.store.CreateAgentContext(ctx, ac)
		if cErr == nil {
			ac.ID = id
		} else {
			slog.Warn("runtime acp pool: create agent context failed", "agent", in.Profile.ID, "flow_id", in.FlowID, "error", cErr)
			ac = nil
		}
	} else if err != nil {
		// Non-fatal; proceed without persistence.
		slog.Warn("runtime acp pool: find agent context failed", "agent", in.Profile.ID, "flow_id", in.FlowID, "error", err)
		ac = nil
	}

	// Create a new ACP process + session (or try to load a prior session id).
	switcher := &switchingEventHandler{}
	handler := acphandler.NewACPHandler(in.WorkDir, "", nil)
	handler.SetSuppressEvents(true)
	client, err := acpclient.New(in.Launch, handler, acpclient.WithEventHandler(switcher))
	if err != nil {
		return nil, ac, fmt.Errorf("launch ACP agent %q: %w", in.Driver.ID, err)
	}
	if err := client.Initialize(ctx, in.Caps); err != nil {
		_ = client.Close(context.Background())
		return nil, ac, fmt.Errorf("initialize ACP agent %q: %w", in.Driver.ID, err)
	}

	var mcpServers []acpproto.McpServer
	if in.MCPFactory != nil {
		mcpServers = in.MCPFactory(client.SupportsSSEMCP())
	}

	var sessionID acpproto.SessionId
	loaded := false
	if ac != nil && strings.TrimSpace(ac.SessionID) != "" {
		sid, lErr := client.LoadSession(ctx, acpproto.LoadSessionRequest{
			SessionId:  acpproto.SessionId(strings.TrimSpace(ac.SessionID)),
			Cwd:        in.WorkDir,
			McpServers: mcpServers,
		})
		if lErr == nil && strings.TrimSpace(string(sid)) != "" {
			sessionID = sid
			loaded = true
		}
	}
	if !loaded {
		sid, nErr := client.NewSession(ctx, acpproto.NewSessionRequest{
			Cwd:        in.WorkDir,
			McpServers: mcpServers,
		})
		if nErr != nil {
			_ = client.Close(context.Background())
			return nil, ac, fmt.Errorf("create ACP session: %w", nErr)
		}
		sessionID = sid
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

	p.mu.Lock()
	p.sessions[key] = sess
	p.mu.Unlock()

	// Persist session id (best-effort).
	if ac != nil && strings.TrimSpace(string(sessionID)) != "" {
		ac.SessionID = strings.TrimSpace(string(sessionID))
		_ = p.store.UpdateAgentContext(ctx, ac)
	}

	slog.Info("runtime acp pool: session acquired",
		"flow_id", in.FlowID, "agent", in.Profile.ID,
		"loaded", loaded)

	return sess, ac, nil
}

func (p *ACPSessionPool) NoteTurn(ctx context.Context, ac *core.AgentContext, sess *pooledACPSession) {
	if p == nil || sess == nil {
		return
	}
	now := time.Now().UTC()
	sess.lastUsed = now
	sess.turns++

	if ac != nil {
		ac.TurnCount++
		ac.UpdatedAt = now
		_ = p.store.UpdateAgentContext(ctx, ac)
	}
}
