package agentruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	"github.com/yoke233/ai-workflow/internal/core"
)

// ---------------------------------------------------------------------------
// Fake ACP server — handles JSON-RPC over pipes
// ---------------------------------------------------------------------------

type fakeACPServer struct {
	mu        sync.Mutex
	replyText string
	prompts   []string
}

func (s *fakeACPServer) serve(reader io.Reader, writer io.Writer) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var msg struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method,omitempty"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil || msg.Method == "" {
			continue
		}
		switch msg.Method {
		case "session/prompt":
			s.handlePrompt(msg.ID, scanner.Bytes(), writer)
		}
	}
}

func (s *fakeACPServer) handlePrompt(id json.RawMessage, raw []byte, w io.Writer) {
	// ContentBlock serializes as {"text":"...", "type":"text"} (flat, not nested).
	var full struct {
		Params struct {
			Prompt []struct {
				Text string `json:"text"`
			} `json:"prompt"`
		} `json:"params"`
	}
	_ = json.Unmarshal(raw, &full)

	s.mu.Lock()
	for _, block := range full.Params.Prompt {
		if block.Text != "" {
			s.prompts = append(s.prompts, block.Text)
		}
	}
	reply := s.replyText
	s.mu.Unlock()

	resp := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"end_turn","text":%q,"usage":{"inputTokens":100,"outputTokens":50}}}`,
		string(id), reply,
	)
	fmt.Fprintln(w, resp)
}

func (s *fakeACPServer) getPrompts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]string, len(s.prompts))
	copy(cp, s.prompts)
	return cp
}

// ---------------------------------------------------------------------------
// Mock agent registry — returns configured profiles
// ---------------------------------------------------------------------------

type mockRegistry struct {
	profiles map[string]*core.AgentProfile
}

func (r *mockRegistry) GetProfile(_ context.Context, id string) (*core.AgentProfile, error) {
	if p, ok := r.profiles[id]; ok {
		return p, nil
	}
	return nil, core.ErrProfileNotFound
}
func (r *mockRegistry) ListProfiles(context.Context) ([]*core.AgentProfile, error) {
	return nil, nil
}
func (r *mockRegistry) CreateProfile(context.Context, *core.AgentProfile) error { return nil }
func (r *mockRegistry) UpdateProfile(context.Context, *core.AgentProfile) error { return nil }
func (r *mockRegistry) DeleteProfile(context.Context, string) error             { return nil }
func (r *mockRegistry) ResolveForAction(context.Context, *core.Action) (*core.AgentProfile, error) {
	return nil, core.ErrProfileNotFound
}
func (r *mockRegistry) ResolveByID(_ context.Context, id string) (*core.AgentProfile, error) {
	if p, ok := r.profiles[id]; ok {
		return p, nil
	}
	return nil, core.ErrProfileNotFound
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testBootstrapFn creates a mock Bootstrap function. Each call creates
// a pipe-based ACP client backed by the given fake server.
func testBootstrapFn(server *fakeACPServer, sessionID string) func(context.Context, acpclient.BootstrapConfig) (*acpclient.BootstrapResult, error) {
	return func(_ context.Context, cfg acpclient.BootstrapConfig) (*acpclient.BootstrapResult, error) {
		clientR, serverW := io.Pipe()
		serverR, clientW := io.Pipe()

		handler := cfg.Handler
		if handler == nil {
			handler = &acpclient.NopHandler{}
		}

		var opts []acpclient.Option
		if cfg.EventHandler != nil {
			opts = append(opts, acpclient.WithEventHandler(cfg.EventHandler))
		}

		client, err := acpclient.NewWithIO(
			acpclient.LaunchConfig{Command: "fake"},
			handler, clientW, clientR, opts...,
		)
		if err != nil {
			return nil, err
		}

		go server.serve(serverR, serverW)

		return &acpclient.BootstrapResult{
			Client: client,
			Session: acpclient.BootstrapSessionOutput{
				ID: acpproto.SessionId(sessionID),
			},
		}, nil
	}
}

func newTestProfile(id string) *core.AgentProfile {
	return &core.AgentProfile{
		ID:   id,
		Name: id,
		Role: core.RoleWorker,
		Driver: core.DriverConfig{
			LaunchCommand: "echo",
		},
	}
}


// waitForAgentStatus polls until the member reaches the expected status.
func waitForAgentStatus(t *testing.T, store core.Store, threadID int64, profileID string, want core.ThreadAgentStatus, timeout time.Duration) *core.ThreadMember {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		members, _ := store.ListThreadMembers(context.Background(), threadID)
		for _, m := range members {
			if m.AgentProfileID == profileID && m.Status == want {
				return m
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for agent %q to reach status %q", profileID, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestThreadPoolFullLifecycle(t *testing.T) {
	server := &fakeACPServer{replyText: "Hello from agent"}
	profile := newTestProfile("agent-lifecycle")

	store := newThreadSessionPoolTestStore(t)
	bus := membus.NewBus()
	dataDir := t.TempDir()
	ctx := context.Background()

	registry := &mockRegistry{profiles: map[string]*core.AgentProfile{
		profile.ID: profile,
	}}
	pool := NewThreadSessionPool(store, bus, registry, dataDir)
	pool.bootstrapFn = testBootstrapFn(server, "session-lifecycle")
	defer pool.Close()

	threadID, err := store.CreateThread(ctx, &core.Thread{
		Title:   "Lifecycle Thread",
		OwnerID: "owner-1",
		Status:  core.ThreadActive,
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// 1. InviteAgent — should start booting in background.
	member, err := pool.InviteAgent(ctx, threadID, profile.ID)
	if err != nil {
		t.Fatalf("InviteAgent: %v", err)
	}
	if member.Status != core.ThreadAgentBooting {
		t.Fatalf("expected booting, got %q", member.Status)
	}

	// 2. Wait for boot to complete.
	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()
	if err := pool.WaitAgentReady(waitCtx, threadID, profile.ID); err != nil {
		t.Fatalf("WaitAgentReady: %v", err)
	}

	// Verify member is now active in DB.
	activeMember := waitForAgentStatus(t, store, threadID, profile.ID, core.ThreadAgentActive, 2*time.Second)
	if activeMember == nil {
		t.Fatal("expected active member")
	}

	// Verify boot prompt was sent.
	prompts := server.getPrompts()
	if len(prompts) == 0 {
		t.Fatal("expected boot prompt to be sent")
	}
	if !strings.Contains(prompts[0], "Lifecycle Thread") {
		t.Errorf("boot prompt should contain thread title, got: %s", prompts[0])
	}

	// 3. SendMessage — should get reply.
	if err := pool.SendMessage(ctx, threadID, profile.ID, "What is your status?"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Verify prompt was captured.
	prompts = server.getPrompts()
	found := false
	for _, p := range prompts {
		if p == "What is your status?" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected user message in prompts, got: %v", prompts)
	}

	// Verify agent response saved as thread message.
	msgs, _ := store.ListThreadMessages(ctx, threadID, 10, 0)
	var agentReply *core.ThreadMessage
	for _, m := range msgs {
		if m.Role == "agent" && m.SenderID == profile.ID {
			agentReply = m
			break
		}
	}
	if agentReply == nil {
		t.Fatal("expected agent reply saved as thread message")
	}
	if agentReply.Content != "Hello from agent" {
		t.Errorf("agent reply = %q, want %q", agentReply.Content, "Hello from agent")
	}

	// 4. Verify active profile IDs.
	activeIDs := pool.ActiveAgentProfileIDs(threadID)
	if len(activeIDs) != 1 || activeIDs[0] != profile.ID {
		t.Errorf("ActiveAgentProfileIDs = %v, want [%s]", activeIDs, profile.ID)
	}

	// 5. RemoveAgent — should capture progress summary and set paused.
	if err := pool.RemoveAgent(ctx, threadID, activeMember.ID); err != nil {
		t.Fatalf("RemoveAgent: %v", err)
	}

	// Verify session removed from pool.
	if ids := pool.ActiveAgentProfileIDs(threadID); len(ids) != 0 {
		t.Errorf("expected no active agents after remove, got %v", ids)
	}

	// Verify member status in DB (should be paused if summary was captured, or left).
	members, _ := store.ListThreadMembers(ctx, threadID)
	for _, m := range members {
		if m.AgentProfileID == profile.ID {
			if m.Status != core.ThreadAgentPaused && m.Status != core.ThreadAgentLeft {
				t.Errorf("expected paused or left, got %q", m.Status)
			}
			break
		}
	}
}

func TestThreadPoolSendMessage(t *testing.T) {
	server := &fakeACPServer{replyText: "I completed the task"}
	profile := newTestProfile("agent-msg")

	store := newThreadSessionPoolTestStore(t)
	bus := membus.NewBus()
	dataDir := t.TempDir()
	ctx := context.Background()

	pool := NewThreadSessionPool(store, bus, &mockRegistry{
		profiles: map[string]*core.AgentProfile{profile.ID: profile},
	}, dataDir)
	pool.bootstrapFn = testBootstrapFn(server, "session-msg")
	defer pool.Close()

	threadID, _ := store.CreateThread(ctx, &core.Thread{
		Title: "Msg Thread", OwnerID: "owner-1", Status: core.ThreadActive,
	})

	// Invite and wait.
	if _, err := pool.InviteAgent(ctx, threadID, profile.ID); err != nil {
		t.Fatalf("InviteAgent: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.WaitAgentReady(waitCtx, threadID, profile.ID); err != nil {
		t.Fatalf("WaitAgentReady: %v", err)
	}

	// Send multiple messages.
	for i, msg := range []string{"First message", "Second message"} {
		if err := pool.SendMessage(ctx, threadID, profile.ID, msg); err != nil {
			t.Fatalf("SendMessage[%d]: %v", i, err)
		}
	}

	// Verify all prompts received.
	prompts := server.getPrompts()
	var userMsgs int
	for _, p := range prompts {
		if p == "First message" || p == "Second message" {
			userMsgs++
		}
	}
	if userMsgs != 2 {
		t.Errorf("expected 2 user messages in prompts, got %d (prompts: %v)", userMsgs, prompts)
	}

	// Verify agent replies saved.
	msgs, _ := store.ListThreadMessages(ctx, threadID, 10, 0)
	var agentReplies int
	for _, m := range msgs {
		if m.Role == "agent" && m.Content == "I completed the task" {
			agentReplies++
		}
	}
	if agentReplies != 2 {
		t.Errorf("expected 2 agent replies, got %d", agentReplies)
	}
}

func TestThreadPoolTokenTracking(t *testing.T) {
	server := &fakeACPServer{replyText: "token reply"}
	profile := newTestProfile("agent-tokens")

	store := newThreadSessionPoolTestStore(t)
	bus := membus.NewBus()
	dataDir := t.TempDir()
	ctx := context.Background()

	pool := NewThreadSessionPool(store, bus, &mockRegistry{
		profiles: map[string]*core.AgentProfile{profile.ID: profile},
	}, dataDir)
	pool.bootstrapFn = testBootstrapFn(server, "session-tokens")
	defer pool.Close()

	threadID, _ := store.CreateThread(ctx, &core.Thread{
		Title: "Token Thread", OwnerID: "owner-1", Status: core.ThreadActive,
	})

	if _, err := pool.InviteAgent(ctx, threadID, profile.ID); err != nil {
		t.Fatalf("InviteAgent: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.WaitAgentReady(waitCtx, threadID, profile.ID); err != nil {
		t.Fatalf("WaitAgentReady: %v", err)
	}

	// Send 3 messages (fake server returns 100 input + 50 output each).
	for i := 0; i < 3; i++ {
		if err := pool.SendMessage(ctx, threadID, profile.ID, fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("SendMessage[%d]: %v", i, err)
		}
	}

	// Check in-memory token stats.
	key := threadSessionKey{threadID: threadID, agentID: profile.ID}
	pool.mu.Lock()
	pooled := pool.sessions[key]
	pool.mu.Unlock()

	if pooled == nil {
		t.Fatal("expected pooled session")
	}
	// 3 messages × (100 input + 50 output) = 300 input, 150 output.
	if pooled.inputTokens != 300 {
		t.Errorf("inputTokens = %d, want 300", pooled.inputTokens)
	}
	if pooled.outputTokens != 150 {
		t.Errorf("outputTokens = %d, want 150", pooled.outputTokens)
	}
	if pooled.turns != 3 {
		t.Errorf("turns = %d, want 3", pooled.turns)
	}
}

func TestThreadPoolBootFailure(t *testing.T) {
	profile := newTestProfile("agent-fail")

	store := newThreadSessionPoolTestStore(t)
	bus := membus.NewBus()
	dataDir := t.TempDir()
	ctx := context.Background()

	pool := NewThreadSessionPool(store, bus, &mockRegistry{
		profiles: map[string]*core.AgentProfile{profile.ID: profile},
	}, dataDir)
	// Mock Bootstrap that always fails.
	pool.bootstrapFn = func(_ context.Context, _ acpclient.BootstrapConfig) (*acpclient.BootstrapResult, error) {
		return nil, fmt.Errorf("simulated boot failure")
	}
	defer pool.Close()

	threadID, _ := store.CreateThread(ctx, &core.Thread{
		Title: "Fail Thread", OwnerID: "owner-1", Status: core.ThreadActive,
	})

	member, err := pool.InviteAgent(ctx, threadID, profile.ID)
	if err != nil {
		t.Fatalf("InviteAgent: %v", err)
	}
	if member.Status != core.ThreadAgentBooting {
		t.Fatalf("expected booting, got %q", member.Status)
	}

	// Wait for background boot to fail.
	waitForAgentStatus(t, store, threadID, profile.ID, core.ThreadAgentFailed, 3*time.Second)

	// WaitAgentReady should return error for failed agent.
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := pool.WaitAgentReady(waitCtx, threadID, profile.ID); err == nil {
		t.Fatal("expected WaitAgentReady to fail for failed agent")
	}
}

func TestThreadPoolSendMessageNoSession(t *testing.T) {
	pool := NewThreadSessionPool(nil, nil, nil, "")

	if err := pool.SendMessage(context.Background(), 1, "nonexistent", "hello"); err == nil {
		t.Fatal("expected error when sending to nonexistent session")
	}
}

func TestThreadPoolResumeFromPaused(t *testing.T) {
	server := &fakeACPServer{replyText: "resumed reply"}
	profile := newTestProfile("agent-resume")

	store := newThreadSessionPoolTestStore(t)
	bus := membus.NewBus()
	dataDir := t.TempDir()
	ctx := context.Background()

	pool := NewThreadSessionPool(store, bus, &mockRegistry{
		profiles: map[string]*core.AgentProfile{profile.ID: profile},
	}, dataDir)
	pool.bootstrapFn = testBootstrapFn(server, "session-resume")
	defer pool.Close()

	threadID, _ := store.CreateThread(ctx, &core.Thread{
		Title: "Resume Thread", OwnerID: "owner-1", Status: core.ThreadActive,
	})

	// Pre-create a paused member with progress summary.
	pausedMember := &core.ThreadMember{
		ThreadID:       threadID,
		Kind:           core.ThreadMemberKindAgent,
		UserID:         profile.ID,
		AgentProfileID: profile.ID,
		Role:           "agent",
		Status:         core.ThreadAgentPaused,
		AgentData: map[string]any{
			"progress_summary": "Previously reviewed auth module.",
		},
	}
	memberID, err := store.AddThreadMember(ctx, pausedMember)
	if err != nil {
		t.Fatalf("add paused member: %v", err)
	}
	pausedMember.ID = memberID

	// InviteAgent should detect paused member and resume.
	member, err := pool.InviteAgent(ctx, threadID, profile.ID)
	if err != nil {
		t.Fatalf("InviteAgent (resume): %v", err)
	}
	if member.ID != memberID {
		t.Errorf("expected same member ID %d, got %d", memberID, member.ID)
	}

	// Wait for boot.
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.WaitAgentReady(waitCtx, threadID, profile.ID); err != nil {
		t.Fatalf("WaitAgentReady: %v", err)
	}

	// Verify boot prompt includes prior summary.
	prompts := server.getPrompts()
	if len(prompts) == 0 {
		t.Fatal("expected boot prompt")
	}
	foundPrior := false
	for _, p := range prompts {
		if strings.Contains(p, "Previously reviewed auth module.") {
			foundPrior = true
			break
		}
	}
	if !foundPrior {
		t.Errorf("boot prompt should contain prior summary, got: %v", prompts)
	}

	// Should be active now.
	waitForAgentStatus(t, store, threadID, profile.ID, core.ThreadAgentActive, 2*time.Second)
}

func TestThreadPoolCleanupThread(t *testing.T) {
	server := &fakeACPServer{replyText: "cleanup reply"}
	p1 := newTestProfile("agent-clean-1")
	p2 := newTestProfile("agent-clean-2")

	store := newThreadSessionPoolTestStore(t)
	bus := membus.NewBus()
	dataDir := t.TempDir()
	ctx := context.Background()

	pool := NewThreadSessionPool(store, bus, &mockRegistry{
		profiles: map[string]*core.AgentProfile{
			p1.ID: p1,
			p2.ID: p2,
		},
	}, dataDir)
	pool.bootstrapFn = testBootstrapFn(server, "session-cleanup")
	defer pool.Close()

	threadID, _ := store.CreateThread(ctx, &core.Thread{
		Title: "Cleanup Thread", OwnerID: "owner-1", Status: core.ThreadActive,
	})

	// Invite two agents.
	for _, p := range []*core.AgentProfile{p1, p2} {
		if _, err := pool.InviteAgent(ctx, threadID, p.ID); err != nil {
			t.Fatalf("InviteAgent(%s): %v", p.ID, err)
		}
	}

	// Wait for both.
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, p := range []*core.AgentProfile{p1, p2} {
		if err := pool.WaitAgentReady(waitCtx, threadID, p.ID); err != nil {
			t.Fatalf("WaitAgentReady(%s): %v", p.ID, err)
		}
	}

	if ids := pool.ActiveAgentProfileIDs(threadID); len(ids) != 2 {
		t.Fatalf("expected 2 active agents, got %d", len(ids))
	}

	// CleanupThread should close all sessions.
	if err := pool.CleanupThread(ctx, threadID); err != nil {
		t.Fatalf("CleanupThread: %v", err)
	}

	if ids := pool.ActiveAgentProfileIDs(threadID); len(ids) != 0 {
		t.Errorf("expected 0 active agents after cleanup, got %d", len(ids))
	}
}

func TestThreadPoolInviteAlreadyActive(t *testing.T) {
	server := &fakeACPServer{replyText: "active reply"}
	profile := newTestProfile("agent-active")

	store := newThreadSessionPoolTestStore(t)
	bus := membus.NewBus()
	dataDir := t.TempDir()
	ctx := context.Background()

	pool := NewThreadSessionPool(store, bus, &mockRegistry{
		profiles: map[string]*core.AgentProfile{profile.ID: profile},
	}, dataDir)
	pool.bootstrapFn = testBootstrapFn(server, "session-active")
	defer pool.Close()

	threadID, _ := store.CreateThread(ctx, &core.Thread{
		Title: "Active Thread", OwnerID: "owner-1", Status: core.ThreadActive,
	})

	// First invite.
	m1, err := pool.InviteAgent(ctx, threadID, profile.ID)
	if err != nil {
		t.Fatalf("InviteAgent(1): %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.WaitAgentReady(waitCtx, threadID, profile.ID); err != nil {
		t.Fatalf("WaitAgentReady: %v", err)
	}

	// Second invite should return the same member (already active).
	m2, err := pool.InviteAgent(ctx, threadID, profile.ID)
	if err != nil {
		t.Fatalf("InviteAgent(2): %v", err)
	}
	if m2.ID != m1.ID {
		t.Errorf("expected same member ID, got %d vs %d", m1.ID, m2.ID)
	}
}

func TestThreadPoolContextBudgetWarning(t *testing.T) {
	server := &fakeACPServer{replyText: "budget reply"}
	profile := newTestProfile("agent-budget")
	profile.Session.MaxContextTokens = 500 // total budget: 500 tokens
	profile.Session.ContextWarnRatio = 0.5 // warn at 50% = 250 tokens

	store := newThreadSessionPoolTestStore(t)
	bus := membus.NewBus()
	dataDir := t.TempDir()
	ctx := context.Background()

	// Subscribe to thread message events for budget warning.
	sub := bus.Subscribe(core.SubscribeOpts{
		Types:      []core.EventType{core.EventThreadMessage},
		BufferSize: 16,
	})

	pool := NewThreadSessionPool(store, bus, &mockRegistry{
		profiles: map[string]*core.AgentProfile{profile.ID: profile},
	}, dataDir)
	pool.bootstrapFn = testBootstrapFn(server, "session-budget")
	defer pool.Close()

	threadID, _ := store.CreateThread(ctx, &core.Thread{
		Title: "Budget Thread", OwnerID: "owner-1", Status: core.ThreadActive,
	})

	if _, err := pool.InviteAgent(ctx, threadID, profile.ID); err != nil {
		t.Fatalf("InviteAgent: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.WaitAgentReady(waitCtx, threadID, profile.ID); err != nil {
		t.Fatalf("WaitAgentReady: %v", err)
	}

	// Each SendMessage adds 100 input + 50 output = 150 tokens.
	// After 2 messages: 300 total > 250 threshold → warning should fire.
	for i := 0; i < 2; i++ {
		if err := pool.SendMessage(ctx, threadID, profile.ID, fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("SendMessage[%d]: %v", i, err)
		}
	}

	// Check for warning event.
	var warningFound bool
	checkCtx, checkCancel := context.WithTimeout(ctx, 2*time.Second)
	defer checkCancel()
	for {
		select {
		case ev := <-sub.C:
			if ev.Data["type"] == "system_warning" {
				msg, _ := ev.Data["message"].(string)
				if strings.Contains(msg, "context budget") {
					warningFound = true
				}
			}
		case <-checkCtx.Done():
			goto done
		}
	}
done:
	if !warningFound {
		t.Error("expected context budget warning event")
	}
}
