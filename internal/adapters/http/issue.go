package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type createIssueRequest struct {
	ProjectID *int64         `json:"project_id,omitempty"`
	Title     string         `json:"title"`
	Body      string         `json:"body,omitempty"`
	Priority  string         `json:"priority,omitempty"`
	Labels    []string       `json:"labels,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type updateIssueRequest struct {
	ProjectID *int64         `json:"project_id,omitempty"`
	Title     *string        `json:"title,omitempty"`
	Body      *string        `json:"body,omitempty"`
	Status    *string        `json:"status,omitempty"`
	Priority  *string        `json:"priority,omitempty"`
	Labels    *[]string      `json:"labels,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
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

	now := time.Now().UTC()
	issue := &core.Issue{
		ProjectID: req.ProjectID,
		Title:     title,
		Body:      strings.TrimSpace(req.Body),
		Status:    core.IssueOpen,
		Priority:  priority,
		Labels:    req.Labels,
		Metadata:  req.Metadata,
		CreatedAt: now,
		UpdatedAt: now,
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
		if _, ok := resolveEnabledSCMRepoFromBindings(r.Context(), bindings); ok {
			if _, err := h.bootstrapPRIssueForIssue(r.Context(), id, bootstrapPRIssueRequest{}); err != nil {
				switch {
				case errors.Is(err, errBootstrapPRIssueMissingProject), errors.Is(err, errBootstrapPRIssueMissingBinding):
					// Ignore when the project does not have an enabled SCM binding.
				default:
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
