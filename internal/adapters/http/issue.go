package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type createIssueRequest struct {
	ProjectID         *int64         `json:"project_id,omitempty"`
	ResourceBindingID *int64         `json:"resource_binding_id,omitempty"`
	Title             string         `json:"title"`
	Body              string         `json:"body,omitempty"`
	Priority          string         `json:"priority,omitempty"`
	Labels            []string       `json:"labels,omitempty"`
	DependsOn         []int64        `json:"depends_on,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type updateIssueRequest struct {
	ProjectID         *int64         `json:"project_id,omitempty"`
	ResourceBindingID *int64         `json:"resource_binding_id,omitempty"`
	Title             *string        `json:"title,omitempty"`
	Body              *string        `json:"body,omitempty"`
	Status            *string        `json:"status,omitempty"`
	Priority          *string        `json:"priority,omitempty"`
	Labels            *[]string      `json:"labels,omitempty"`
	DependsOn         *[]int64       `json:"depends_on,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

func (h *Handler) createIssue(w http.ResponseWriter, r *http.Request) {
	var req createIssueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "MISSING_TITLE")
		return
	}

	var project *core.Project
	if req.ProjectID != nil {
		var err error
		project, err = h.store.GetProject(r.Context(), *req.ProjectID)
		if err != nil {
			if err == core.ErrNotFound {
				writeError(w, http.StatusNotFound, "project not found", "PROJECT_NOT_FOUND")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
	}

	priority := core.IssuePriority(strings.TrimSpace(req.Priority))
	if priority == "" {
		priority = core.PriorityMedium
	}
	if _, err := validateIssueResourceBinding(r.Context(), h.store, req.ProjectID, req.ResourceBindingID); err != nil {
		switch {
		case errors.Is(err, core.ErrNotFound):
			writeError(w, http.StatusNotFound, "resource binding not found", "RESOURCE_BINDING_NOT_FOUND")
		default:
			writeError(w, http.StatusBadRequest, err.Error(), "INVALID_RESOURCE_BINDING")
		}
		return
	}
	if err := validateIssueDependencies(r.Context(), h.store, 0, req.ProjectID, req.DependsOn); err != nil {
		switch {
		case errors.Is(err, core.ErrNotFound):
			writeError(w, http.StatusNotFound, "dependency issue not found", "ISSUE_DEPENDENCY_NOT_FOUND")
		default:
			writeError(w, http.StatusBadRequest, err.Error(), "INVALID_ISSUE_DEPENDENCY")
		}
		return
	}

	now := time.Now().UTC()
	issue := &core.Issue{
		ProjectID:         req.ProjectID,
		ResourceBindingID: req.ResourceBindingID,
		Title:             title,
		Body:              strings.TrimSpace(req.Body),
		Status:            core.IssueOpen,
		Priority:          priority,
		Labels:            req.Labels,
		DependsOn:         req.DependsOn,
		Metadata:          req.Metadata,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	id, err := h.store.CreateIssue(r.Context(), issue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	issue.ID = id

	if project != nil {
		bindings, err := h.store.ListResourceBindings(r.Context(), project.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
		bootstrapBindings, filterErr := bindingsForIssue(issue, bindings)
		if filterErr != nil && !errors.Is(filterErr, errBootstrapPRIssueAmbiguousBinding) && !errors.Is(filterErr, errBootstrapPRIssueMissingBinding) {
			writeError(w, http.StatusInternalServerError, filterErr.Error(), "STORE_ERROR")
			return
		}
		if _, ok := resolveEnabledSCMRepoFromBindings(r.Context(), bootstrapBindings); ok {
			if _, err := h.bootstrapPRIssueForIssue(r.Context(), id, bootstrapPRIssueRequest{}); err != nil {
				switch {
				case errors.Is(err, errBootstrapPRIssueMissingProject), errors.Is(err, errBootstrapPRIssueMissingBinding), errors.Is(err, errBootstrapPRIssueAmbiguousBinding):
					// Ignore when the project does not have an enabled SCM binding.
				default:
					if rollbackErr := rollbackIssueCreation(r.Context(), h.store, id); rollbackErr != nil {
						writeError(w, http.StatusInternalServerError, fmt.Sprintf("%s; rollback failed: %v", err.Error(), rollbackErr), "AUTO_SCM_ISSUE_BOOTSTRAP_FAILED")
						return
					}
					writeError(w, http.StatusInternalServerError, err.Error(), "AUTO_SCM_ISSUE_BOOTSTRAP_FAILED")
					return
				}
			}
		}
	}

	writeJSON(w, http.StatusCreated, issue)
}

func (h *Handler) getIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}
	issue, err := h.store.GetIssue(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) listIssues(w http.ResponseWriter, r *http.Request) {
	filter := core.IssueFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	if projectID, ok := queryInt64(r, "project_id"); ok {
		filter.ProjectID = &projectID
	}
	if s := r.URL.Query().Get("status"); s != "" {
		status := core.IssueStatus(s)
		filter.Status = &status
	}
	if s := r.URL.Query().Get("priority"); s != "" {
		priority := core.IssuePriority(s)
		filter.Priority = &priority
	}
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("archived"))) {
	case "":
		archived := false
		filter.Archived = &archived
	case "true":
		archived := true
		filter.Archived = &archived
	case "false":
		archived := false
		filter.Archived = &archived
	case "all":
		// no filter
	default:
		writeError(w, http.StatusBadRequest, "invalid archived filter", "BAD_ARCHIVED_FILTER")
		return
	}

	issues, err := h.store.ListIssues(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if issues == nil {
		issues = []*core.Issue{}
	}
	writeJSON(w, http.StatusOK, issues)
}

func (h *Handler) updateIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	existing, err := h.store.GetIssue(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req updateIssueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	if req.ProjectID != nil {
		if _, err := h.store.GetProject(r.Context(), *req.ProjectID); err != nil {
			if err == core.ErrNotFound {
				writeError(w, http.StatusNotFound, "project not found", "PROJECT_NOT_FOUND")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
		existing.ProjectID = req.ProjectID
	}
	targetProjectID := existing.ProjectID
	if req.ProjectID != nil {
		targetProjectID = req.ProjectID
	}
	if req.ResourceBindingID != nil {
		if _, err := validateIssueResourceBinding(r.Context(), h.store, targetProjectID, req.ResourceBindingID); err != nil {
			switch {
			case errors.Is(err, core.ErrNotFound):
				writeError(w, http.StatusNotFound, "resource binding not found", "RESOURCE_BINDING_NOT_FOUND")
			default:
				writeError(w, http.StatusBadRequest, err.Error(), "INVALID_RESOURCE_BINDING")
			}
			return
		}
		existing.ResourceBindingID = req.ResourceBindingID
	}
	if req.Title != nil {
		existing.Title = strings.TrimSpace(*req.Title)
	}
	if req.Body != nil {
		existing.Body = strings.TrimSpace(*req.Body)
	}
	if req.Status != nil {
		existing.Status = core.IssueStatus(strings.TrimSpace(*req.Status))
	}
	if req.Priority != nil {
		existing.Priority = core.IssuePriority(strings.TrimSpace(*req.Priority))
	}
	if req.Labels != nil {
		existing.Labels = *req.Labels
	}
	if req.DependsOn != nil {
		if err := validateIssueDependencies(r.Context(), h.store, existing.ID, targetProjectID, *req.DependsOn); err != nil {
			switch {
			case errors.Is(err, core.ErrNotFound):
				writeError(w, http.StatusNotFound, "dependency issue not found", "ISSUE_DEPENDENCY_NOT_FOUND")
			default:
				writeError(w, http.StatusBadRequest, err.Error(), "INVALID_ISSUE_DEPENDENCY")
			}
			return
		}
		existing.DependsOn = *req.DependsOn
	}
	if req.Metadata != nil {
		existing.Metadata = req.Metadata
	}

	if err := h.store.UpdateIssue(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) deleteIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	if err := h.store.DeleteIssue(r.Context(), id); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) archiveIssue(w http.ResponseWriter, r *http.Request) {
	h.setIssueArchived(w, r, true)
}

func (h *Handler) unarchiveIssue(w http.ResponseWriter, r *http.Request) {
	h.setIssueArchived(w, r, false)
}

func (h *Handler) setIssueArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	id, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	issue, err := h.store.GetIssue(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if archived {
		switch issue.Status {
		case core.IssueQueued, core.IssueRunning, core.IssueBlocked:
			writeError(w, http.StatusConflict, "active issue cannot be archived", "INVALID_STATE")
			return
		}
	}

	if err := h.store.SetIssueArchived(r.Context(), id, archived); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
			return
		}
		if errors.Is(err, core.ErrInvalidTransition) {
			writeError(w, http.StatusConflict, "issue cannot be archived in current state", "INVALID_STATE")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	issue, err = h.store.GetIssue(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

// runIssue triggers async execution of an issue. Returns immediately.
// If a scheduler is configured, the issue is queued; otherwise it runs directly.
func (h *Handler) runIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	// Verify the issue has at least one step before allowing execution.
	steps, err := h.store.ListStepsByIssue(r.Context(), id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if len(steps) == 0 {
		writeError(w, http.StatusBadRequest, "issue has no steps; add at least one step before running", "NO_STEPS")
		return
	}

	// If scheduler is available, submit to queue.
	if h.scheduler != nil {
		if err := h.scheduler.Submit(r.Context(), id); err != nil {
			switch {
			case errors.Is(err, core.ErrNotFound):
				writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
				return
			case errors.Is(err, core.ErrInvalidTransition):
				issue, getErr := h.store.GetIssue(r.Context(), id)
				if getErr == nil && issue.ArchivedAt != nil {
					writeError(w, http.StatusConflict, "archived issue cannot be executed", "ISSUE_ARCHIVED")
					return
				}
				writeError(w, http.StatusConflict, "issue is not in a runnable state", "INVALID_STATE")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error(), "SCHEDULER_ERROR")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"issue_id": id,
			"status":   "queued",
			"message":  "issue queued for execution",
		})
		return
	}

	if err := h.store.PrepareIssueRun(r.Context(), id, core.IssueQueued); err != nil {
		switch {
		case errors.Is(err, core.ErrNotFound):
			writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
			return
		case errors.Is(err, core.ErrInvalidTransition):
			issue, getErr := h.store.GetIssue(r.Context(), id)
			if getErr == nil && issue.ArchivedAt != nil {
				writeError(w, http.StatusConflict, "archived issue cannot be executed", "ISSUE_ARCHIVED")
				return
			}
			writeError(w, http.StatusConflict, "issue is not in a runnable state", "INVALID_STATE")
			return
		default:
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
	}

	// Fallback: run directly in background goroutine.
	go func() {
		ctx := context.Background()
		if err := h.engine.Run(ctx, id); err != nil {
			h.bus.Publish(ctx, core.Event{
				Type:      core.EventIssueFailed,
				IssueID:   id,
				Timestamp: time.Now().UTC(),
				Data:      map[string]any{"error": err.Error()},
			})
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"issue_id": id,
		"status":   "accepted",
		"message":  "issue execution started",
	})
}

func (h *Handler) cancelIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	// If scheduler is available, cancel via scheduler (handles both queued and running).
	var err error
	if h.scheduler != nil {
		err = h.scheduler.Cancel(r.Context(), id)
	} else {
		err = h.engine.Cancel(r.Context(), id)
	}

	if err != nil {
		if err == core.ErrInvalidTransition {
			writeError(w, http.StatusConflict, "issue cannot be cancelled in current state", "INVALID_STATE")
			return
		}
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "ENGINE_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"issue_id": id,
		"status":   "cancelled",
	})
}

func validateIssueResourceBinding(ctx context.Context, store Store, projectID *int64, bindingID *int64) (*core.ResourceBinding, error) {
	if bindingID == nil {
		return nil, nil
	}
	if projectID == nil {
		return nil, fmt.Errorf("resource binding requires project_id")
	}
	binding, err := store.GetResourceBinding(ctx, *bindingID)
	if err != nil {
		return nil, err
	}
	if binding.ProjectID != *projectID {
		return nil, fmt.Errorf("resource binding %d does not belong to project %d", *bindingID, *projectID)
	}
	return binding, nil
}

func validateIssueDependencies(ctx context.Context, store Store, issueID int64, projectID *int64, deps []int64) error {
	seen := make(map[int64]struct{}, len(deps))
	for _, depID := range deps {
		if depID <= 0 {
			return fmt.Errorf("dependency issue id must be positive")
		}
		if depID == issueID && issueID != 0 {
			return fmt.Errorf("issue cannot depend on itself")
		}
		if _, ok := seen[depID]; ok {
			return fmt.Errorf("duplicate dependency issue id %d", depID)
		}
		seen[depID] = struct{}{}

		depIssue, err := store.GetIssue(ctx, depID)
		if err != nil {
			return err
		}
		if projectID != nil && depIssue.ProjectID != nil && *depIssue.ProjectID != *projectID {
			return fmt.Errorf("dependency issue %d belongs to a different project", depID)
		}
	}
	return nil
}

func rollbackIssueCreation(ctx context.Context, store Store, issueID int64) error {
	steps, err := store.ListStepsByIssue(ctx, issueID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if step == nil {
			continue
		}
		if err := store.DeleteStep(ctx, step.ID); err != nil && !errors.Is(err, core.ErrNotFound) {
			return err
		}
	}
	if err := store.DeleteIssue(ctx, issueID); err != nil && !errors.Is(err, core.ErrNotFound) {
		return err
	}
	return nil
}
