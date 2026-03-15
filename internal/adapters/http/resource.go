package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

type createResourceRequest struct {
	Kind    string         `json:"kind"`
	RootURI string         `json:"root_uri"`
	Role    string         `json:"role,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
	Label   string         `json:"label,omitempty"`
}

func (h *Handler) createResourceSpace(w http.ResponseWriter, r *http.Request) {
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
	rootURI := strings.TrimSpace(req.RootURI)
	if kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required", "MISSING_KIND")
		return
	}
	if rootURI == "" {
		writeError(w, http.StatusBadRequest, "root_uri is required", "MISSING_ROOT_URI")
		return
	}

	rs := &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      kind,
		RootURI:   rootURI,
		Role:      strings.TrimSpace(req.Role),
		Config:    req.Config,
		Label:     strings.TrimSpace(req.Label),
	}
	id, err := h.store.CreateResourceSpace(r.Context(), rs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	rs.ID = id
	writeJSON(w, http.StatusCreated, rs)
}

func (h *Handler) listResourceSpaces(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}

	spaces, err := h.store.ListResourceSpaces(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if spaces == nil {
		spaces = []*core.ResourceSpace{}
	}
	writeJSON(w, http.StatusOK, spaces)
}

func (h *Handler) getResourceSpace(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "spaceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid space ID", "BAD_ID")
		return
	}

	rs, err := h.store.GetResourceSpace(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "resource space not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

type updateResourceRequest struct {
	Kind    *string        `json:"kind,omitempty"`
	RootURI *string        `json:"root_uri,omitempty"`
	Role    *string        `json:"role,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
	Label   *string        `json:"label,omitempty"`
}

func (h *Handler) updateResourceSpace(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "spaceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid space ID", "BAD_ID")
		return
	}
	rs, err := h.store.GetResourceSpace(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "resource space not found", "NOT_FOUND")
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
		rs.Kind = strings.TrimSpace(*req.Kind)
	}
	if req.RootURI != nil {
		rs.RootURI = strings.TrimSpace(*req.RootURI)
	}
	if req.Role != nil {
		rs.Role = strings.TrimSpace(*req.Role)
	}
	if req.Label != nil {
		rs.Label = strings.TrimSpace(*req.Label)
	}
	if req.Config != nil {
		rs.Config = req.Config
	}

	if err := h.store.UpdateResourceSpace(r.Context(), rs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (h *Handler) deleteResourceSpace(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "spaceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid space ID", "BAD_ID")
		return
	}

	if err := h.store.DeleteResourceSpace(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "resource space not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
