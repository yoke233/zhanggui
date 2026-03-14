package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
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
	r.Post("/chat/sessions/{sessionID}/crystallize-thread", handlers.crystallizeThread)
	r.Post("/chat", handlers.sendMessage)
	r.Get("/chat/{sessionID}", handlers.getSession)
	r.Post("/chat/{sessionID}/cancel", handlers.cancelChat)
	r.Post("/chat/{sessionID}/close", handlers.closeSession)
	r.Delete("/chat/{sessionID}", handlers.deleteSession)
	r.Get("/chat/{sessionID}/status", handlers.getStatus)
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

type crystallizeChatSessionRequest struct {
	ThreadTitle        string   `json:"thread_title"`
	ThreadSummary      string   `json:"thread_summary"`
	WorkItemTitle      string   `json:"work_item_title,omitempty"`
	WorkItemBody       string   `json:"work_item_body,omitempty"`
	ProjectID          *int64   `json:"project_id,omitempty"`
	ParticipantUserIDs []string `json:"participant_user_ids,omitempty"`
	CreateWorkItem     bool     `json:"create_work_item,omitempty"`
	OwnerID            string   `json:"owner_id,omitempty"`
}

type crystallizeChatSessionResponse struct {
	Thread       *core.Thread              `json:"thread"`
	WorkItem     *core.WorkItem            `json:"work_item,omitempty"`
	Participants []*core.ThreadMember `json:"participants"`
}

func (h *chatHandlers) crystallizeThread(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required", "BAD_REQUEST")
		return
	}

	detail, err := h.lead.GetSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "GET_CHAT_SESSION_FAILED")
		return
	}

	var req crystallizeChatSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	threadTitle := strings.TrimSpace(req.ThreadTitle)
	if threadTitle == "" {
		threadTitle = strings.TrimSpace(detail.Title)
	}
	if threadTitle == "" {
		threadTitle = fmt.Sprintf("Chat Session %s", sessionID)
	}

	threadSummary := strings.TrimSpace(req.ThreadSummary)
	ownerID := strings.TrimSpace(req.OwnerID)
	thread := &core.Thread{
		Title:    threadTitle,
		Status:   core.ThreadActive,
		OwnerID:  ownerID,
		Summary:  threadSummary,
		Metadata: map[string]any{"source_chat_session_id": sessionID},
	}

	participants := buildThreadParticipants(ownerID, req.ParticipantUserIDs)
	var workItem *core.WorkItem
	if txRunner, ok := h.handler.store.(core.TransactionalStore); ok {
		err = txRunner.InTx(r.Context(), func(txStore core.Store) error {
			threadID, err := txStore.CreateThread(r.Context(), thread)
			if err != nil {
				return err
			}
			thread.ID = threadID

			for _, participant := range participants {
				if participant == nil {
					continue
				}
				participant.ThreadID = thread.ID
				id, err := txStore.AddThreadMember(r.Context(), participant)
				if err != nil {
					return err
				}
				participant.ID = id
			}

			if req.CreateWorkItem {
				workItem, err = createWorkItemFromThreadDataWithStore(
					txStore, r.Context(), thread, req.WorkItemTitle, req.WorkItemBody, req.ProjectID,
				)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			if apiErr, ok := err.(*threadMessageAPIError); ok {
				writeError(w, http.StatusBadRequest, apiErr.Message, apiErr.Code)
				return
			}
			code := "CREATE_THREAD_FAILED"
			if req.CreateWorkItem {
				code = "CREATE_ISSUE_FAILED"
				if strings.Contains(err.Error(), "rollback failed") {
					code = "CREATE_LINK_FAILED"
				}
			}
			writeError(w, http.StatusInternalServerError, err.Error(), code)
			return
		}
	} else if txStore, ok := h.handler.store.(threadAggregateStore); ok {
		if err := txStore.CreateThreadWithParticipants(r.Context(), thread, participants); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_THREAD_FAILED")
			return
		}
		if req.CreateWorkItem {
			workItem, err = h.handler.createWorkItemFromThreadData(r.Context(), thread, req.WorkItemTitle, req.WorkItemBody, req.ProjectID)
			if err != nil {
				_ = h.handler.store.DeleteThread(r.Context(), thread.ID)
				if apiErr, ok := err.(*threadMessageAPIError); ok {
					writeError(w, http.StatusBadRequest, apiErr.Message, apiErr.Code)
					return
				}
				code := "CREATE_ISSUE_FAILED"
				if strings.Contains(err.Error(), "rollback failed") {
					code = "CREATE_LINK_FAILED"
				}
				writeError(w, http.StatusInternalServerError, err.Error(), code)
				return
			}
		}
	} else {
		threadID, err := h.handler.store.CreateThread(r.Context(), thread)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_THREAD_FAILED")
			return
		}
		thread.ID = threadID

		participants = participants[:0]
		if ownerParticipant, err := h.handler.ensureThreadParticipant(r.Context(), thread.ID, ownerID, "owner"); err != nil {
			_ = h.handler.store.DeleteThread(r.Context(), thread.ID)
			writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_THREAD_FAILED")
			return
		} else if ownerParticipant != nil {
			participants = append(participants, ownerParticipant)
		}

		seen := make(map[string]bool)
		if ownerID != "" {
			seen[ownerID] = true
		}
		for _, participantID := range req.ParticipantUserIDs {
			participantID = strings.TrimSpace(participantID)
			if participantID == "" || seen[participantID] {
				continue
			}
			participant, err := h.handler.ensureThreadParticipant(r.Context(), thread.ID, participantID, "member")
			if err != nil {
				_ = h.handler.store.DeleteThread(r.Context(), thread.ID)
				writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_THREAD_FAILED")
				return
			}
			if participant != nil {
				participants = append(participants, participant)
			}
			seen[participantID] = true
		}
		if req.CreateWorkItem {
			workItem, err = h.handler.createWorkItemFromThreadData(r.Context(), thread, req.WorkItemTitle, req.WorkItemBody, req.ProjectID)
			if err != nil {
				_ = h.handler.store.DeleteThread(r.Context(), thread.ID)
				if apiErr, ok := err.(*threadMessageAPIError); ok {
					writeError(w, http.StatusBadRequest, apiErr.Message, apiErr.Code)
					return
				}
				code := "CREATE_ISSUE_FAILED"
				if strings.Contains(err.Error(), "rollback failed") {
					code = "CREATE_LINK_FAILED"
				}
				writeError(w, http.StatusInternalServerError, err.Error(), code)
				return
			}
		}
	}

	writeJSON(w, http.StatusCreated, crystallizeChatSessionResponse{
		Thread:       thread,
		WorkItem:     workItem,
		Participants: participants,
	})
}
