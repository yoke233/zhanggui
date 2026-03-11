package api

import (
	"encoding/json"
	"net/http"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- Request / Response types ---

type createDAGTemplateRequest struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	ProjectID   *int64              `json:"project_id,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Metadata    map[string]string   `json:"metadata,omitempty"`
	Steps       []core.DAGTemplateStep `json:"steps"`
}

type updateDAGTemplateRequest struct {
	Name        *string              `json:"name,omitempty"`
	Description *string              `json:"description,omitempty"`
	ProjectID   *int64               `json:"project_id,omitempty"`
	Tags        *[]string            `json:"tags,omitempty"`
	Metadata    map[string]string    `json:"metadata,omitempty"`
	Steps       *[]core.DAGTemplateStep `json:"steps,omitempty"`
}

type saveFlowAsTemplateRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type createFlowFromTemplateRequest struct {
	Name      string            `json:"name"`
	ProjectID *int64            `json:"project_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
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
		Steps:       req.Steps,
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
		existing.Steps = *req.Steps
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

// POST /flows/{flowID}/save-as-template
// Snapshots the current flow's steps into a new DAGTemplate.
func (h *Handler) saveFlowAsTemplate(w http.ResponseWriter, r *http.Request) {
	flowID, ok := urlParamInt64(r, "flowID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid flow ID", "BAD_ID")
		return
	}

	flow, err := h.store.GetFlow(r.Context(), flowID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	steps, err := h.store.ListStepsByFlow(r.Context(), flowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if len(steps) == 0 {
		writeError(w, http.StatusBadRequest, "flow has no steps to save as template", "NO_STEPS")
		return
	}

	var req saveFlowAsTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		req.Name = flow.Name + " (template)"
	}

	// Build step ID -> name lookup for dependency resolution.
	idToName := make(map[int64]string, len(steps))
	for _, s := range steps {
		idToName[s.ID] = s.Name
	}

	// Convert runtime steps to template steps (name-based dependencies).
	templateSteps := make([]core.DAGTemplateStep, 0, len(steps))
	for _, s := range steps {
		var depNames []string
		for _, depID := range s.DependsOn {
			if name, ok := idToName[depID]; ok {
				depNames = append(depNames, name)
			}
		}
		templateSteps = append(templateSteps, core.DAGTemplateStep{
			Name:                 s.Name,
			Description:          s.Description,
			Type:                 string(s.Type),
			DependsOn:            depNames,
			AgentRole:            s.AgentRole,
			RequiredCapabilities: s.RequiredCapabilities,
			AcceptanceCriteria:   s.AcceptanceCriteria,
		})
	}

	t := &core.DAGTemplate{
		Name:        req.Name,
		Description: req.Description,
		ProjectID:   flow.ProjectID,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		Steps:       templateSteps,
	}
	id, err := h.store.CreateDAGTemplate(r.Context(), t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	t.ID = id
	writeJSON(w, http.StatusCreated, t)
}

// POST /templates/{templateID}/create-flow
// Creates a new Flow and materializes template steps into it.
func (h *Handler) createFlowFromTemplate(w http.ResponseWriter, r *http.Request) {
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

	var req createFlowFromTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		req.Name = tmpl.Name
	}

	projectID := req.ProjectID
	if projectID == nil {
		projectID = tmpl.ProjectID
	}

	// Create the flow.
	flow := &core.Flow{
		Name:      req.Name,
		ProjectID: projectID,
		Status:    core.FlowPending,
		Metadata:  req.Metadata,
	}
	flowID, err := h.store.CreateFlow(r.Context(), flow)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	flow.ID = flowID

	// Materialize template steps into the flow.
	// First pass: create all steps with no dependencies to get their IDs.
	nameToID := make(map[string]int64, len(tmpl.Steps))
	createdSteps := make([]*core.Step, 0, len(tmpl.Steps))

	for _, ts := range tmpl.Steps {
		step := &core.Step{
			FlowID:               flowID,
			Name:                 ts.Name,
			Description:          ts.Description,
			Type:                 core.StepType(ts.Type),
			Status:               core.StepPending,
			AgentRole:            ts.AgentRole,
			RequiredCapabilities: ts.RequiredCapabilities,
			AcceptanceCriteria:   ts.AcceptanceCriteria,
		}
		id, err := h.store.CreateStep(r.Context(), step)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
		step.ID = id
		nameToID[ts.Name] = id
		createdSteps = append(createdSteps, step)
	}

	// Second pass: resolve name-based dependencies to ID-based.
	for i, ts := range tmpl.Steps {
		if len(ts.DependsOn) == 0 {
			continue
		}
		var depIDs []int64
		for _, depName := range ts.DependsOn {
			if depID, ok := nameToID[depName]; ok {
				depIDs = append(depIDs, depID)
			}
		}
		createdSteps[i].DependsOn = depIDs
		if err := h.store.UpdateStep(r.Context(), createdSteps[i]); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"flow":  flow,
		"steps": createdSteps,
	})
}
