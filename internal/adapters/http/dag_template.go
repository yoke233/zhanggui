package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/yoke233/zhanggui/internal/core"
)

// --- Request / Response types ---

type createDAGTemplateRequest struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	ProjectID   *int64                   `json:"project_id,omitempty"`
	Tags        []string                 `json:"tags,omitempty"`
	Metadata    map[string]string        `json:"metadata,omitempty"`
	Actions     []core.DAGTemplateAction `json:"actions"`
}

type updateDAGTemplateRequest struct {
	Name        *string                   `json:"name,omitempty"`
	Description *string                   `json:"description,omitempty"`
	ProjectID   *int64                    `json:"project_id,omitempty"`
	Tags        *[]string                 `json:"tags,omitempty"`
	Metadata    map[string]string         `json:"metadata,omitempty"`
	Actions     *[]core.DAGTemplateAction `json:"actions,omitempty"`
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

func validateTemplateActions(actions []core.DAGTemplateAction) error {
	if len(actions) == 0 {
		return fmt.Errorf("at least one action is required")
	}

	nameSet := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if action.Name == "" {
			return fmt.Errorf("action name is required")
		}
		if action.Type == "" {
			return fmt.Errorf("action %q type is required", action.Name)
		}
		if _, exists := nameSet[action.Name]; exists {
			return fmt.Errorf("duplicate action name %q", action.Name)
		}
		nameSet[action.Name] = struct{}{}
	}

	for _, action := range actions {
		for _, depName := range action.DependsOn {
			if depName == action.Name {
				return fmt.Errorf("action %q depends on itself", action.Name)
			}
			if _, exists := nameSet[depName]; !exists {
				return fmt.Errorf("action %q depends on unknown action %q", action.Name, depName)
			}
		}
	}

	return nil
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
	if len(req.Actions) == 0 {
		writeError(w, http.StatusBadRequest, "at least one action is required", "MISSING_ACTIONS")
		return
	}
	if err := validateTemplateActions(req.Actions); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_TEMPLATE")
		return
	}

	t := &core.DAGTemplate{
		Name:        req.Name,
		Description: req.Description,
		ProjectID:   req.ProjectID,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		Actions:     req.Actions,
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
	if req.Actions != nil {
		if err := validateTemplateActions(*req.Actions); err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "INVALID_TEMPLATE")
			return
		}
		existing.Actions = *req.Actions
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

// POST /work-items/{workItemID}/save-as-template
// Snapshots the current work item's actions into a new DAGTemplate.
func (h *Handler) saveWorkItemAsTemplate(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}

	workItem, err := h.store.GetWorkItem(r.Context(), workItemID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "work item not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	actions, err := h.store.ListActionsByWorkItem(r.Context(), workItemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if len(actions) == 0 {
		writeError(w, http.StatusBadRequest, "work item has no actions to save as template", "NO_ACTIONS")
		return
	}

	var req saveWorkItemAsTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		req.Name = workItem.Title + " (template)"
	}

	// Reject duplicate action names — templates use names for DependsOn references,
	// so duplicates would cause ambiguous or incorrect dependency wiring on replay.
	namesSeen := make(map[string]bool, len(actions))
	for _, s := range actions {
		if namesSeen[s.Name] {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("duplicate action name %q; cannot save as template with ambiguous dependency names", s.Name),
				"DUPLICATE_NAME")
			return
		}
		namesSeen[s.Name] = true
	}

	// Build ID→name map for reverse-resolving DependsOn IDs to names.
	idToName := make(map[int64]string, len(actions))
	for _, s := range actions {
		idToName[s.ID] = s.Name
	}

	// Convert runtime actions to template actions (position-ordered), preserving DependsOn.
	templateActions := make([]core.DAGTemplateAction, 0, len(actions))
	for _, s := range actions {
		var depNames []string
		for _, depID := range s.DependsOn {
			name, ok := idToName[depID]
			if !ok {
				writeError(w, http.StatusInternalServerError,
					fmt.Sprintf("action %q depends on unknown action ID %d", s.Name, depID), "TEMPLATE_ERROR")
				return
			}
			depNames = append(depNames, name)
		}
		templateActions = append(templateActions, core.DAGTemplateAction{
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
		ProjectID:   workItem.ProjectID,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		Actions:     templateActions,
	}
	if err := validateTemplateActions(t.Actions); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_TEMPLATE")
		return
	}
	id, err := h.store.CreateDAGTemplate(r.Context(), t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	t.ID = id
	writeJSON(w, http.StatusCreated, t)
}

// POST /templates/{templateID}/create-work-item
// Creates a new work item and materializes template actions into it.
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

	if err := validateTemplateActions(tmpl.Actions); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_TEMPLATE")
		return
	}

	// Create the work item.
	workItem := &core.WorkItem{
		Title:     req.Title,
		ProjectID: projectID,
		Status:    core.WorkItemOpen,
		Metadata:  req.Metadata,
	}
	workItemID, err := h.store.CreateWorkItem(r.Context(), workItem)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	workItem.ID = workItemID

	// Phase 1: Materialize template actions into the work item with position-based ordering.
	nameToID := make(map[string]int64, len(tmpl.Actions))
	createdActions := make([]*core.Action, 0, len(tmpl.Actions))

	for i, ts := range tmpl.Actions {
		action := &core.Action{
			WorkItemID:           workItemID,
			Name:                 ts.Name,
			Description:          ts.Description,
			Type:                 core.ActionType(ts.Type),
			Status:               core.ActionPending,
			Position:             i,
			AgentRole:            ts.AgentRole,
			RequiredCapabilities: ts.RequiredCapabilities,
			AcceptanceCriteria:   ts.AcceptanceCriteria,
		}
		id, err := h.store.CreateAction(r.Context(), action)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
		action.ID = id
		nameToID[ts.Name] = id
		createdActions = append(createdActions, action)
	}

	// Phase 2: Resolve template DependsOn names → action IDs and persist.
	for i, ts := range tmpl.Actions {
		if len(ts.DependsOn) == 0 {
			continue
		}
		resolved := make([]int64, 0, len(ts.DependsOn))
		for _, depName := range ts.DependsOn {
			depID, ok := nameToID[depName]
			if !ok {
				writeError(w, http.StatusInternalServerError,
					"template action "+ts.Name+" depends on unknown action "+depName, "TEMPLATE_ERROR")
				return
			}
			resolved = append(resolved, depID)
		}
		if err := h.store.UpdateActionDependsOn(r.Context(), createdActions[i].ID, resolved); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
		createdActions[i].DependsOn = resolved
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"work_item": workItem,
		"actions":   createdActions,
	})
}
