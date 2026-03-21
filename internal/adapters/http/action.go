package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	flowapp "github.com/yoke233/zhanggui/internal/application/flow"
	"github.com/yoke233/zhanggui/internal/core"
)

// createActionRequest is the request body for POST /work-items/{workItemID}/actions.
type createActionRequest struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description,omitempty"`
	Type                 core.ActionType `json:"type"`
	Position             *int            `json:"position,omitempty"`
	DependsOn            []int64         `json:"depends_on,omitempty"`
	AgentRole            string          `json:"agent_role,omitempty"`
	RequiredCapabilities []string        `json:"required_capabilities,omitempty"`
	AcceptanceCriteria   []string        `json:"acceptance_criteria,omitempty"`
	Timeout              string          `json:"timeout,omitempty"` // Go duration string
	MaxRetries           int             `json:"max_retries"`
	Config               map[string]any  `json:"config,omitempty"`
}

func (h *Handler) createAction(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}

	var req createActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "MISSING_NAME")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required", "MISSING_TYPE")
		return
	}
	position, err := h.resolveCreateActionPosition(r.Context(), workItemID, req.Position)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_POSITION")
		return
	}

	var timeout time.Duration
	if req.Timeout != "" {
		timeout, err = time.ParseDuration(req.Timeout)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid timeout duration", "BAD_TIMEOUT")
			return
		}
	}

	s := &core.Action{
		WorkItemID:           workItemID,
		Name:                 req.Name,
		Description:          req.Description,
		Type:                 req.Type,
		Status:               core.ActionPending,
		Position:             position,
		DependsOn:            req.DependsOn,
		AgentRole:            req.AgentRole,
		RequiredCapabilities: req.RequiredCapabilities,
		AcceptanceCriteria:   req.AcceptanceCriteria,
		Timeout:              timeout,
		MaxRetries:           req.MaxRetries,
		Config:               req.Config,
	}
	if err := h.validateDAGConsistency(r.Context(), workItemID, 0, s); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "INCOMPLETE_DAG")
		return
	}
	id, err := h.store.CreateAction(r.Context(), s)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	s.ID = id
	writeJSON(w, http.StatusCreated, s)
}

func (h *Handler) listActions(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}

	actions, err := h.store.ListActionsByWorkItem(r.Context(), workItemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if actions == nil {
		actions = []*core.Action{}
	}
	writeJSON(w, http.StatusOK, actions)
}

// updateActionRequest is the request body for PUT /actions/{actionID}.
// All fields are optional — only provided fields are applied.
type updateActionRequest struct {
	Name                 *string          `json:"name,omitempty"`
	Description          *string          `json:"description,omitempty"`
	Type                 *core.ActionType `json:"type,omitempty"`
	Position             *int             `json:"position,omitempty"`
	DependsOn            *[]int64         `json:"depends_on,omitempty"`
	AgentRole            *string          `json:"agent_role,omitempty"`
	RequiredCapabilities *[]string        `json:"required_capabilities,omitempty"`
	AcceptanceCriteria   *[]string        `json:"acceptance_criteria,omitempty"`
	Timeout              *string          `json:"timeout,omitempty"`
	MaxRetries           *int             `json:"max_retries,omitempty"`
	Config               map[string]any   `json:"config,omitempty"`
}

func (h *Handler) updateAction(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}

	existing, err := h.store.GetAction(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "action not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	// Only allow editing pending actions.
	if existing.Status != core.ActionPending {
		writeError(w, http.StatusConflict, "only pending actions can be edited", "INVALID_STATE")
		return
	}

	var req updateActionRequest
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
	if req.Type != nil {
		existing.Type = *req.Type
	}
	if req.Position != nil {
		if err := h.validateActionPosition(r.Context(), existing.WorkItemID, existing.ID, *req.Position); err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "INVALID_POSITION")
			return
		}
		existing.Position = *req.Position
	}
	if req.DependsOn != nil {
		existing.DependsOn = *req.DependsOn
	}
	if req.AgentRole != nil {
		existing.AgentRole = *req.AgentRole
	}
	if req.RequiredCapabilities != nil {
		existing.RequiredCapabilities = *req.RequiredCapabilities
	}
	if req.AcceptanceCriteria != nil {
		existing.AcceptanceCriteria = *req.AcceptanceCriteria
	}
	if req.Timeout != nil {
		t, err := time.ParseDuration(*req.Timeout)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid timeout duration", "BAD_TIMEOUT")
			return
		}
		existing.Timeout = t
	}
	if req.MaxRetries != nil {
		existing.MaxRetries = *req.MaxRetries
	}
	if req.Config != nil {
		existing.Config = req.Config
	}
	if err := h.validateDAGConsistency(r.Context(), existing.WorkItemID, existing.ID, existing); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "INCOMPLETE_DAG")
		return
	}

	if err := h.store.UpdateAction(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) deleteAction(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}

	// Only allow deleting pending actions.
	existing, err := h.store.GetAction(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "action not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if existing.Status != core.ActionPending {
		writeError(w, http.StatusConflict, "only pending actions can be deleted", "INVALID_STATE")
		return
	}

	if err := h.store.DeleteAction(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "action not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getAction(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}

	s, err := h.store.GetAction(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "action not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handler) resolveCreateActionPosition(ctx context.Context, workItemID int64, requested *int) (int, error) {
	if requested != nil {
		if err := h.validateActionPosition(ctx, workItemID, 0, *requested); err != nil {
			return 0, err
		}
		return *requested, nil
	}

	actions, err := h.store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		return 0, err
	}
	position := 0
	for _, action := range actions {
		if action != nil && action.Position >= position {
			position = action.Position + 1
		}
	}
	return position, nil
}

func actionSetHasDependsOn(actions []*core.Action) bool {
	for _, action := range actions {
		if len(action.DependsOn) > 0 {
			return true
		}
	}
	return false
}

// validateDAGConsistency checks that the full action set for a WorkItem won't
// contain "false roots" — actions that silently lose their Position-based
// ordering when DAG mode is triggered.  An action is a false root when it has
// no DependsOn yet sits at a Position higher than the minimum (meaning it
// previously depended on lower-Position actions in Position mode).
//
// targetID == 0 means the action is not yet persisted (create path).
func (h *Handler) validateDAGConsistency(ctx context.Context, workItemID int64, targetID int64, target *core.Action) error {
	siblings, err := h.store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}

	// Build the projected action set with the pending change applied.
	actions := make([]*core.Action, 0, len(siblings)+1)
	replaced := false
	for _, s := range siblings {
		if targetID != 0 && s.ID == targetID {
			actions = append(actions, target)
			replaced = true
		} else {
			actions = append(actions, s)
		}
	}
	if !replaced {
		actions = append(actions, target)
	}

	currentHasDeps := actionSetHasDependsOn(siblings)
	projectedHasDeps := actionSetHasDependsOn(actions)

	if !currentHasDeps && projectedHasDeps {
		minPos := actions[0].Position
		for _, a := range actions[1:] {
			if a.Position < minPos {
				minPos = a.Position
			}
		}

		// Entering DAG mode from legacy Position mode requires every non-root
		// action to declare explicit dependencies; otherwise lower-position
		// ordering would silently disappear for actions that still have
		// empty depends_on.
		for _, a := range actions {
			if len(a.DependsOn) == 0 && a.Position > minPos {
				return fmt.Errorf(
					"action %q (position %d) has no depends_on and would become a false root in DAG mode; set depends_on on all non-root actions first",
					a.Name, a.Position)
			}
		}
	}

	return flowapp.ValidateActions(actions)
}

func (h *Handler) validateActionPosition(ctx context.Context, workItemID, actionID int64, position int) error {
	if position < 0 {
		return fmt.Errorf("position must be non-negative")
	}
	actions, err := h.store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}
	for _, action := range actions {
		if action == nil || action.ID == actionID {
			continue
		}
		if action.Position == position {
			return fmt.Errorf("position %d is already used by action %d", position, action.ID)
		}
	}
	return nil
}
