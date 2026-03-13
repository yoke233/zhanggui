package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- Action Resource endpoints ---

type createActionResourceRequest struct {
	LocatorID   int64          `json:"locator_id"`
	Direction   string         `json:"direction"`
	Path        string         `json:"path"`
	MediaType   string         `json:"media_type,omitempty"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func (h *Handler) createActionResource(w http.ResponseWriter, r *http.Request) {
	actionID, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}

	// Verify action exists.
	if _, err := h.store.GetAction(r.Context(), actionID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "action not found", "ACTION_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req createActionResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	direction := strings.TrimSpace(req.Direction)
	path := strings.TrimSpace(req.Path)
	if direction != "input" && direction != "output" {
		writeError(w, http.StatusBadRequest, "direction must be 'input' or 'output'", "INVALID_DIRECTION")
		return
	}
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required", "MISSING_PATH")
		return
	}
	if req.LocatorID <= 0 {
		writeError(w, http.StatusBadRequest, "locator_id is required", "MISSING_LOCATOR_ID")
		return
	}

	// Verify locator exists.
	if _, err := h.store.GetResourceLocator(r.Context(), req.LocatorID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "resource locator not found", "LOCATOR_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	ar := &core.ActionResource{
		ActionID:    actionID,
		LocatorID:   req.LocatorID,
		Direction:   core.ActionResourceDirection(direction),
		Path:        path,
		MediaType:   strings.TrimSpace(req.MediaType),
		Description: strings.TrimSpace(req.Description),
		Required:    req.Required,
		Metadata:    req.Metadata,
	}
	id, err := h.store.CreateActionResource(r.Context(), ar)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	ar.ID = id
	writeJSON(w, http.StatusCreated, ar)
}

func (h *Handler) listActionResources(w http.ResponseWriter, r *http.Request) {
	actionID, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}
	resources, err := h.store.ListActionResources(r.Context(), actionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if resources == nil {
		resources = []*core.ActionResource{}
	}
	writeJSON(w, http.StatusOK, resources)
}

func (h *Handler) deleteActionResource(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "resourceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid resource ID", "BAD_ID")
		return
	}
	if err := h.store.DeleteActionResource(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "action resource not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
