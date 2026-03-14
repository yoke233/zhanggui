package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

type createResourceRequest struct {
	Kind   string         `json:"kind"`
	URI    string         `json:"uri"`
	Config map[string]any `json:"config,omitempty"`
	Label  string         `json:"label,omitempty"`
}

func (h *Handler) createResourceBinding(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}

	// Verify project exists.
	if _, err := h.store.GetProject(r.Context(), projectID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found", "PROJECT_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req createResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	kind := strings.TrimSpace(req.Kind)
	uri := strings.TrimSpace(req.URI)
	if kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required", "MISSING_KIND")
		return
	}
	if uri == "" {
		writeError(w, http.StatusBadRequest, "uri is required", "MISSING_URI")
		return
	}

	rb := &core.ResourceBinding{
		ProjectID: projectID,
		Kind:      kind,
		URI:       uri,
		Config:    req.Config,
		Label:     strings.TrimSpace(req.Label),
	}
	id, err := h.store.CreateResourceBinding(r.Context(), rb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	rb.ID = id
	writeJSON(w, http.StatusCreated, rb)
}

func (h *Handler) listResourceBindings(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}

	bindings, err := h.store.ListResourceBindings(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if bindings == nil {
		bindings = []*core.ResourceBinding{}
	}
	writeJSON(w, http.StatusOK, bindings)
}

func (h *Handler) getResourceBinding(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "resourceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid resource ID", "BAD_ID")
		return
	}

	rb, err := h.store.GetResourceBinding(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "resource binding not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, rb)
}

type updateResourceRequest struct {
	Kind   *string        `json:"kind,omitempty"`
	URI    *string        `json:"uri,omitempty"`
	Config map[string]any `json:"config,omitempty"`
	Label  *string        `json:"label,omitempty"`
}

func (h *Handler) updateResourceBinding(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "resourceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid resource ID", "BAD_ID")
		return
	}
	rb, err := h.store.GetResourceBinding(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "resource binding not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req updateResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Kind != nil {
		rb.Kind = strings.TrimSpace(*req.Kind)
	}
	if req.URI != nil {
		rb.URI = strings.TrimSpace(*req.URI)
	}
	if req.Label != nil {
		rb.Label = strings.TrimSpace(*req.Label)
	}
	if req.Config != nil {
		rb.Config = req.Config
	}

	if err := h.store.UpdateResourceBinding(r.Context(), rb); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, rb)
}

func (h *Handler) deleteResourceBinding(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "resourceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid resource ID", "BAD_ID")
		return
	}

	if err := h.store.DeleteResourceBinding(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "resource binding not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

