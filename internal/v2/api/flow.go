package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// createFlowRequest is the request body for POST /flows.
type createFlowRequest struct {
	Name      string            `json:"name"`
	ProjectID *int64            `json:"project_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func (h *Handler) createFlow(w http.ResponseWriter, r *http.Request) {
	var req createFlowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "MISSING_NAME")
		return
	}

	if req.ProjectID != nil {
		if _, err := h.store.GetProject(r.Context(), *req.ProjectID); err != nil {
			if err == core.ErrNotFound {
				writeError(w, http.StatusNotFound, "project not found", "PROJECT_NOT_FOUND")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
	}

	f := &core.Flow{
		Name:      req.Name,
		ProjectID: req.ProjectID,
		Status:    core.FlowPending,
		Metadata:  req.Metadata,
	}
	id, err := h.store.CreateFlow(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	f.ID = id
	writeJSON(w, http.StatusCreated, f)
}

func (h *Handler) getFlow(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "flowID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid flow ID", "BAD_ID")
		return
	}

	f, err := h.store.GetFlow(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *Handler) listFlows(w http.ResponseWriter, r *http.Request) {
	archived := false
	filter := core.FlowFilter{
		Archived: &archived,
		Limit:    queryInt(r, "limit", 50),
		Offset:   queryInt(r, "offset", 0),
	}
	if projectID, ok := queryInt64(r, "project_id"); ok {
		filter.ProjectID = &projectID
	}
	if s := r.URL.Query().Get("status"); s != "" {
		status := core.FlowStatus(s)
		filter.Status = &status
	}
	if raw := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("archived"))); raw != "" {
		switch raw {
		case "true", "1", "yes":
			archived = true
			filter.Archived = &archived
		case "false", "0", "no":
			archived = false
			filter.Archived = &archived
		case "all":
			filter.Archived = nil
		default:
			writeError(w, http.StatusBadRequest, "invalid archived filter", "BAD_ARCHIVED_FILTER")
			return
		}
	}

	flows, err := h.store.ListFlows(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if flows == nil {
		flows = []*core.Flow{}
	}
	writeJSON(w, http.StatusOK, flows)
}

func (h *Handler) archiveFlow(w http.ResponseWriter, r *http.Request) {
	h.setFlowArchived(w, r, true)
}

func (h *Handler) unarchiveFlow(w http.ResponseWriter, r *http.Request) {
	h.setFlowArchived(w, r, false)
}

func (h *Handler) setFlowArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	id, ok := urlParamInt64(r, "flowID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid flow ID", "BAD_ID")
		return
	}

	f, err := h.store.GetFlow(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if archived {
		switch f.Status {
		case core.FlowQueued, core.FlowRunning, core.FlowBlocked:
			writeError(w, http.StatusConflict, "active flow cannot be archived", "INVALID_STATE")
			return
		}
	}

	if err := h.store.SetFlowArchived(r.Context(), id, archived); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
			return
		}
		if errors.Is(err, core.ErrInvalidTransition) {
			writeError(w, http.StatusConflict, "flow cannot be archived in current state", "INVALID_STATE")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	f, err = h.store.GetFlow(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// runFlow triggers async execution of a flow. Returns immediately.
// If a scheduler is configured, the flow is queued; otherwise it runs directly.
func (h *Handler) runFlow(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "flowID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid flow ID", "BAD_ID")
		return
	}

	// If scheduler is available, submit to queue.
	if h.scheduler != nil {
		if err := h.scheduler.Submit(r.Context(), id); err != nil {
			switch {
			case errors.Is(err, core.ErrNotFound):
				writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
				return
			case errors.Is(err, core.ErrInvalidTransition):
				f, getErr := h.store.GetFlow(r.Context(), id)
				if getErr == nil && f.ArchivedAt != nil {
					writeError(w, http.StatusConflict, "archived flow cannot be executed", "FLOW_ARCHIVED")
					return
				}
				writeError(w, http.StatusConflict, "flow is not pending", "INVALID_STATE")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error(), "SCHEDULER_ERROR")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"flow_id": id,
			"status":  "queued",
			"message": "flow queued for execution",
		})
		return
	}

	if err := h.store.PrepareFlowRun(r.Context(), id, core.FlowQueued); err != nil {
		switch {
		case errors.Is(err, core.ErrNotFound):
			writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
			return
		case errors.Is(err, core.ErrInvalidTransition):
			f, getErr := h.store.GetFlow(r.Context(), id)
			if getErr == nil && f.ArchivedAt != nil {
				writeError(w, http.StatusConflict, "archived flow cannot be executed", "FLOW_ARCHIVED")
				return
			}
			writeError(w, http.StatusConflict, "flow is not pending", "INVALID_STATE")
			return
		default:
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
	}

	// Fallback: run directly in background goroutine.
	go func() {
		ctx := context.Background()
		if err := h.engine.Run(ctx, id); err != nil {
			h.bus.Publish(ctx, core.Event{
				Type:      core.EventFlowFailed,
				FlowID:    id,
				Timestamp: time.Now().UTC(),
				Data:      map[string]any{"error": err.Error()},
			})
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"flow_id": id,
		"status":  "accepted",
		"message": "flow execution started",
	})
}

func (h *Handler) cancelFlow(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "flowID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid flow ID", "BAD_ID")
		return
	}

	// If scheduler is available, cancel via scheduler (handles both queued and running).
	var err error
	if h.scheduler != nil {
		err = h.scheduler.Cancel(r.Context(), id)
	} else {
		err = h.engine.Cancel(r.Context(), id)
	}

	if err != nil {
		if err == core.ErrInvalidTransition {
			writeError(w, http.StatusConflict, "flow cannot be cancelled in current state", "INVALID_STATE")
			return
		}
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "ENGINE_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"flow_id": id,
		"status":  "cancelled",
	})
}
