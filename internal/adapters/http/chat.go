package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

// chatHandlers holds the lead agent chat endpoint handlers.
type chatHandlers struct {
	lead LeadChatService
}

func registerChatRoutes(r chi.Router, lead LeadChatService) {
	if lead == nil {
		return
	}
	h := &chatHandlers{lead: lead}
	r.Get("/chat/sessions", h.listSessions)
	r.Post("/chat", h.sendMessage)
	r.Get("/chat/{sessionID}", h.getSession)
	r.Post("/chat/{sessionID}/cancel", h.cancelChat)
	r.Post("/chat/{sessionID}/close", h.closeSession)
	r.Delete("/chat/{sessionID}", h.deleteSession)
	r.Get("/chat/{sessionID}/status", h.getStatus)
}

// GET /chat/sessions — list persisted lead chat sessions.
func (h *chatHandlers) listSessions(w http.ResponseWriter, r *http.Request) {
	resp, err := h.lead.ListSessions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "LIST_CHAT_SESSIONS_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /chat — deprecated, use WebSocket chat.send instead.
func (h *chatHandlers) sendMessage(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusGone, "POST /api/chat is deprecated; use websocket message type chat.send", "CHAT_HTTP_DEPRECATED")
}

// GET /chat/{sessionID} — load one persisted session including message history.
func (h *chatHandlers) getSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}

	resp, err := h.lead.GetSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "GET_CHAT_SESSION_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /chat/{sessionID}/cancel — cancel the current prompt.
func (h *chatHandlers) cancelChat(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}
	if err := h.lead.CancelChat(sessionID); err != nil {
		writeError(w, http.StatusConflict, err.Error(), "CANCEL_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"session_id": sessionID,
		"status":     "cancelled",
	})
}

// POST /chat/{sessionID}/close — close the session (recycle agent, keep workspace).
func (h *chatHandlers) closeSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}
	h.lead.CloseSession(sessionID)
	writeJSON(w, http.StatusOK, map[string]string{
		"session_id": sessionID,
		"status":     "closed",
	})
}

// DELETE /chat/{sessionID} — permanently delete session and clean up workspace.
func (h *chatHandlers) deleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}
	h.lead.DeleteSession(sessionID)
	writeJSON(w, http.StatusOK, map[string]string{
		"session_id": sessionID,
		"status":     "deleted",
	})
}

// GET /chat/{sessionID}/status — check session status.
func (h *chatHandlers) getStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}
	status := "not_found"
	if h.lead.IsSessionAlive(sessionID) {
		status = "alive"
		if h.lead.IsSessionRunning(sessionID) {
			status = "running"
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"session_id": sessionID,
		"status":     status,
	})
}
