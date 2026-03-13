package api

import (
	"encoding/json"
	"net/http"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- Request / Response types ---

type createDAGTemplateRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	ProjectID   *int64                 `json:"project_id,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Metadata    map[string]string      `json:"metadata,omitempty"`
	Steps       []core.DAGTemplateAction `json:"steps"`
}

type updateDAGTemplateRequest struct {
	Name        *string                 `json:"name,omitempty"`
	Description *string                 `json:"description,omitempty"`
	ProjectID   *int64                  `json:"project_id,omitempty"`
	Tags        *[]string               `json:"tags,omitempty"`
	Metadata    map[string]string       `json:"metadata,omitempty"`
	Steps       *[]core.DAGTemplateAction `json:"steps,omitempty"`
}

type saveWorkItemAsTemplateRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type createWorkItemFromTemplateRequest struct {
	Title     string         `json:"title"`
	ProjectID *int64         `json:"project_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// --- Handlers ---

// POST /templates
func (h *Handler) createDAGTemplate(w http.ResponseWriter, r *http.Request) {
	var req createDAGTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "MISSING_NAME")
		return
	}
	if len(req.Steps) == 0 {
		writeError(w, http.StatusBadRequest, "at least one step is required", "MISSING_STEPS")
		return
	}

	t := &core.DAGTemplate{
		Name:        req.Name,
		Description: req.Description,
		ProjectID:   req.ProjectID,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		Actions:     req.Steps,
	}
	id, err := h.store.CreateDAGTemplate(r.Context(), t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	t.ID = id
	writeJSON(w, http.StatusCreated, t)
}

// GET /templates
func (h *Handler) listDAGTemplates(w http.ResponseWriter, r *http.Request) {
	filter := core.DAGTemplateFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
		Search: r.URL.Query().Get("search"),
		Tag:    r.URL.Query().Get("tag"),
	}
	if pid, ok := queryInt64(r, "project_id"); ok {
		filter.ProjectID = &pid
	}
	templates, err := h.store.ListDAGTemplates(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if templates == nil {
		templates = []*core.DAGTemplate{}
	}
	writeJSON(w, http.StatusOK, templates)
}

// GET /templates/{templateID}
func (h *Handler) getDAGTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "templateID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid template ID", "BAD_ID")
		return
	}
	t, err := h.store.GetDAGTemplate(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "template not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// PUT /templates/{templateID}
func (h *Handler) updateDAGTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "templateID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid template ID", "BAD_ID")
		return
	}

	existing, err := h.store.GetDAGTemplate(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "template not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req updateDAGTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.ProjectID != nil {
		existing.ProjectID = req.ProjectID
	}
	if req.Tags != nil {
		existing.Tags = *req.Tags
	}
	if req.Metadata != nil {
		existing.Metadata = req.Metadata
	}
	if req.Steps != nil {
		existing.Actions = *req.Steps
	}

	if err := h.store.UpdateDAGTemplate(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// DELETE /templates/{templateID}
func (h *Handler) deleteDAGTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "templateID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid template ID", "BAD_ID")
		return
	}
	if err := h.store.DeleteDAGTemplate(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "template not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /work-items/{issueID}/save-as-template
// Snapshots the current issue's steps into a new DAGTemplate.
func (h *Handler) saveWorkItemAsTemplate(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	issue, err := h.store.GetWorkItem(r.Context(), issueID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	steps, err := h.store.ListActionsByWorkItem(r.Context(), issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if len(steps) == 0 {
		writeError(w, http.StatusBadRequest, "issue has no steps to save as template", "NO_STEPS")
		return
	}

	var req saveWorkItemAsTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		req.Name = issue.Title + " (template)"
	}

	// Convert runtime steps to template steps (position-ordered).
	templateSteps := make([]core.DAGTemplateAction, 0, len(steps))
	for _, s := range steps {
		templateSteps = append(templateSteps, core.DAGTemplateAction{
			Name:                 s.Name,
			Description:          s.Description,
			Type:                 string(s.Type),
			AgentRole:            s.AgentRole,
			RequiredCapabilities: s.RequiredCapabilities,
			AcceptanceCriteria:   s.AcceptanceCriteria,
		})
	}

	t := &core.DAGTemplate{
		Name:        req.Name,
		Description: req.Description,
		ProjectID:   issue.ProjectID,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		Actions:     templateSteps,
	}
	id, err := h.store.CreateDAGTemplate(r.Context(), t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	t.ID = id
	writeJSON(w, http.StatusCreated, t)
}

// POST /templates/{templateID}/create-issue
// Creates a new Issue and materializes template steps into it.
func (h *Handler) createWorkItemFromTemplate(w http.ResponseWriter, r *http.Request) {
	templateID, ok := urlParamInt64(r, "templateID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid template ID", "BAD_ID")
		return
	}

	tmpl, err := h.store.GetDAGTemplate(r.Context(), templateID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "template not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req createWorkItemFromTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Title == "" {
		req.Title = tmpl.Name
	}

	projectID := req.ProjectID
	if projectID == nil {
		projectID = tmpl.ProjectID
	}

	// Create the work item.
	issue := &core.WorkItem{
		Title:     req.Title,
		ProjectID: projectID,
		Status:    core.WorkItemOpen,
		Metadata:  req.Metadata,
	}
	issueID, err := h.store.CreateWorkItem(r.Context(), issue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	issue.ID = issueID

	// Materialize template steps into the issue with position-based ordering.
	createdSteps := make([]*core.Action, 0, len(tmpl.Actions))

	for i, ts := range tmpl.Actions {
		step := &core.Action{
			WorkItemID:           issueID,
			Name:                 ts.Name,
			Description:          ts.Description,
			Type:                 core.ActionType(ts.Type),
			Status:               core.ActionPending,
			Position:             i,
			AgentRole:            ts.AgentRole,
			RequiredCapabilities: ts.RequiredCapabilities,
			AcceptanceCriteria:   ts.AcceptanceCriteria,
		}
		id, err := h.store.CreateAction(r.Context(), step)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
		step.ID = id
		createdSteps = append(createdSteps, step)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"issue": issue,
		"steps": createdSteps,
	})
}
