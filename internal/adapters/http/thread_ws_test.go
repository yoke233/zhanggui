package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yoke233/zhanggui/internal/core"
)

type stubThreadAgentRuntime struct {
	mu               sync.Mutex
	activeProfileIDs []string
	inviteCalls      []stubThreadSendCall
	inviteErrs       map[string]error
	waitCalls        []stubThreadSendCall
	waitErrs         map[string]error
	sendCalls        []stubThreadSendCall
	sendErr          error
	promptCalls      []stubThreadSendCall
	promptReplies    map[string]string
	promptErrs       map[string]error
	promptErr        error
	cleanupCalls     []int64
	cleanupErr       error
}

type stubThreadSendCall struct {
	threadID  int64
	profileID string
	message   string
}

func (s *stubThreadAgentRuntime) InviteAgent(_ context.Context, threadID int64, profileID string) (*core.ThreadMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.inviteErrs[profileID]; err != nil {
		return nil, err
	}
	s.inviteCalls = append(s.inviteCalls, stubThreadSendCall{
		threadID:  threadID,
		profileID: profileID,
	})
	found := false
	for _, id := range s.activeProfileIDs {
		if id == profileID {
			found = true
			break
		}
	}
	if !found {
		s.activeProfileIDs = append(s.activeProfileIDs, profileID)
	}
	return &core.ThreadMember{
		ThreadID:       threadID,
		Kind:           core.ThreadMemberKindAgent,
		UserID:         profileID,
		AgentProfileID: profileID,
		Role:           core.ThreadMemberKindAgent,
		Status:         core.ThreadAgentJoining,
	}, nil
}

func (s *stubThreadAgentRuntime) WaitAgentReady(_ context.Context, threadID int64, profileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.waitErrs[profileID]; err != nil {
		return err
	}
	s.waitCalls = append(s.waitCalls, stubThreadSendCall{
		threadID:  threadID,
		profileID: profileID,
	})
	return nil
}

func (s *stubThreadAgentRuntime) PromptAgent(_ context.Context, threadID int64, profileID string, message string) (*core.ThreadAgentPromptResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.promptErrs[profileID]; err != nil {
		return nil, err
	}
	if s.promptErr != nil {
		return nil, s.promptErr
	}
	s.promptCalls = append(s.promptCalls, stubThreadSendCall{
		threadID:  threadID,
		profileID: profileID,
		message:   message,
	})
	return &core.ThreadAgentPromptResult{
		Content: s.promptReplies[profileID],
	}, nil
}

func (s *stubThreadAgentRuntime) SendMessage(_ context.Context, threadID int64, profileID string, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sendCalls = append(s.sendCalls, stubThreadSendCall{
		threadID:  threadID,
		profileID: profileID,
		message:   message,
	})
	return nil
}

func (s *stubThreadAgentRuntime) RemoveAgent(context.Context, int64, int64) error {
	return nil
}

func (s *stubThreadAgentRuntime) CleanupThread(_ context.Context, threadID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cleanupErr != nil {
		return s.cleanupErr
	}
	s.cleanupCalls = append(s.cleanupCalls, threadID)
	return nil
}

func (s *stubThreadAgentRuntime) ActiveAgentProfileIDs(threadID int64) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.activeProfileIDs))
	for _, profileID := range s.activeProfileIDs {
		out = append(out, profileID)
	}
	_ = threadID
	return out
}

func (s *stubThreadAgentRuntime) snapshotPromptCalls() []stubThreadSendCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]stubThreadSendCall, len(s.promptCalls))
	copy(out, s.promptCalls)
	return out
}

func (s *stubThreadAgentRuntime) snapshotWaitCalls() []stubThreadSendCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]stubThreadSendCall, len(s.waitCalls))
	copy(out, s.waitCalls)
	return out
}

func (s *stubThreadAgentRuntime) snapshotInviteCalls() []stubThreadSendCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]stubThreadSendCall, len(s.inviteCalls))
	copy(out, s.inviteCalls)
	return out
}

func (s *stubThreadAgentRuntime) snapshotSendCalls() []stubThreadSendCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]stubThreadSendCall, len(s.sendCalls))
	copy(out, s.sendCalls)
	return out
}

func waitForThreadCondition(t *testing.T, timeout time.Duration, check func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(10 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatal(lastErr)
	}
	t.Fatalf("condition not met within %s", timeout)
}

type stubThreadAgentRegistry struct {
	profiles map[string]*core.AgentProfile
}

func (s *stubThreadAgentRegistry) GetProfile(_ context.Context, id string) (*core.AgentProfile, error) {
	return s.ResolveByID(context.Background(), id)
}

func (s *stubThreadAgentRegistry) ListProfiles(_ context.Context) ([]*core.AgentProfile, error) {
	out := make([]*core.AgentProfile, 0, len(s.profiles))
	for _, profile := range s.profiles {
		out = append(out, profile)
	}
	return out, nil
}

func (s *stubThreadAgentRegistry) CreateProfile(context.Context, *core.AgentProfile) error {
	return nil
}

func (s *stubThreadAgentRegistry) UpdateProfile(context.Context, *core.AgentProfile) error {
	return nil
}

func (s *stubThreadAgentRegistry) DeleteProfile(context.Context, string) error {
	return nil
}

func (s *stubThreadAgentRegistry) ResolveForAction(context.Context, *core.Action) (*core.AgentProfile, error) {
	return nil, core.ErrProfileNotFound
}

func (s *stubThreadAgentRegistry) ResolveByID(_ context.Context, id string) (*core.AgentProfile, error) {
	profile, ok := s.profiles[id]
	if !ok {
		return nil, core.ErrProfileNotFound
	}
	return profile, nil
}

func TestAPI_WebSocket_ThreadSend(t *testing.T) {
	_, ts := setupAPI(t)

	// Create a thread first.
	resp, err := post(ts, "/threads", map[string]any{
		"title":    "ws-test-thread",
		"owner_id": "user-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Connect WebSocket.
	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send thread.send message.
	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-t1",
			"thread_id":  thread.ID,
			"message":    "hello thread",
			"sender_id":  "user-1",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read thread.ack response.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
		Data struct {
			RequestID string `json:"request_id"`
			ThreadID  int64  `json:"thread_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}
	if ack.Data.RequestID != "req-t1" {
		t.Fatalf("request_id = %q, want req-t1", ack.Data.RequestID)
	}
	if ack.Data.ThreadID != thread.ID {
		t.Fatalf("thread_id = %d, want %d", ack.Data.ThreadID, thread.ID)
	}
	if ack.Data.Status != "accepted" {
		t.Fatalf("status = %q, want accepted", ack.Data.Status)
	}
}

func TestAPI_WebSocket_ThreadSend_TargetAgent(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a", "worker-b"}}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title":    "ws-target-thread",
		"owner_id": "user-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id":      "req-target",
			"thread_id":       thread.ID,
			"message":         "@worker-a 请处理这个问题",
			"sender_id":       "user-1",
			"target_agent_id": "worker-a",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
		Data struct {
			RequestID string `json:"request_id"`
			ThreadID  int64  `json:"thread_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	time.Sleep(100 * time.Millisecond)
	if len(threadPool.sendCalls) != 1 {
		t.Fatalf("send calls = %d, want 1", len(threadPool.sendCalls))
	}
	if threadPool.sendCalls[0].profileID != "worker-a" {
		t.Fatalf("profile_id = %q, want worker-a", threadPool.sendCalls[0].profileID)
	}
	if !strings.Contains(threadPool.sendCalls[0].message, "@worker-a 请处理这个问题") {
		t.Fatalf("message = %q, want original mention content", threadPool.sendCalls[0].message)
	}
	if !strings.Contains(threadPool.sendCalls[0].message, "已经被 thread runtime 路由给你") {
		t.Fatalf("message = %q, want routed dispatch prompt", threadPool.sendCalls[0].message)
	}
	if strings.Contains(threadPool.sendCalls[0].message, "明确 @了你") {
		t.Fatalf("message = %q, should not hardcode explicit mention routing", threadPool.sendCalls[0].message)
	}

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	if got := msgs[0].Metadata["target_agent_id"]; got != "worker-a" {
		t.Fatalf("message metadata target_agent_id = %v, want worker-a", got)
	}
}

func TestAPI_WebSocket_ThreadSend_TargetAgentIDs(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{
		activeProfileIDs: []string{"worker-a", "worker-b", "worker-c"},
	}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title":    "ws-targets-thread",
		"owner_id": "user-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id":       "req-targets",
			"thread_id":        thread.ID,
			"message":          "请你们一起评审这个方案",
			"sender_id":        "user-1",
			"target_agent_ids": []string{"worker-a", "worker-b"},
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	time.Sleep(100 * time.Millisecond)
	if len(threadPool.sendCalls) != 2 {
		t.Fatalf("send calls = %d, want 2", len(threadPool.sendCalls))
	}
	gotProfiles := map[string]struct{}{}
	for _, call := range threadPool.sendCalls {
		gotProfiles[call.profileID] = struct{}{}
	}
	if _, ok := gotProfiles["worker-a"]; !ok {
		t.Fatalf("profile_ids = %+v, want worker-a + worker-b", threadPool.sendCalls)
	}
	if _, ok := gotProfiles["worker-b"]; !ok {
		t.Fatalf("profile_ids = %+v, want worker-a + worker-b", threadPool.sendCalls)
	}

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	targets, ok := msgs[0].Metadata["target_agent_ids"].([]any)
	if !ok || len(targets) != 2 {
		t.Fatalf("message metadata target_agent_ids = %#v, want 2 items", msgs[0].Metadata["target_agent_ids"])
	}
}

func TestAPI_WebSocket_ThreadSend_DefaultModeDoesNotBroadcastToAgents(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a", "worker-b"}}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-mention-only-thread",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-default",
			"thread_id":  thread.ID,
			"message":    "普通讨论消息",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	time.Sleep(100 * time.Millisecond)
	if len(threadPool.sendCalls) != 0 {
		t.Fatalf("send calls = %d, want 0 when thread is in mention_only mode", len(threadPool.sendCalls))
	}
}

func TestAPI_WebSocket_ThreadSend_BroadcastModeRoutesToAllActiveAgents(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a", "worker-b"}}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-broadcast-thread",
		"metadata": map[string]any{
			"agent_routing_mode": "broadcast",
		},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-broadcast",
			"thread_id":  thread.ID,
			"message":    "请大家同步一下",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	time.Sleep(100 * time.Millisecond)
	if len(threadPool.sendCalls) != 2 {
		t.Fatalf("send calls = %d, want 2 in broadcast mode", len(threadPool.sendCalls))
	}
}

func TestAPI_WebSocket_ThreadSend_AutoModeRoutesToBestMatchingAgent(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a", "worker-b"}}
	h.threadPool = threadPool
	h.registry = &stubThreadAgentRegistry{
		profiles: map[string]*core.AgentProfile{
			"worker-a": {
				ID:           "worker-a",
				Name:         "frontend specialist",
				Capabilities: []string{"react", "ui"},
				Skills:       []string{"tailwind"},
				Role:         "worker",
			},
			"worker-b": {
				ID:           "worker-b",
				Name:         "backend specialist",
				Capabilities: []string{"go", "api"},
				Skills:       []string{"sql"},
				Role:         "worker",
			},
		},
	}

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-auto-thread",
		"metadata": map[string]any{
			"agent_routing_mode": "auto",
		},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-auto",
			"thread_id":  thread.ID,
			"message":    "请负责 react 和 ui 的同学看一下这个页面",
			"sender_id":  "user-1",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	time.Sleep(100 * time.Millisecond)
	if len(threadPool.sendCalls) != 1 {
		t.Fatalf("send calls = %d, want 1", len(threadPool.sendCalls))
	}
	if threadPool.sendCalls[0].profileID != "worker-a" {
		t.Fatalf("profile_id = %q, want worker-a", threadPool.sendCalls[0].profileID)
	}
	if !strings.Contains(threadPool.sendCalls[0].message, "不要因为消息文本里没有 @你") {
		t.Fatalf("message = %q, want non-mention dispatch instruction", threadPool.sendCalls[0].message)
	}

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	autoRoutedTo, ok := msgs[0].Metadata["auto_routed_to"].([]any)
	if !ok || len(autoRoutedTo) != 1 || autoRoutedTo[0] != "worker-a" {
		t.Fatalf("message metadata auto_routed_to = %#v, want [worker-a]", msgs[0].Metadata["auto_routed_to"])
	}
}

func TestAPI_WebSocket_ThreadSend_ConcurrentMeetingAggregatesReplies(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{
		activeProfileIDs: []string{"worker-a", "worker-b"},
		promptReplies: map[string]string{
			"worker-a": "前端建议：先检查组件状态流。",
			"worker-b": "后端建议：确认接口返回是否稳定。",
		},
	}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-concurrent-thread",
		"metadata": map[string]any{
			"agent_routing_mode": "broadcast",
			"meeting_mode":       "concurrent",
		},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-concurrent",
			"thread_id":  thread.ID,
			"message":    "请并行分析这个问题",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	waitForThreadCondition(t, 2*time.Second, func() error {
		sendCalls := threadPool.snapshotSendCalls()
		promptCalls := threadPool.snapshotPromptCalls()
		if len(sendCalls) != 0 {
			return fmt.Errorf("send calls = %d, want 0 in concurrent meeting mode", len(sendCalls))
		}
		if len(promptCalls) != 2 {
			return fmt.Errorf("prompt calls = %d, want 2", len(promptCalls))
		}
		msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
		if err != nil {
			return err
		}
		if len(msgs) != 4 {
			return fmt.Errorf("messages = %d, want 4 (1 human + 2 agent + 1 summary)", len(msgs))
		}
		return nil
	})

	promptCalls := threadPool.snapshotPromptCalls()
	if !strings.Contains(promptCalls[0].message, "会议模式：concurrent") || !strings.Contains(promptCalls[1].message, "会议模式：concurrent") {
		t.Fatalf("unexpected concurrent prompt calls: %+v", promptCalls)
	}

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("messages = %d, want 4 (1 human + 2 agent + 1 summary)", len(msgs))
	}
	if msgs[1].Role != "agent" || msgs[2].Role != "agent" {
		t.Fatalf("expected agent messages in the middle, got roles %q and %q", msgs[1].Role, msgs[2].Role)
	}
	runID := fmt.Sprint(msgs[3].Metadata["meeting_run_id"])
	for i := 1; i <= 2; i++ {
		if got := msgs[i].Metadata["meeting_round"]; got != int64(1) && got != 1 && got != float64(1) {
			t.Fatalf("agent message %d round = %v, want 1", i, got)
		}
		if fmt.Sprint(msgs[i].Metadata["meeting_run_id"]) != runID {
			t.Fatalf("agent message %d run_id = %v, want %s", i, msgs[i].Metadata["meeting_run_id"], runID)
		}
	}
	if msgs[3].Role != "system" {
		t.Fatalf("summary role = %q, want system", msgs[3].Role)
	}
	if got := msgs[3].Metadata["meeting_mode"]; got != "concurrent" {
		t.Fatalf("summary meeting_mode = %v, want concurrent", got)
	}
	if !strings.Contains(msgs[3].Content, "并行会议已完成") {
		t.Fatalf("summary content = %q, want concurrent summary", msgs[3].Content)
	}
}

func TestAPI_WebSocket_ThreadSend_GroupChatMeetingRotatesSpeakers(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{
		activeProfileIDs: []string{"worker-a", "worker-b"},
		promptReplies: map[string]string{
			"worker-a": "第一轮我建议先确认前端状态。",
			"worker-b": "[FINAL] 第二轮我建议再检查接口契约。",
		},
	}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-group-chat-thread",
		"metadata": map[string]any{
			"agent_routing_mode": "broadcast",
			"meeting_mode":       "group_chat",
			"meeting_max_rounds": 4,
		},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-group-chat",
			"thread_id":  thread.ID,
			"message":    "请轮流讨论这个问题",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	waitForThreadCondition(t, 2*time.Second, func() error {
		waitCalls := threadPool.snapshotWaitCalls()
		if len(waitCalls) != 2 {
			return fmt.Errorf("wait calls = %d, want 2", len(waitCalls))
		}
		promptCalls := threadPool.snapshotPromptCalls()
		if len(promptCalls) != 2 {
			return fmt.Errorf("prompt calls = %d, want 2", len(promptCalls))
		}
		msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
		if err != nil {
			return err
		}
		if len(msgs) != 4 {
			return fmt.Errorf("messages = %d, want 4 (1 human + 2 rounds + 1 summary)", len(msgs))
		}
		return nil
	})
	waitCalls := threadPool.snapshotWaitCalls()
	gotWaitProfiles := []string{waitCalls[0].profileID, waitCalls[1].profileID}
	sort.Strings(gotWaitProfiles)
	if gotWaitProfiles[0] != "worker-a" || gotWaitProfiles[1] != "worker-b" {
		t.Fatalf("unexpected ready wait profiles: %+v", waitCalls)
	}
	promptCalls := threadPool.snapshotPromptCalls()
	if promptCalls[0].profileID != "worker-a" || promptCalls[1].profileID != "worker-b" {
		t.Fatalf("unexpected speaker order: %+v", promptCalls)
	}
	if !strings.Contains(promptCalls[1].message, "本次会议已有发言") {
		t.Fatalf("second prompt missing prior turns context: %q", promptCalls[1].message)
	}

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("messages = %d, want 4 (1 human + 2 rounds + 1 summary)", len(msgs))
	}
	if got := msgs[1].Metadata["meeting_round"]; got != int64(1) && got != 1 && got != float64(1) {
		t.Fatalf("round 1 metadata = %v, want 1", got)
	}
	if got := msgs[2].Metadata["meeting_round"]; got != int64(2) && got != 2 && got != float64(2) {
		t.Fatalf("round 2 metadata = %v, want 2", got)
	}
	if strings.Contains(msgs[2].Content, "[FINAL]") {
		t.Fatalf("round 2 content = %q, want FINAL marker removed", msgs[2].Content)
	}
	if msgs[3].Role != "system" {
		t.Fatalf("summary role = %q, want system", msgs[3].Role)
	}
	if got := msgs[3].Metadata["meeting_mode"]; got != "group_chat" {
		t.Fatalf("summary meeting_mode = %v, want group_chat", got)
	}
	if got := msgs[3].Metadata["meeting_selector"]; got != "round_robin" {
		t.Fatalf("summary meeting_selector = %v, want round_robin", got)
	}
	if got := msgs[3].Metadata["meeting_rounds"]; got != int64(2) && got != 2 && got != float64(2) {
		t.Fatalf("summary meeting_rounds = %v, want 2", got)
	}
	if got := msgs[3].Metadata["stop_reason"]; got != "worker-b declared final" {
		t.Fatalf("summary stop_reason = %v, want worker-b declared final", got)
	}
	if !strings.Contains(msgs[3].Content, "主持人会议已完成") {
		t.Fatalf("summary content = %q, want group chat summary", msgs[3].Content)
	}
}

func TestAPI_WebSocket_ThreadSend_GroupChatMeetingPersistsSummaryWhenNoTurns(t *testing.T) {
	h, ts := setupAPI(t)
	h.threadPool = &stubThreadAgentRuntime{
		activeProfileIDs: []string{"worker-a", "worker-b"},
		promptErr:        errors.New("agent offline"),
	}

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-group-chat-empty-thread",
		"metadata": map[string]any{
			"agent_routing_mode": "broadcast",
			"meeting_mode":       "group_chat",
		},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-group-chat-empty",
			"thread_id":  thread.ID,
			"message":    "请轮流讨论这个问题",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	waitForThreadCondition(t, 2*time.Second, func() error {
		msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
		if err != nil {
			return err
		}
		if len(msgs) != 2 {
			return fmt.Errorf("messages = %d, want 2 (1 human + 1 summary)", len(msgs))
		}
		return nil
	})

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if msgs[1].Role != "system" {
		t.Fatalf("summary role = %q, want system", msgs[1].Role)
	}
	if got := msgs[1].Metadata["meeting_rounds"]; got != int64(0) && got != 0 && got != float64(0) {
		t.Fatalf("summary meeting_rounds = %v, want 0", got)
	}
	if got := msgs[1].Metadata["stop_reason"]; got != "all speakers failed" {
		t.Fatalf("summary stop_reason = %v, want all speakers failed", got)
	}
	if !strings.Contains(msgs[1].Content, "未产生有效发言") {
		t.Fatalf("summary content = %q, want empty-turns hint", msgs[1].Content)
	}
}

func TestAPI_WebSocket_ThreadSend_GroupChatMeetingSkipsFailedSpeaker(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{
		activeProfileIDs: []string{"worker-a", "worker-b"},
		promptErrs: map[string]error{
			"worker-a": errors.New("agent offline"),
		},
		promptReplies: map[string]string{
			"worker-b": "[FINAL] 我接手并给出最终方案。",
		},
	}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-group-chat-skip-failed-thread",
		"metadata": map[string]any{
			"agent_routing_mode": "broadcast",
			"meeting_mode":       "group_chat",
			"meeting_max_rounds": 4,
		},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-group-chat-skip-failed",
			"thread_id":  thread.ID,
			"message":    "请轮流讨论这个问题",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	waitForThreadCondition(t, 2*time.Second, func() error {
		msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
		if err != nil {
			return err
		}
		if len(msgs) != 3 {
			return fmt.Errorf("messages = %d, want 3 (1 human + 1 agent + 1 summary)", len(msgs))
		}
		return nil
	})

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if msgs[1].SenderID != "worker-b" {
		t.Fatalf("agent sender = %q, want worker-b", msgs[1].SenderID)
	}
	if got := msgs[2].Metadata["stop_reason"]; got != "worker-b declared final" {
		t.Fatalf("summary stop_reason = %v, want worker-b declared final", got)
	}
	if got := msgs[2].Metadata["meeting_rounds"]; got != int64(1) && got != 1 && got != float64(1) {
		t.Fatalf("summary meeting_rounds = %v, want 1", got)
	}
}

func TestAPI_WebSocket_ThreadSend_ConcurrentMeetingDeduplicatesRecipients(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{
		activeProfileIDs: []string{"worker-a", "worker-a", "worker-b"},
		promptReplies: map[string]string{
			"worker-a": "A",
			"worker-b": "B",
		},
	}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-concurrent-dedupe-thread",
		"metadata": map[string]any{
			"agent_routing_mode": "broadcast",
			"meeting_mode":       "concurrent",
		},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-concurrent-dedupe",
			"thread_id":  thread.ID,
			"message":    "请并行分析这个问题",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	waitForThreadCondition(t, 2*time.Second, func() error {
		promptCalls := threadPool.snapshotPromptCalls()
		if len(promptCalls) != 2 {
			return fmt.Errorf("prompt calls = %d, want 2 after dedupe", len(promptCalls))
		}
		return nil
	})
}

func TestAPI_WebSocket_ThreadSend_TargetAgentUnavailable(t *testing.T) {
	h, ts := setupAPI(t)
	h.threadPool = &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a"}}

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-target-thread",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id":      "req-miss",
			"thread_id":       thread.ID,
			"message":         "@worker-z 处理一下",
			"target_agent_id": "worker-z",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var errResp struct {
		Type string `json:"type"`
		Data struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
			Error     string `json:"error"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&errResp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if errResp.Type != "thread.error" {
		t.Fatalf("type = %q, want thread.error", errResp.Type)
	}
	if errResp.Data.Code != "TARGET_AGENT_UNAVAILABLE" {
		t.Fatalf("code = %q, want TARGET_AGENT_UNAVAILABLE", errResp.Data.Code)
	}

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages = %d, want 0", len(msgs))
	}
}

func TestAPI_WebSocket_ThreadSend_TargetAgentUsesPersistedParticipantWhenRuntimeSetCold(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-target-persisted-thread",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if _, err := h.store.AddThreadMember(context.Background(), &core.ThreadMember{
		ThreadID:       thread.ID,
		Kind:           core.ThreadMemberKindAgent,
		UserID:         "worker-a",
		AgentProfileID: "worker-a",
		Role:           "agent",
		Status:         core.ThreadAgentActive,
	}); err != nil {
		t.Fatalf("add thread member: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id":      "req-target-persisted",
			"thread_id":       thread.ID,
			"message":         "请继续处理上次的任务",
			"target_agent_id": "worker-a",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	waitForThreadCondition(t, 2*time.Second, func() error {
		sendCalls := threadPool.snapshotSendCalls()
		if len(sendCalls) != 1 {
			return fmt.Errorf("send calls = %d, want 1", len(sendCalls))
		}
		if sendCalls[0].profileID != "worker-a" {
			return fmt.Errorf("profile_id = %q, want worker-a", sendCalls[0].profileID)
		}
		return nil
	})
}

func TestAPI_WebSocket_ThreadSend_InvalidThread(t *testing.T) {
	_, ts := setupAPI(t)

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send thread.send with non-existent thread_id.
	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-bad",
			"thread_id":  9999,
			"message":    "nobody home",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var errResp struct {
		Type string `json:"type"`
		Data struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
			Error     string `json:"error"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&errResp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if errResp.Type != "thread.error" {
		t.Fatalf("type = %q, want thread.error", errResp.Type)
	}
	if errResp.Data.Code != "THREAD_NOT_FOUND" {
		t.Fatalf("code = %q, want THREAD_NOT_FOUND", errResp.Data.Code)
	}
}

func TestAPI_WebSocket_ThreadSend_PersistFailureReturnsError(t *testing.T) {
	h, ts := setupAPI(t)
	h.store = &failingCreateThreadMessageStore{
		Store: h.store,
		err:   errors.New("insert failed"),
	}

	resp, err := post(ts, "/threads", map[string]any{
		"title":    "ws-test-thread",
		"owner_id": "user-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-t1",
			"thread_id":  thread.ID,
			"message":    "hello thread",
			"sender_id":  "user-1",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var errResp struct {
		Type string `json:"type"`
		Data struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
			Error     string `json:"error"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&errResp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if errResp.Type != "thread.error" {
		t.Fatalf("type = %q, want thread.error", errResp.Type)
	}
	if errResp.Data.Code != "THREAD_SEND_FAILED" {
		t.Fatalf("code = %q, want THREAD_SEND_FAILED", errResp.Data.Code)
	}
	if errResp.Data.RequestID != "req-t1" {
		t.Fatalf("request_id = %q, want req-t1", errResp.Data.RequestID)
	}
}

func TestAPI_WebSocket_SubscribeThread(t *testing.T) {
	h, ts := setupAPI(t)

	// Create a thread.
	resp, _ := post(ts, "/threads", map[string]any{"title": "sub-test-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	// Connect WebSocket.
	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Subscribe to thread.
	if err := conn.WriteJSON(map[string]any{
		"type": "subscribe_thread",
		"data": map[string]any{"thread_id": thread.ID},
	}); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}

	// Read subscribe ack.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var subAck struct {
		Type string `json:"type"`
		Data struct {
			ThreadID int64  `json:"thread_id"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&subAck); err != nil {
		t.Fatalf("read subscribe ack: %v", err)
	}
	if subAck.Type != "thread.subscribed" {
		t.Fatalf("type = %q, want thread.subscribed", subAck.Type)
	}
	if subAck.Data.ThreadID != thread.ID {
		t.Fatalf("thread_id = %d, want %d", subAck.Data.ThreadID, thread.ID)
	}

	// Publish a thread.message event.
	h.bus.Publish(context.Background(), core.Event{
		Type:      core.EventThreadMessage,
		Data:      map[string]any{"thread_id": thread.ID, "message": "hello from bus"},
		Timestamp: time.Now().UTC(),
	})

	// Should receive the thread event.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ev core.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Type != core.EventThreadMessage {
		t.Fatalf("event type = %q, want thread.message", ev.Type)
	}
}

func TestAPI_WebSocket_UnsubscribeThread(t *testing.T) {
	h, ts := setupAPI(t)

	// Create a thread.
	resp, _ := post(ts, "/threads", map[string]any{"title": "unsub-test-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	// Connect WebSocket.
	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Subscribe to thread.
	conn.WriteJSON(map[string]any{
		"type": "subscribe_thread",
		"data": map[string]any{"thread_id": thread.ID},
	})
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack map[string]any
	conn.ReadJSON(&ack) // consume subscribe ack

	// Unsubscribe from thread.
	if err := conn.WriteJSON(map[string]any{
		"type": "unsubscribe_thread",
		"data": map[string]any{"thread_id": thread.ID},
	}); err != nil {
		t.Fatalf("write unsubscribe: %v", err)
	}

	// Read unsubscribe ack.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var unsubAck struct {
		Type string `json:"type"`
		Data struct {
			ThreadID int64  `json:"thread_id"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&unsubAck); err != nil {
		t.Fatalf("read unsubscribe ack: %v", err)
	}
	if unsubAck.Type != "thread.unsubscribed" {
		t.Fatalf("type = %q, want thread.unsubscribed", unsubAck.Type)
	}

	// Publish thread event — should NOT be received (unsubscribed).
	h.bus.Publish(context.Background(), core.Event{
		Type:      core.EventThreadMessage,
		Data:      map[string]any{"thread_id": thread.ID, "message": "should not arrive"},
		Timestamp: time.Now().UTC(),
	})

	// Publish a non-thread event to confirm connection is still alive.
	h.bus.Publish(context.Background(), core.Event{
		Type:       core.EventWorkItemStarted,
		WorkItemID: 42,
		Timestamp:  time.Now().UTC(),
	})

	// Should receive the issue event but NOT the thread event.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ev core.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Type == core.EventThreadMessage {
		t.Fatal("received thread.message after unsubscribing")
	}
	if ev.Type != core.EventWorkItemStarted {
		t.Fatalf("expected issue.started, got %s", ev.Type)
	}
}
