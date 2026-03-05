package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

type chatHandlers struct {
	store     core.Store
	assistant ChatAssistant
	publisher chatEventPublisher

	runMu     sync.Mutex
	activeRun map[string]context.CancelFunc
}

type chatSessionDeleter interface {
	DeleteChatSession(id string) error
}

type chatRunEventReader interface {
	ListChatRunEvents(sessionID string) ([]core.ChatRunEvent, error)
}

type chatEventPublisher interface {
	Publish(ctx context.Context, evt core.Event) error
}

type createChatSessionRequest struct {
	Message   string `json:"message"`
	Role      string `json:"role,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type createChatSessionResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

type cancelChatSessionResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

func registerChatRoutes(r chi.Router, store core.Store, assistant ChatAssistant, publisher chatEventPublisher) {
	h := &chatHandlers{
		store:     store,
		assistant: assistant,
		publisher: publisher,
		activeRun: make(map[string]context.CancelFunc),
	}
	r.Get("/projects/{projectID}/chat", h.listSessions)
	r.Post("/projects/{projectID}/chat", h.createSession)
	r.Post("/projects/{projectID}/chat/{sessionID}/cancel", h.cancelSession)
	r.Get("/projects/{projectID}/chat/{sessionID}/events", h.listSessionEvents)
	r.Get("/projects/{projectID}/chat/{sessionID}", h.getSession)
	r.Delete("/projects/{projectID}/chat/{sessionID}", h.deleteSession)
}

func (h *chatHandlers) listSessions(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id is required", "PROJECT_ID_REQUIRED")
		return
	}
	if _, err := h.store.GetProject(projectID); err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", projectID), "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return
	}

	sessions, err := h.store.ListChatSessions(projectID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list chat sessions", "LIST_CHAT_SESSIONS_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *chatHandlers) createSession(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id is required", "PROJECT_ID_REQUIRED")
		return
	}
	project, err := h.store.GetProject(projectID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", projectID), "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return
	}

	var req createChatSessionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}

	req.Message = strings.TrimSpace(req.Message)
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	req.SessionID = strings.TrimSpace(req.SessionID)
	if req.Message == "" {
		writeAPIError(w, http.StatusBadRequest, "message is required", "MESSAGE_REQUIRED")
		return
	}
	if req.Role == "" {
		req.Role = "team_leader"
	}
	if !isValidRoleID(req.Role) {
		writeAPIError(w, http.StatusBadRequest, "invalid role", "INVALID_ROLE")
		return
	}
	if h.assistant == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "chat assistant is not configured", "CHAT_ASSISTANT_UNAVAILABLE")
		return
	}

	isNewSession := req.SessionID == ""
	var session *core.ChatSession
	if isNewSession {
		session = &core.ChatSession{
			ID:        core.NewChatSessionID(),
			ProjectID: projectID,
		}
	} else {
		existing, err := h.store.GetChatSession(req.SessionID)
		if err != nil {
			if isNotFoundError(err) {
				writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", req.SessionID), "CHAT_SESSION_NOT_FOUND")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
			return
		}
		if existing.ProjectID != projectID {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", req.SessionID, projectID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		session = existing
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	if !h.registerSessionRun(session.ID, cancelRun) {
		cancelRun()
		writeAPIError(w, http.StatusConflict, "chat session is already running", "CHAT_SESSION_BUSY")
		return
	}

	now := time.Now().UTC()
	session.Messages = append(session.Messages, core.ChatMessage{
		Role:    "user",
		Content: req.Message,
		Time:    now,
	})

	if isNewSession {
		if err := h.store.CreateChatSession(session); err != nil {
			h.unregisterSessionRun(session.ID)
			cancelRun()
			writeAPIError(w, http.StatusInternalServerError, "failed to create chat session", "CREATE_CHAT_SESSION_FAILED")
			return
		}
	} else {
		if err := h.store.UpdateChatSession(session); err != nil {
			h.unregisterSessionRun(session.ID)
			cancelRun()
			writeAPIError(w, http.StatusInternalServerError, "failed to update chat session", "UPDATE_CHAT_SESSION_FAILED")
			return
		}
	}

	writeJSON(w, http.StatusAccepted, createChatSessionResponse{
		SessionID: session.ID,
		Status:    "accepted",
	})

	go h.executeChatTurn(runCtx, chatRunInput{
		ProjectID:      projectID,
		WorkDir:        strings.TrimSpace(project.RepoPath),
		Role:           req.Role,
		Message:        req.Message,
		SessionID:      session.ID,
		AgentSessionID: strings.TrimSpace(session.AgentSessionID),
	})
}

type chatRunInput struct {
	ProjectID      string
	WorkDir        string
	Role           string
	Message        string
	SessionID      string
	AgentSessionID string
}

func (h *chatHandlers) executeChatTurn(ctx context.Context, input chatRunInput) {
	defer h.unregisterSessionRun(input.SessionID)

	h.publishChatEvent(core.EventRunStarted, input.ProjectID, input.SessionID, input.Role, nil)

	assistantResp, err := h.assistant.Reply(ctx, ChatAssistantRequest{
		Message:        input.Message,
		Role:           input.Role,
		WorkDir:        input.WorkDir,
		AgentSessionID: input.AgentSessionID,
		ProjectID:      input.ProjectID,
		ChatSessionID:  input.SessionID,
	})
	if err != nil {
		eventType := core.EventRunFailed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			eventType = core.EventRunCancelled
		}
		log.Printf("[chat] assistant reply failed project_id=%s role=%s session_id=%s err=%v", input.ProjectID, input.Role, input.SessionID, err)
		h.publishChatEvent(eventType, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reply := strings.TrimSpace(assistantResp.Reply)
	if reply == "" {
		h.publishChatEvent(core.EventRunFailed, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": "chat assistant returned empty reply",
		})
		return
	}

	session, err := h.store.GetChatSession(input.SessionID)
	if err != nil {
		h.publishChatEvent(core.EventRunFailed, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": "failed to load chat session",
		})
		return
	}
	if session.ProjectID != input.ProjectID {
		h.publishChatEvent(core.EventRunFailed, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": "chat session project mismatch",
		})
		return
	}

	if strings.TrimSpace(assistantResp.AgentSessionID) != "" {
		session.AgentSessionID = strings.TrimSpace(assistantResp.AgentSessionID)
	}
	session.Messages = append(session.Messages, core.ChatMessage{
		Role:    "assistant",
		Content: reply,
		Time:    time.Now().UTC(),
	})
	if err := h.store.UpdateChatSession(session); err != nil {
		h.publishChatEvent(core.EventRunFailed, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": "failed to update chat session",
		})
		return
	}

	eventData := map[string]string{
		"reply": reply,
	}
	if strings.TrimSpace(session.AgentSessionID) != "" {
		eventData["agent_session_id"] = strings.TrimSpace(session.AgentSessionID)
	}
	h.publishChatEvent(core.EventRunCompleted, input.ProjectID, input.SessionID, input.Role, eventData)
}

func (h *chatHandlers) cancelSession(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if projectID == "" || sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and session id are required", "INVALID_PATH_PARAM")
		return
	}

	session, err := h.store.GetChatSession(sessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", sessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	if !h.cancelSessionRun(sessionID) {
		writeAPIError(w, http.StatusConflict, "chat session is not running", "CHAT_SESSION_NOT_RUNNING")
		return
	}

	if canceler, ok := h.assistant.(ChatAssistantCanceler); ok {
		_ = canceler.CancelChat(sessionID)
	}

	writeJSON(w, http.StatusAccepted, cancelChatSessionResponse{
		SessionID: sessionID,
		Status:    "cancelling",
	})
}

func isValidRoleID(role string) bool {
	if role == "" {
		return false
	}
	for _, r := range role {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func (h *chatHandlers) getSession(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if projectID == "" || sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and session id are required", "INVALID_PATH_PARAM")
		return
	}

	session, err := h.store.GetChatSession(sessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", sessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, session)
}

func (h *chatHandlers) listSessionEvents(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if projectID == "" || sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and session id are required", "INVALID_PATH_PARAM")
		return
	}

	session, err := h.store.GetChatSession(sessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", sessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	reader, ok := h.store.(chatRunEventReader)
	if !ok {
		writeJSON(w, http.StatusOK, []core.ChatRunEvent{})
		return
	}
	events, err := reader.ListChatRunEvents(sessionID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list chat run events", "LIST_CHAT_RUN_EVENTS_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *chatHandlers) deleteSession(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if projectID == "" || sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and session id are required", "INVALID_PATH_PARAM")
		return
	}

	session, err := h.store.GetChatSession(sessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", sessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	deleter, ok := h.store.(chatSessionDeleter)
	if !ok {
		writeAPIError(w, http.StatusNotImplemented, "chat session delete is not supported by current store", "DELETE_CHAT_SESSION_UNSUPPORTED")
		return
	}
	if err := deleter.DeleteChatSession(sessionID); err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to delete chat session", "DELETE_CHAT_SESSION_FAILED")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *chatHandlers) registerSessionRun(sessionID string, cancel context.CancelFunc) bool {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" || cancel == nil {
		return false
	}
	h.runMu.Lock()
	defer h.runMu.Unlock()
	if _, exists := h.activeRun[trimmed]; exists {
		return false
	}
	h.activeRun[trimmed] = cancel
	return true
}

func (h *chatHandlers) unregisterSessionRun(sessionID string) {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return
	}
	h.runMu.Lock()
	defer h.runMu.Unlock()
	delete(h.activeRun, trimmed)
}

func (h *chatHandlers) cancelSessionRun(sessionID string) bool {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return false
	}
	h.runMu.Lock()
	cancel, exists := h.activeRun[trimmed]
	h.runMu.Unlock()
	if !exists || cancel == nil {
		return false
	}
	cancel()
	return true
}

func (h *chatHandlers) publishChatEvent(
	eventType core.EventType,
	projectID string,
	sessionID string,
	role string,
	data map[string]string,
) {
	if h.publisher == nil {
		return
	}
	payload := map[string]string{
		"session_id": strings.TrimSpace(sessionID),
	}
	if trimmedRole := strings.TrimSpace(role); trimmedRole != "" {
		payload["role"] = trimmedRole
	}
	for key, value := range data {
		payload[key] = value
	}
	h.publisher.Publish(context.Background(), core.Event{
		Type:      eventType,
		ProjectID: strings.TrimSpace(projectID),
		Data:      payload,
		Timestamp: time.Now().UTC(),
	})
}
