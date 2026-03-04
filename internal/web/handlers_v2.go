package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

type v2IssueHandlers struct {
	store core.Store
}

type v2RunHandlers struct {
	store core.Store
}

type workflowProfileDescriptor struct {
	Type          core.WorkflowProfileType `json:"type"`
	SLAMinutes    int                      `json:"sla_minutes"`
	ReviewerCount int                      `json:"reviewer_count"`
	Description   string                   `json:"description"`
}

type workflowProfileListResponse struct {
	Items []workflowProfileDescriptor `json:"items"`
}

type workflowRunResponse struct {
	ID         string                   `json:"id"`
	ProjectID  string                   `json:"project_id"`
	IssueID    string                   `json:"issue_id,omitempty"`
	Profile    core.WorkflowProfileType `json:"profile"`
	Status     core.WorkflowRunStatus   `json:"status"`
	Error      string                   `json:"error,omitempty"`
	CreatedAt  string                   `json:"created_at,omitempty"`
	UpdatedAt  string                   `json:"updated_at,omitempty"`
	StartedAt  string                   `json:"started_at,omitempty"`
	FinishedAt string                   `json:"finished_at,omitempty"`
}

type workflowRunListResponse struct {
	Items  []workflowRunResponse `json:"items"`
	Total  int                   `json:"total"`
	Offset int                   `json:"offset"`
}

func registerV2Routes(
	r chi.Router,
	store core.Store,
	issueManager IssueManager,
	issueParserRoleID string,
	executor RunExecutor,
	stageRoleBindings map[string]string,
) {
	_ = issueManager
	_ = issueParserRoleID
	_ = executor
	_ = stageRoleBindings

	issueHandlers := &v2IssueHandlers{store: store}
	runHandlers := &v2RunHandlers{store: store}

	r.Get("/issues", issueHandlers.listIssues)
	r.Get("/issues/{id}", issueHandlers.getIssue)

	r.Get("/workflow-profiles", handleListWorkflowProfiles)
	r.Get("/workflow-profiles/{type}", handleGetWorkflowProfile)

	r.Get("/runs", runHandlers.listRuns)
	r.Get("/runs/{id}", runHandlers.getRun)
}

func (h *v2IssueHandlers) listIssues(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project_id is required", "PROJECT_ID_REQUIRED")
		return
	}
	if _, err := h.store.GetProject(projectID); err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", projectID), "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return
	}

	limit, offset, err := parsePaginationParams(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_QUERY_PARAM")
		return
	}

	items, total, err := h.store.ListIssues(projectID, core.IssueFilter{
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list issues", "LIST_ISSUES_FAILED")
		return
	}

	writeJSON(w, http.StatusOK, issueListResponse{
		Items:  normalizeIssuesForAPI(items),
		Total:  total,
		Offset: offset,
	})
}

func (h *v2IssueHandlers) getIssue(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	issueID := strings.TrimSpace(chi.URLParam(r, "id"))
	if issueID == "" {
		writeAPIError(w, http.StatusBadRequest, "issue id is required", "ISSUE_ID_REQUIRED")
		return
	}

	issue, err := h.store.GetIssue(issueID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("issue %s not found", issueID), "ISSUE_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load issue", "GET_ISSUE_FAILED")
		return
	}

	normalized := normalizeIssueForAPI(issue)
	if normalized == nil {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("issue %s not found", issueID), "ISSUE_NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, normalized)
}

func handleListWorkflowProfiles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, workflowProfileListResponse{
		Items: []workflowProfileDescriptor{
			{
				Type:          core.WorkflowProfileNormal,
				SLAMinutes:    core.MaxWorkflowProfileSLAMinutes,
				ReviewerCount: 1,
				Description:   "default coding and review flow",
			},
			{
				Type:          core.WorkflowProfileStrict,
				SLAMinutes:    core.MaxWorkflowProfileSLAMinutes,
				ReviewerCount: 3,
				Description:   "strict review flow with three parallel reviewers",
			},
			{
				Type:          core.WorkflowProfileFastRelease,
				SLAMinutes:    core.MaxWorkflowProfileSLAMinutes,
				ReviewerCount: 1,
				Description:   "lightweight review for fast release",
			},
		},
	})
}

func handleGetWorkflowProfile(w http.ResponseWriter, r *http.Request) {
	rawType := strings.TrimSpace(strings.ToLower(chi.URLParam(r, "type")))
	profile := core.WorkflowProfileType(rawType)
	if err := profile.Validate(); err != nil {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("workflow profile %s not found", rawType), "WORKFLOW_PROFILE_NOT_FOUND")
		return
	}

	req := workflowProfileByType(profile)
	writeJSON(w, http.StatusOK, req)
}

func workflowProfileByType(profile core.WorkflowProfileType) workflowProfileDescriptor {
	switch profile {
	case core.WorkflowProfileStrict:
		return workflowProfileDescriptor{
			Type:          profile,
			SLAMinutes:    core.MaxWorkflowProfileSLAMinutes,
			ReviewerCount: 3,
			Description:   "strict review flow with three parallel reviewers",
		}
	case core.WorkflowProfileFastRelease:
		return workflowProfileDescriptor{
			Type:          profile,
			SLAMinutes:    core.MaxWorkflowProfileSLAMinutes,
			ReviewerCount: 1,
			Description:   "lightweight review for fast release",
		}
	default:
		return workflowProfileDescriptor{
			Type:          core.WorkflowProfileNormal,
			SLAMinutes:    core.MaxWorkflowProfileSLAMinutes,
			ReviewerCount: 1,
			Description:   "default coding and review flow",
		}
	}
}

func (h *v2RunHandlers) listRuns(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project_id is required", "PROJECT_ID_REQUIRED")
		return
	}
	if _, err := h.store.GetProject(projectID); err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", projectID), "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return
	}

	limit, offset, err := parsePaginationParams(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_QUERY_PARAM")
		return
	}

	items, err := h.store.ListRuns(projectID, core.RunFilter{
		Status: core.RunStatus(strings.TrimSpace(r.URL.Query().Get("status"))),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list runs", "LIST_RUNS_FAILED")
		return
	}

	runs := make([]workflowRunResponse, 0, len(items))
	for i := range items {
		runs = append(runs, toWorkflowRunResponse(items[i]))
	}

	writeJSON(w, http.StatusOK, workflowRunListResponse{
		Items:  runs,
		Total:  len(runs),
		Offset: offset,
	})
}

func (h *v2RunHandlers) getRun(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	runID := strings.TrimSpace(chi.URLParam(r, "id"))
	if runID == "" {
		writeAPIError(w, http.StatusBadRequest, "run id is required", "RUN_ID_REQUIRED")
		return
	}

	Run, err := h.store.GetRun(runID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("run %s not found", runID), "RUN_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load run", "GET_RUN_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, toWorkflowRunResponse(*Run))
}

func toWorkflowRunResponse(p core.Run) workflowRunResponse {
	profile := core.WorkflowProfileNormal
	if raw, ok := p.Config["workflow_profile"]; ok {
		if text, ok := raw.(string); ok {
			candidate := core.WorkflowProfileType(strings.TrimSpace(strings.ToLower(text)))
			if candidate.Validate() == nil {
				profile = candidate
			}
		}
	}
	return workflowRunResponse{
		ID:         p.ID,
		ProjectID:  p.ProjectID,
		IssueID:    strings.TrimSpace(p.IssueID),
		Profile:    profile,
		Status:     mapRunStatusToWorkflowRunStatus(p.Status),
		Error:      strings.TrimSpace(p.ErrorMessage),
		CreatedAt:  toRFC3339OrEmpty(p.CreatedAt),
		UpdatedAt:  toRFC3339OrEmpty(p.UpdatedAt),
		StartedAt:  toRFC3339OrEmpty(p.StartedAt),
		FinishedAt: toRFC3339OrEmpty(p.FinishedAt),
	}
}

func mapRunStatusToWorkflowRunStatus(status core.RunStatus) core.WorkflowRunStatus {
	switch status {
	case core.StatusCreated:
		return core.WorkflowRunStatusCreated
	case core.StatusRunning:
		return core.WorkflowRunStatusRunning
	case core.StatusWaitingReview:
		return core.WorkflowRunStatusWaitingReview
	case core.StatusDone:
		return core.WorkflowRunStatusDone
	case core.StatusFailed:
		return core.WorkflowRunStatusFailed
	default:
		return core.WorkflowRunStatusFailed
	}
}

func toRFC3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
