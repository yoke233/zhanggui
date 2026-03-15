package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- Action IO declaration endpoints ---

type createActionIODeclRequest struct {
	SpaceID     *int64 `json:"space_id,omitempty"`
	ResourceID  *int64 `json:"resource_id,omitempty"`
	Direction   string `json:"direction"`
	Path        string `json:"path"`
	MediaType   string `json:"media_type,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

func (h *Handler) createActionIODecl(w http.ResponseWriter, r *http.Request) {
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

	var req createActionIODeclRequest
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
	if (req.SpaceID == nil) == (req.ResourceID == nil) {
		writeError(w, http.StatusBadRequest, "exactly one of space_id or resource_id is required", "INVALID_REFERENCE")
		return
	}
	if req.SpaceID != nil {
		if _, err := h.store.GetResourceSpace(r.Context(), *req.SpaceID); err != nil {
			if err == core.ErrNotFound {
				writeError(w, http.StatusNotFound, "resource space not found", "RESOURCE_SPACE_NOT_FOUND")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
	}
	if req.ResourceID != nil {
		if _, err := h.store.GetResource(r.Context(), *req.ResourceID); err != nil {
			if err == core.ErrNotFound {
				writeError(w, http.StatusNotFound, "resource not found", "RESOURCE_NOT_FOUND")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
	}

	decl := &core.ActionIODecl{
		ActionID:    actionID,
		SpaceID:     req.SpaceID,
		ResourceID:  req.ResourceID,
		Direction:   core.IODirection(direction),
		Path:        path,
		MediaType:   strings.TrimSpace(req.MediaType),
		Description: strings.TrimSpace(req.Description),
		Required:    req.Required,
	}
	id, err := h.store.CreateActionIODecl(r.Context(), decl)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "STORE_ERROR")
		return
	}
	decl.ID = id
	writeJSON(w, http.StatusCreated, decl)
}

func (h *Handler) listActionIODecls(w http.ResponseWriter, r *http.Request) {
	actionID, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}
	decls, err := h.store.ListActionIODecls(r.Context(), actionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if decls == nil {
		decls = []*core.ActionIODecl{}
	}
	writeJSON(w, http.StatusOK, decls)
}

func (h *Handler) deleteActionIODecl(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "declID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid decl ID", "BAD_ID")
		return
	}
	if err := h.store.DeleteActionIODecl(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "action io decl not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
