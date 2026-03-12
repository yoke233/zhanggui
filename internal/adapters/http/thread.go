package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

type createThreadRequest struct {
	Title    string         `json:"title"`
	OwnerID  string         `json:"owner_id,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type updateThreadRequest struct {
	Title    *string        `json:"title,omitempty"`
	Status   *string        `json:"status,omitempty"`
	OwnerID  *string        `json:"owner_id,omitempty"`
	Summary  *string        `json:"summary,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// registerThreadRoutes mounts thread endpoints onto the given router.
func registerThreadRoutes(r chi.Router, h *Handler) {
	r.Post("/threads", h.createThread)
	r.Get("/threads", h.listThreads)
	r.Get("/threads/{threadID}", h.getThread)
	r.Put("/threads/{threadID}", h.updateThread)
	r.Delete("/threads/{threadID}", h.deleteThread)
}

func (h *Handler) createThread(w http.ResponseWriter, r *http.Request) {
	var req createThreadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "MISSING_TITLE")
		return
	}

	thread := &core.Thread{
		Title:    title,
		Status:   core.ThreadActive,
		OwnerID:  strings.TrimSpace(req.OwnerID),
		Metadata: req.Metadata,
	}

	id, err := h.store.CreateThread(r.Context(), thread)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_THREAD_FAILED")
		return
	}
	thread.ID = id
	writeJSON(w, http.StatusCreated, thread)
}

func (h *Handler) listThreads(w http.ResponseWriter, r *http.Request) {
	filter := core.ThreadFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	if s := r.URL.Query().Get("status"); s != "" {
		st := core.ThreadStatus(s)
		filter.Status = &st
	}

	threads, err := h.store.ListThreads(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if threads == nil {
		threads = []*core.Thread{}
	}
	writeJSON(w, http.StatusOK, threads)
}

func (h *Handler) getThread(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	thread, err := h.store.GetThread(r.Context(), threadID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (h *Handler) updateThread(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	thread, err := h.store.GetThread(r.Context(), threadID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req updateThreadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	if req.Title != nil {
		thread.Title = strings.TrimSpace(*req.Title)
	}
	if req.Status != nil {
		thread.Status = core.ThreadStatus(strings.TrimSpace(*req.Status))
	}
	if req.OwnerID != nil {
		thread.OwnerID = strings.TrimSpace(*req.OwnerID)
	}
	if req.Summary != nil {
		thread.Summary = strings.TrimSpace(*req.Summary)
	}
	if req.Metadata != nil {
		thread.Metadata = req.Metadata
	}

	if err := h.store.UpdateThread(r.Context(), thread); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "UPDATE_THREAD_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (h *Handler) deleteThread(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	if err := h.store.DeleteThread(r.Context(), threadID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "DELETE_THREAD_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
