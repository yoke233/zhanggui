package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/zhanggui/internal/core"
)

// chatHandlers holds the lead agent chat endpoint handlers.
type chatHandlers struct {
	handler *Handler
	lead    LeadChatService
}

func registerChatRoutes(r chi.Router, h *Handler) {
	if h == nil || h.lead == nil {
		return
	}
	handlers := &chatHandlers{handler: h, lead: h.lead}
	r.Get("/chat/sessions", handlers.listSessions)
	r.Post("/chat/sessions/{sessionID}/archive", handlers.archiveSession)
	r.Patch("/chat/sessions/{sessionID}/rename", handlers.renameSession)
	r.Post("/chat", handlers.sendMessage)
	r.Get("/chat/{sessionID}", handlers.getSession)
	r.Post("/chat/{sessionID}/cancel", handlers.cancelChat)
	r.Post("/chat/{sessionID}/close", handlers.closeSession)
	r.Post("/chat/sessions/{sessionID}/submit-code", handlers.submitCode)
	r.Delete("/chat/{sessionID}", handlers.deleteSession)
	r.Get("/chat/{sessionID}/status", handlers.getStatus)
	r.Post("/chat/sessions/{sessionID}/create-pr", handlers.createPR)
	r.Post("/chat/sessions/{sessionID}/refresh-pr", handlers.refreshPR)
}

// GET /chat/sessions — list persisted lead chat sessions.
// Query params:
//   - archived=true  → include only archived sessions
//   - archived=all   → include both active and archived
//   - (default)      → exclude archived sessions
func (h *chatHandlers) listSessions(w http.ResponseWriter, r *http.Request) {
	resp, err := h.lead.ListSessions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "LIST_CHAT_SESSIONS_FAILED")
		return
	}

	archivedParam := strings.TrimSpace(r.URL.Query().Get("archived"))
	if archivedParam != "all" {
		wantArchived := archivedParam == "true"
		filtered := resp[:0]
		for _, s := range resp {
			if s.Archived == wantArchived {
				filtered = append(filtered, s)
			}
		}
		resp = filtered
	}

	writeJSON(w, http.StatusOK, resp)
}

// POST /chat/sessions/{sessionID}/archive — toggle session archived state.
func (h *chatHandlers) archiveSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}

	var body struct {
		Archived bool `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Default to archive when no body is provided.
		body.Archived = true
	}

	if err := h.lead.ArchiveSession(sessionID, body.Archived); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "ARCHIVE_SESSION_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sessionID,
		"archived":   body.Archived,
	})
}

// PATCH /chat/sessions/{sessionID}/rename — update session title.
func (h *chatHandlers) renameSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "BAD_REQUEST")
		return
	}

	if err := h.lead.RenameSession(sessionID, title); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "RENAME_SESSION_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sessionID,
		"title":      title,
	})
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

// POST /chat/sessions/{sessionID}/submit-code — commit and push the chat session branch.
func (h *chatHandlers) submitCode(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	stats, err := h.lead.SubmitCode(r.Context(), sessionID, req.Message)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "SUBMIT_CODE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// POST /chat/sessions/{sessionID}/create-pr — create a PR/MR for the session's branch.
func (h *chatHandlers) createPR(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}
	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	stats, err := h.lead.CreatePR(r.Context(), sessionID, req.Title, req.Body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "CREATE_PR_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// POST /chat/sessions/{sessionID}/refresh-pr — refresh PR state from SCM.
func (h *chatHandlers) refreshPR(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}
	stats, err := h.lead.RefreshPR(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "REFRESH_PR_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
