package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type createIssueRequest struct {
	ProjectID *int64            `json:"project_id,omitempty"`
	Title     string            `json:"title"`
	Body      string            `json:"body,omitempty"`
	Priority  string            `json:"priority,omitempty"`
	Labels    []string          `json:"labels,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type updateIssueRequest struct {
	ProjectID *int64            `json:"project_id,omitempty"`
	Title     *string           `json:"title,omitempty"`
	Body      *string           `json:"body,omitempty"`
	Status    *string           `json:"status,omitempty"`
	Priority  *string           `json:"priority,omitempty"`
	Labels    *[]string         `json:"labels,omitempty"`
	FlowID    *int64            `json:"flow_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
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

	if req.ProjectID != nil {
		if _, err := h.store.GetProject(r.Context(), *req.ProjectID); err != nil {
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
	if req.FlowID != nil {
		existing.FlowID = req.FlowID
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
