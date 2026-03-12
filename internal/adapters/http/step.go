package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// createStepRequest is the request body for POST /issues/{issueID}/steps.
type createStepRequest struct {
	Name                 string         `json:"name"`
	Description          string         `json:"description,omitempty"`
	Type                 core.StepType  `json:"type"`
	Position             *int           `json:"position,omitempty"`
	AgentRole            string         `json:"agent_role,omitempty"`
	RequiredCapabilities []string       `json:"required_capabilities,omitempty"`
	AcceptanceCriteria   []string       `json:"acceptance_criteria,omitempty"`
	Timeout              string         `json:"timeout,omitempty"` // Go duration string
	MaxRetries           int            `json:"max_retries"`
	Config               map[string]any `json:"config,omitempty"`
}

func (h *Handler) createStep(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	var req createStepRequest
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
	position, err := h.resolveCreateStepPosition(r.Context(), issueID, req.Position)
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

	s := &core.Step{
		IssueID:              issueID,
		Name:                 req.Name,
		Description:          req.Description,
		Type:                 req.Type,
		Status:               core.StepPending,
		Position:             position,
		AgentRole:            req.AgentRole,
		RequiredCapabilities: req.RequiredCapabilities,
		AcceptanceCriteria:   req.AcceptanceCriteria,
		Timeout:              timeout,
		MaxRetries:           req.MaxRetries,
		Config:               req.Config,
	}
	id, err := h.store.CreateStep(r.Context(), s)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	s.ID = id
	writeJSON(w, http.StatusCreated, s)
}

func (h *Handler) listSteps(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	steps, err := h.store.ListStepsByIssue(r.Context(), issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if steps == nil {
		steps = []*core.Step{}
	}
	writeJSON(w, http.StatusOK, steps)
}

// updateStepRequest is the request body for PUT /steps/{stepID}.
// All fields are optional — only provided fields are applied.
type updateStepRequest struct {
	Name                 *string        `json:"name,omitempty"`
	Description          *string        `json:"description,omitempty"`
	Type                 *core.StepType `json:"type,omitempty"`
	Position             *int           `json:"position,omitempty"`
	AgentRole            *string        `json:"agent_role,omitempty"`
	RequiredCapabilities *[]string      `json:"required_capabilities,omitempty"`
	AcceptanceCriteria   *[]string      `json:"acceptance_criteria,omitempty"`
	Timeout              *string        `json:"timeout,omitempty"`
	MaxRetries           *int           `json:"max_retries,omitempty"`
	Config               map[string]any `json:"config,omitempty"`
}

func (h *Handler) updateStep(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "stepID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid step ID", "BAD_ID")
		return
	}

	existing, err := h.store.GetStep(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "step not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	// Only allow editing pending steps.
	if existing.Status != core.StepPending {
		writeError(w, http.StatusConflict, "only pending steps can be edited", "INVALID_STATE")
		return
	}

	var req updateStepRequest
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
		if err := h.validateStepPosition(r.Context(), existing.IssueID, existing.ID, *req.Position); err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "INVALID_POSITION")
			return
		}
		existing.Position = *req.Position
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

	if err := h.store.UpdateStep(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) deleteStep(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "stepID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid step ID", "BAD_ID")
		return
	}

	// Only allow deleting pending steps.
	existing, err := h.store.GetStep(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "step not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if existing.Status != core.StepPending {
		writeError(w, http.StatusConflict, "only pending steps can be deleted", "INVALID_STATE")
		return
	}

	if err := h.store.DeleteStep(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "step not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getStep(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "stepID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid step ID", "BAD_ID")
		return
	}

	s, err := h.store.GetStep(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "step not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handler) resolveCreateStepPosition(ctx context.Context, issueID int64, requested *int) (int, error) {
	if requested != nil {
		if err := h.validateStepPosition(ctx, issueID, 0, *requested); err != nil {
			return 0, err
		}
		return *requested, nil
	}

	steps, err := h.store.ListStepsByIssue(ctx, issueID)
	if err != nil {
		return 0, err
	}
	position := 0
	for _, step := range steps {
		if step != nil && step.Position >= position {
			position = step.Position + 1
		}
	}
	return position, nil
}

func (h *Handler) validateStepPosition(ctx context.Context, issueID, stepID int64, position int) error {
	if position < 0 {
		return fmt.Errorf("position must be non-negative")
	}
	steps, err := h.store.ListStepsByIssue(ctx, issueID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if step == nil || step.ID == stepID {
			continue
		}
		if step.Position == position {
			return fmt.Errorf("position %d is already used by step %d", position, step.ID)
		}
	}
	return nil
}
