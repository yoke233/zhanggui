package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	membus "github.com/yoke233/zhanggui/internal/adapters/events/memory"
	"github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	chatapp "github.com/yoke233/zhanggui/internal/application/chat"
	flowapp "github.com/yoke233/zhanggui/internal/application/flow"
	"github.com/yoke233/zhanggui/internal/core"
)

type stubLeadChatService struct {
	listResp     []chatapp.SessionSummary
	detailResp   *chatapp.SessionDetail
	detailErr    error
	startResp    *chatapp.AcceptedResponse
	startErr     error
	lastStartReq chatapp.Request
}

func (s *stubLeadChatService) Chat(context.Context, chatapp.Request) (*chatapp.Response, error) {
	return &chatapp.Response{SessionID: "s-1", Reply: "ok"}, nil
}

func (s *stubLeadChatService) StartChat(_ context.Context, req chatapp.Request) (*chatapp.AcceptedResponse, error) {
	s.lastStartReq = req
	if s.startResp != nil || s.startErr != nil {
		return s.startResp, s.startErr
	}
	return &chatapp.AcceptedResponse{SessionID: "s-1", WSPath: "/api/ws?session_id=s-1&types=chat.output"}, nil
}

func (s *stubLeadChatService) ListSessions(context.Context) ([]chatapp.SessionSummary, error) {
	return s.listResp, nil
}

func (s *stubLeadChatService) GetSession(context.Context, string) (*chatapp.SessionDetail, error) {
	return s.detailResp, s.detailErr
}

func (s *stubLeadChatService) SetConfigOption(context.Context, string, string, string) ([]chatapp.ConfigOption, error) {
	return nil, nil
}
func (s *stubLeadChatService) SetSessionMode(context.Context, string, string) (*chatapp.SessionModeState, error) {
	return nil, nil
}
func (s *stubLeadChatService) ResolvePermission(string, string, bool) error { return nil }
func (s *stubLeadChatService) CancelChat(string) error                      { return nil }
func (s *stubLeadChatService) RenameSession(string, string) error           { return nil }
func (s *stubLeadChatService) ArchiveSession(string, bool) error            { return nil }
func (s *stubLeadChatService) CloseSession(string)                          {}
func (s *stubLeadChatService) DeleteSession(string)                         {}
func (s *stubLeadChatService) IsSessionAlive(string) bool {
	return false
}
func (s *stubLeadChatService) IsSessionRunning(string) bool {
	return false
}

func (s *stubLeadChatService) CancelPending(string) bool { return false }
func (s *stubLeadChatService) SubmitCode(context.Context, string, string) (*chatapp.GitStats, error) {
	return &chatapp.GitStats{HeadSHA: "abc123"}, nil
}

func (s *stubLeadChatService) CreatePR(context.Context, string, string, string) (*chatapp.GitStats, error) {
	return nil, nil
}

func (s *stubLeadChatService) RefreshPR(context.Context, string) (*chatapp.GitStats, error) {
	return nil, nil
}

func TestChatRoutes_ListSessions(t *testing.T) {
	svc := &stubLeadChatService{
		listResp: []chatapp.SessionSummary{
			{
				SessionID:    "acp-session-1",
				Title:        "历史会话",
				Status:       "alive",
				CreatedAt:    time.Now().UTC(),
				UpdatedAt:    time.Now().UTC(),
				MessageCount: 2,
			},
		},
	}

	r := chi.NewRouter()
	registerChatRoutes(r, &Handler{lead: svc})

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []chatapp.SessionSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "acp-session-1" {
		t.Fatalf("unexpected sessions: %+v", got)
	}
}

func TestChatRoutes_GetSession_NotFound(t *testing.T) {
	svc := &stubLeadChatService{detailErr: core.ErrNotFound}
	r := chi.NewRouter()
	registerChatRoutes(r, &Handler{lead: svc})

	req := httptest.NewRequest(http.MethodGet, "/chat/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestChatRoutes_SendMessage_Deprecated(t *testing.T) {
	svc := &stubLeadChatService{}
	r := chi.NewRouter()
	registerChatRoutes(r, &Handler{lead: svc})

	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusGone)
	}
	if svc.lastStartReq.Message != "" {
		t.Fatalf("unexpected ws start call: %+v", svc.lastStartReq)
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["code"] != "CHAT_HTTP_DEPRECATED" {
		t.Fatalf("unexpected code: %+v", got)
	}
}

func setupAPIWithLead(t *testing.T, lead LeadChatService) (*Handler, *httptest.Server) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "chat-test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	bus := membus.NewBus()
	executor := func(_ context.Context, step *core.Action, exec *core.Run) error {
		return nil
	}
	eng := flowapp.New(store, bus, executor, flowapp.WithConcurrency(2))
	h := NewHandler(store, bus, eng, WithLeadAgent(lead))
	r := chi.NewRouter()
	h.Register(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return h, ts
}
