package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/secretary"
)

func TestCreateChatSessionThenGetChatSession(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-api",
		Name:     "chat-api",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-api"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message": "请帮我拆解一个 OAuth 登录改造计划",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	createResp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-chat-api/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/chat: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", createResp.StatusCode)
	}

	var created struct {
		SessionID string `json:"session_id"`
		Reply     string `json:"reply"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create chat response: %v", err)
	}
	if created.SessionID == "" {
		t.Fatal("expected non-empty session_id")
	}
	if created.Reply == "" {
		t.Fatal("expected non-empty reply")
	}

	getResp, err := http.Get(ts.URL + "/api/v1/projects/proj-chat-api/chat/" + created.SessionID)
	if err != nil {
		t.Fatalf("GET /api/v1/projects/{pid}/chat/{sid}: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	var session core.ChatSession
	if err := json.NewDecoder(getResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode chat session response: %v", err)
	}
	if session.ID != created.SessionID {
		t.Fatalf("expected session id %s, got %s", created.SessionID, session.ID)
	}
	if session.ProjectID != "proj-chat-api" {
		t.Fatalf("expected project id proj-chat-api, got %s", session.ProjectID)
	}
	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(session.Messages))
	}
	if session.Messages[0].Role != "user" {
		t.Fatalf("expected first message role=user, got %s", session.Messages[0].Role)
	}
	if session.Messages[1].Role != "assistant" {
		t.Fatalf("expected second message role=assistant, got %s", session.Messages[1].Role)
	}
}

func TestCreateChatSessionRequiresMessage(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-required",
		Name:     "chat-required",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-required"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message": "   ",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-chat-required/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode api error: %v", err)
	}
	if apiErr.Code != "MESSAGE_REQUIRED" {
		t.Fatalf("expected code MESSAGE_REQUIRED, got %s", apiErr.Code)
	}
}

func TestDeleteChatSession(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-delete",
		Name:     "chat-delete",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-delete"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	session := &core.ChatSession{
		ID:        "chat-20260301-delete01",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "准备删除会话"},
		},
	}
	if err := store.CreateChatSession(session); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(
		http.MethodDelete,
		ts.URL+"/api/v1/projects/proj-chat-delete/chat/"+session.ID,
		nil,
	)
	if err != nil {
		t.Fatalf("create delete request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /api/v1/projects/{pid}/chat/{sid}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	getResp, err := http.Get(ts.URL + "/api/v1/projects/proj-chat-delete/chat/" + session.ID)
	if err != nil {
		t.Fatalf("GET deleted session: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted session, got %d", getResp.StatusCode)
	}
}

func TestCreateChatSessionRejectsAutoCreatePlanParam(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-plan-draft",
		Name:     "chat-plan-draft",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-plan-draft"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	createDraftCalled := false
	planManager := &testPlanManager{
		createDraftFn: func(_ context.Context, _ secretary.CreateDraftInput) (*core.TaskPlan, error) {
			createDraftCalled = true
			return nil, nil
		},
	}

	srv := NewServer(Config{Store: store, PlanManager: planManager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message":          "请拆分一个认证系统改造计划",
		"auto_create_plan": true,
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-chat-plan-draft/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if createDraftCalled {
		t.Fatal("expected no plan manager calls from /chat")
	}
}

func TestCreateChatSessionDoesNotAutoCreatePlanByDefault(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-plan-default-off",
		Name:     "chat-plan-default-off",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-plan-default-off"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	createDraftCalled := false
	planManager := &testPlanManager{
		createDraftFn: func(_ context.Context, _ secretary.CreateDraftInput) (*core.TaskPlan, error) {
			createDraftCalled = true
			return nil, nil
		},
	}

	srv := NewServer(Config{Store: store, PlanManager: planManager})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message": "默认不自动建计划",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-chat-plan-default-off/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/projects/{pid}/chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var created createChatSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create chat response: %v", err)
	}
	if createDraftCalled {
		t.Fatal("expected no plan manager calls from /chat")
	}
}

func TestChatSessionCreateWithRole(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-role",
		Name:     "chat-role",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-role"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	assistant := &stubChatAssistant{
		response: ChatAssistantResponse{
			Reply: "收到角色上下文",
		},
	}
	srv := NewServer(Config{
		Store:         store,
		ChatAssistant: assistant,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message": "请以 reviewer 视角给建议",
		"role":    "reviewer",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-chat-role/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var created createChatSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.SessionID == "" {
		t.Fatal("expected non-empty session id")
	}

	calls := assistant.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected one assistant call, got %d", len(calls))
	}
	if calls[0].Role != "reviewer" {
		t.Fatalf("expected assistant request role reviewer, got %q", calls[0].Role)
	}
	if calls[0].Message != "请以 reviewer 视角给建议" {
		t.Fatalf("expected assistant request message forwarded, got %q", calls[0].Message)
	}
	if calls[0].WorkDir != project.RepoPath {
		t.Fatalf("expected assistant request workdir %q, got %q", project.RepoPath, calls[0].WorkDir)
	}

	session, err := store.GetChatSession(created.SessionID)
	if err != nil {
		t.Fatalf("reload chat session: %v", err)
	}
	if len(session.Messages) < 1 {
		t.Fatal("expected chat messages to be persisted")
	}
	if session.Messages[0].Role != "user" {
		t.Fatalf("expected first message role user, got %q", session.Messages[0].Role)
	}
}

func TestCreateChatSessionRejectsInvalidRole(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-invalid-role",
		Name:     "chat-invalid-role",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-invalid-role"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message": "请处理这条消息",
		"role":    "reviewer admin",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-chat-invalid-role/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode api error: %v", err)
	}
	if apiErr.Code != "INVALID_ROLE" {
		t.Fatalf("expected INVALID_ROLE, got %s", apiErr.Code)
	}
}

func TestCreateChatSessionContinuesExistingSessionWithAssistant(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-continue",
		Name:     "chat-continue",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-continue"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	existing := &core.ChatSession{
		ID:             "chat-20260302-cont",
		ProjectID:      project.ID,
		AgentSessionID: "claude-sid-old",
		Messages: []core.ChatMessage{
			{Role: "user", Content: "第一轮", Time: time.Now().UTC()},
			{Role: "assistant", Content: "第一轮回复", Time: time.Now().UTC()},
		},
	}
	if err := store.CreateChatSession(existing); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}

	assistant := &stubChatAssistant{
		response: ChatAssistantResponse{
			Reply:          "第二轮回复",
			AgentSessionID: "claude-sid-new",
		},
	}

	srv := NewServer(Config{
		Store:         store,
		ChatAssistant: assistant,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"session_id": existing.ID,
		"message":    "第二轮问题",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-chat-continue/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST continue chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out createChatSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.SessionID != existing.ID {
		t.Fatalf("expected same session id %q, got %q", existing.ID, out.SessionID)
	}
	if out.Reply != "第二轮回复" {
		t.Fatalf("expected assistant reply %q, got %q", "第二轮回复", out.Reply)
	}

	updated, err := store.GetChatSession(existing.ID)
	if err != nil {
		t.Fatalf("reload chat session: %v", err)
	}
	if updated.AgentSessionID != "claude-sid-new" {
		t.Fatalf("expected updated agent_session_id %q, got %q", "claude-sid-new", updated.AgentSessionID)
	}
	if len(updated.Messages) != 4 {
		t.Fatalf("expected 4 total messages after continuation, got %d", len(updated.Messages))
	}
	if updated.Messages[2].Content != "第二轮问题" || updated.Messages[3].Content != "第二轮回复" {
		t.Fatalf("unexpected continuation message pair: %#v", updated.Messages[2:])
	}

	calls := assistant.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected one assistant call, got %d", len(calls))
	}
	if calls[0].Role != "secretary" {
		t.Fatalf("expected default role secretary, got %q", calls[0].Role)
	}
	if calls[0].AgentSessionID != "claude-sid-old" {
		t.Fatalf("expected resume from old session id, got %q", calls[0].AgentSessionID)
	}
	if calls[0].WorkDir != project.RepoPath {
		t.Fatalf("expected assistant request workdir %q, got %q", project.RepoPath, calls[0].WorkDir)
	}
}

func TestCreateChatSessionAssistantFailureReturnsBadGateway(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-assistant-fail",
		Name:     "chat-assistant-fail",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-assistant-fail"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	assistant := &stubChatAssistant{
		err: errors.New("claude unavailable"),
	}
	srv := NewServer(Config{
		Store:         store,
		ChatAssistant: assistant,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message": "请回复",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	resp, err := http.Post(
		ts.URL+"/api/v1/projects/proj-chat-assistant-fail/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode api error: %v", err)
	}
	if apiErr.Code != "CHAT_ASSISTANT_FAILED" {
		t.Fatalf("expected CHAT_ASSISTANT_FAILED, got %s", apiErr.Code)
	}
}

type stubChatAssistant struct {
	mu       sync.Mutex
	response ChatAssistantResponse
	err      error
	calls    []ChatAssistantRequest
}

func (s *stubChatAssistant) Reply(_ context.Context, req ChatAssistantRequest) (ChatAssistantResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	if s.err != nil {
		return ChatAssistantResponse{}, s.err
	}
	return s.response, nil
}

func (s *stubChatAssistant) Calls() []ChatAssistantRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ChatAssistantRequest, len(s.calls))
	copy(out, s.calls)
	return out
}
