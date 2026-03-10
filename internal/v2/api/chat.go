package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/v2/engine"
)

// chatHandlers holds the lead agent chat endpoint handlers.
type chatHandlers struct {
	lead *engine.LeadAgent
}

func registerChatRoutes(r chi.Router, lead *engine.LeadAgent) {
	if lead == nil {
		return
	}
	h := &chatHandlers{lead: lead}
	r.Post("/chat", h.sendMessage)
	r.Post("/chat/{sessionID}/cancel", h.cancelChat)
	r.Delete("/chat/{sessionID}", h.closeSession)
	r.Get("/chat/{sessionID}/status", h.getStatus)
}

// POST /chat — send a message to the lead agent.
func (h *chatHandlers) sendMessage(w http.ResponseWriter, r *http.Request) {
	var req engine.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}

	resp, err := h.lead.Chat(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "CHAT_FAILED")
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

// DELETE /chat/{sessionID} — close the session.
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
