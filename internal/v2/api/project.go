package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

type createProjectRequest struct {
	Name        string            `json:"name"`
	Kind        string            `json:"kind,omitempty"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type updateProjectRequest struct {
	Name        *string           `json:"name,omitempty"`
	Kind        *string           `json:"kind,omitempty"`
	Description *string           `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "MISSING_NAME")
		return
	}

	kind := core.ProjectKind(strings.TrimSpace(req.Kind))
	if kind == "" {
		kind = core.ProjectGeneral
	}

	now := time.Now().UTC()
	p := &core.Project{
		Name:        name,
		Kind:        kind,
		Description: strings.TrimSpace(req.Description),
		Metadata:    req.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	id, err := h.store.CreateProject(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	p.ID = id
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}
	p, err := h.store.GetProject(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "project not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	projects, err := h.store.ListProjects(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if projects == nil {
		projects = []*core.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}

	existing, err := h.store.GetProject(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "project not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.Kind != nil {
		existing.Kind = core.ProjectKind(strings.TrimSpace(*req.Kind))
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.Metadata != nil {
		existing.Metadata = req.Metadata
	}

	if err := h.store.UpdateProject(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}

	if err := h.store.DeleteProject(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
