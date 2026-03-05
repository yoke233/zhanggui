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

	"github.com/yoke233/ai-workflow/internal/core"
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

	assistant := &stubChatAssistant{
		response: ChatAssistantResponse{
			Reply: "收到，开始处理。",
		},
	}
	srv := NewServer(Config{
		Store:         store,
		ChatAssistant: assistant,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message": "请帮我拆解一个 OAuth 登录改造计划",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	createResp, err := http.Post(
		ts.URL+"/api/v3/projects/proj-chat-api/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v3/projects/{pid}/chat: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", createResp.StatusCode)
	}

	var created createChatSessionResponse
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create chat response: %v", err)
	}
	if created.SessionID == "" {
		t.Fatal("expected non-empty session_id")
	}
	if created.Status != "accepted" {
		t.Fatalf("expected status accepted, got %q", created.Status)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		session, err := store.GetChatSession(created.SessionID)
		if err != nil {
			t.Fatalf("reload chat session: %v", err)
		}
		if len(session.Messages) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("chat session did not complete in time, messages=%d", len(session.Messages))
		}
		time.Sleep(20 * time.Millisecond)
	}

	getResp, err := http.Get(ts.URL + "/api/v3/projects/proj-chat-api/chat/" + created.SessionID)
	if err != nil {
		t.Fatalf("GET /api/v3/projects/{pid}/chat/{sid}: %v", err)
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

func TestListChatSessions(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-list",
		Name:     "chat-list",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-list"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	otherProject := core.Project{
		ID:       "proj-chat-list-other",
		Name:     "chat-list-other",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-list-other"),
	}
	if err := store.CreateProject(&otherProject); err != nil {
		t.Fatalf("seed other project: %v", err)
	}

	mainSessionA := &core.ChatSession{
		ID:        "chat-20260302-list01",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "list-a"},
		},
	}
	if err := store.CreateChatSession(mainSessionA); err != nil {
		t.Fatalf("seed main session A: %v", err)
	}
	mainSessionB := &core.ChatSession{
		ID:        "chat-20260302-list02",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "list-b"},
			{Role: "assistant", Content: "ok"},
		},
	}
	if err := store.CreateChatSession(mainSessionB); err != nil {
		t.Fatalf("seed main session B: %v", err)
	}
	otherSession := &core.ChatSession{
		ID:        "chat-20260302-list03",
		ProjectID: otherProject.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "should-not-appear"},
		},
	}
	if err := store.CreateChatSession(otherSession); err != nil {
		t.Fatalf("seed other project session: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v3/projects/proj-chat-list/chat")
	if err != nil {
		t.Fatalf("GET /api/v3/projects/{pid}/chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var sessions []core.ChatSession
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode chat session list response: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	wantByID := map[string]bool{
		mainSessionA.ID: false,
		mainSessionB.ID: false,
	}
	for _, session := range sessions {
		if session.ProjectID != project.ID {
			t.Fatalf("expected project id %s, got %s", project.ID, session.ProjectID)
		}
		if _, ok := wantByID[session.ID]; !ok {
			t.Fatalf("unexpected session id in list: %s", session.ID)
		}
		wantByID[session.ID] = true
	}
	for id, hit := range wantByID {
		if !hit {
			t.Fatalf("expected session %s in list response", id)
		}
	}
}

func TestListChatSessionsProjectNotFound(t *testing.T) {
	store := newTestStore(t)
	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v3/projects/proj-chat-list-missing/chat")
	if err != nil {
		t.Fatalf("GET /api/v3/projects/{pid}/chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if apiErr.Code != "PROJECT_NOT_FOUND" {
		t.Fatalf("expected code PROJECT_NOT_FOUND, got %s", apiErr.Code)
	}
}

func TestListChatSessionEvents(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-events",
		Name:     "chat-events",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-events"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	session := &core.ChatSession{
		ID:        "chat-20260303-events",
		ProjectID: project.ID,
		Messages: []core.ChatMessage{
			{Role: "user", Content: "执行任务"},
		},
	}
	if err := store.CreateChatSession(session); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}
	runEventRecorder, ok := store.(interface {
		AppendChatRunEvent(event core.ChatRunEvent) error
	})
	if !ok {
		t.Fatal("store does not support AppendChatRunEvent")
	}
	if err := runEventRecorder.AppendChatRunEvent(core.ChatRunEvent{
		SessionID:  session.ID,
		ProjectID:  project.ID,
		EventType:  "chat_run_update",
		UpdateType: "tool_call",
		Payload: map[string]any{
			"session_id": session.ID,
			"acp": map[string]any{
				"sessionUpdate": "tool_call",
				"title":         "Terminal",
				"status":        "pending",
			},
		},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed chat run event: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v3/projects/proj-chat-events/chat/" + session.ID + "/events")
	if err != nil {
		t.Fatalf("GET /api/v3/projects/{pid}/chat/{sid}/events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var events []core.ChatRunEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("decode events response: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].SessionID != session.ID || events[0].ProjectID != project.ID {
		t.Fatalf("unexpected event identity: %#v", events[0])
	}
	if events[0].UpdateType != "tool_call" {
		t.Fatalf("unexpected update type: %q", events[0].UpdateType)
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
		ts.URL+"/api/v3/projects/proj-chat-required/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v3/projects/{pid}/chat: %v", err)
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
		ts.URL+"/api/v3/projects/proj-chat-delete/chat/"+session.ID,
		nil,
	)
	if err != nil {
		t.Fatalf("create delete request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /api/v3/projects/{pid}/chat/{sid}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	getResp, err := http.Get(ts.URL + "/api/v3/projects/proj-chat-delete/chat/" + session.ID)
	if err != nil {
		t.Fatalf("GET deleted session: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted session, got %d", getResp.StatusCode)
	}
}

func TestCreateChatSessionRejectsLegacyAutoCreatePlanParam(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-issue-draft",
		Name:     "chat-issue-draft",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-issue-draft"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	createIssuesCalled := false
	issueManager := &testPlanManager{
		createIssuesFn: func(_ context.Context, _ IssueCreateInput) ([]core.Issue, error) {
			createIssuesCalled = true
			return nil, nil
		},
	}

	srv := NewServer(Config{Store: store, IssueManager: issueManager})
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
		ts.URL+"/api/v3/projects/proj-chat-issue-draft/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v3/projects/{pid}/chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode api error: %v", err)
	}
	if apiErr.Code != "INVALID_JSON" {
		t.Fatalf("expected INVALID_JSON, got %s", apiErr.Code)
	}

	if createIssuesCalled {
		t.Fatal("expected no issue manager calls from /chat")
	}
}

func TestCreateChatSessionDoesNotAutoCreateIssueByDefault(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-issue-default-off",
		Name:     "chat-issue-default-off",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-issue-default-off"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	createIssuesCalled := false
	issueManager := &testPlanManager{
		createIssuesFn: func(_ context.Context, _ IssueCreateInput) ([]core.Issue, error) {
			createIssuesCalled = true
			return nil, nil
		},
	}

	assistant := &stubChatAssistant{
		response: ChatAssistantResponse{
			Reply: "已记录你的消息。",
		},
	}
	srv := NewServer(Config{
		Store:         store,
		IssueManager:  issueManager,
		ChatAssistant: assistant,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message": "默认不自动建 issue",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	resp, err := http.Post(
		ts.URL+"/api/v3/projects/proj-chat-issue-default-off/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST /api/v3/projects/{pid}/chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var created createChatSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create chat response: %v", err)
	}
	if createIssuesCalled {
		t.Fatal("expected no issue manager calls from /chat")
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
		ts.URL+"/api/v3/projects/proj-chat-role/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var created createChatSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.SessionID == "" {
		t.Fatal("expected non-empty session id")
	}

	var calls []ChatAssistantRequest
	deadline := time.Now().Add(3 * time.Second)
	for {
		calls = assistant.Calls()
		if len(calls) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected one assistant call, got %d", len(calls))
		}
		time.Sleep(20 * time.Millisecond)
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
		ts.URL+"/api/v3/projects/proj-chat-invalid-role/chat",
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
		ts.URL+"/api/v3/projects/proj-chat-continue/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST continue chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var out createChatSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.SessionID != existing.ID {
		t.Fatalf("expected same session id %q, got %q", existing.ID, out.SessionID)
	}
	if out.Status != "accepted" {
		t.Fatalf("expected status accepted, got %q", out.Status)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		updated, err := store.GetChatSession(existing.ID)
		if err != nil {
			t.Fatalf("reload chat session: %v", err)
		}
		if updated.AgentSessionID == "claude-sid-new" && len(updated.Messages) == 4 {
			if updated.Messages[2].Content != "第二轮问题" || updated.Messages[3].Content != "第二轮回复" {
				t.Fatalf("unexpected continuation message pair: %#v", updated.Messages[2:])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("chat continuation did not complete in time, got agent_session_id=%q messages=%d", updated.AgentSessionID, len(updated.Messages))
		}
		time.Sleep(30 * time.Millisecond)
	}

	calls := assistant.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected one assistant call, got %d", len(calls))
	}
	if calls[0].Role != "team_leader" {
		t.Fatalf("expected default role team_leader, got %q", calls[0].Role)
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
		ts.URL+"/api/v3/projects/proj-chat-assistant-fail/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
}

func TestCreateChatSessionAssistantUnavailableReturnsServiceUnavailable(t *testing.T) {
	store := newTestStore(t)
	project := core.Project{
		ID:       "proj-chat-assistant-missing",
		Name:     "chat-assistant-missing",
		RepoPath: filepath.Join(t.TempDir(), "repo-chat-assistant-missing"),
	}
	if err := store.CreateProject(&project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rawBody, err := json.Marshal(map[string]any{
		"message": "请回复",
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	resp, err := http.Post(
		ts.URL+"/api/v3/projects/proj-chat-assistant-missing/chat",
		"application/json",
		bytes.NewReader(rawBody),
	)
	if err != nil {
		t.Fatalf("POST chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}

	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode api error: %v", err)
	}
	if apiErr.Code != "CHAT_ASSISTANT_UNAVAILABLE" {
		t.Fatalf("expected CHAT_ASSISTANT_UNAVAILABLE, got %s", apiErr.Code)
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
