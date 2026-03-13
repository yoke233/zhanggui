package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- Resource Locator endpoints ---

type createResourceLocatorRequest struct {
	Kind    string         `json:"kind"`
	Label   string         `json:"label"`
	BaseURI string         `json:"base_uri"`
	Config  map[string]any `json:"config,omitempty"`
}

type updateResourceLocatorRequest struct {
	Kind    *string        `json:"kind,omitempty"`
	Label   *string        `json:"label,omitempty"`
	BaseURI *string        `json:"base_uri,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
}

func (h *Handler) createResourceLocator(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}
	if _, err := h.store.GetProject(r.Context(), projectID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found", "PROJECT_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req createResourceLocatorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	kind := strings.TrimSpace(req.Kind)
	baseURI := strings.TrimSpace(req.BaseURI)
	if kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required", "MISSING_KIND")
		return
	}
	if baseURI == "" {
		writeError(w, http.StatusBadRequest, "base_uri is required", "MISSING_BASE_URI")
		return
	}

	loc := &core.ResourceLocator{
		ProjectID: projectID,
		Kind:      core.ResourceLocatorKind(kind),
		Label:     strings.TrimSpace(req.Label),
		BaseURI:   baseURI,
		Config:    req.Config,
	}
	id, err := h.store.CreateResourceLocator(r.Context(), loc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	loc.ID = id
	writeJSON(w, http.StatusCreated, loc)
}

func (h *Handler) listResourceLocators(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}
	locators, err := h.store.ListResourceLocators(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if locators == nil {
		locators = []*core.ResourceLocator{}
	}
	writeJSON(w, http.StatusOK, locators)
}

func (h *Handler) getResourceLocator(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "locatorID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid locator ID", "BAD_ID")
		return
	}
	loc, err := h.store.GetResourceLocator(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "resource locator not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, loc)
}

func (h *Handler) updateResourceLocator(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "locatorID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid locator ID", "BAD_ID")
		return
	}
	loc, err := h.store.GetResourceLocator(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "resource locator not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req updateResourceLocatorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Kind != nil {
		loc.Kind = core.ResourceLocatorKind(strings.TrimSpace(*req.Kind))
	}
	if req.Label != nil {
		loc.Label = strings.TrimSpace(*req.Label)
	}
	if req.BaseURI != nil {
		loc.BaseURI = strings.TrimSpace(*req.BaseURI)
	}
	if req.Config != nil {
		loc.Config = req.Config
	}

	if err := h.store.UpdateResourceLocator(r.Context(), loc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, loc)
}

func (h *Handler) deleteResourceLocator(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "locatorID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid locator ID", "BAD_ID")
		return
	}
	if err := h.store.DeleteResourceLocator(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "resource locator not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
