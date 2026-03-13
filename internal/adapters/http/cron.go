package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	cronapp "github.com/yoke233/ai-workflow/internal/application/cron"
	"github.com/yoke233/ai-workflow/internal/core"
)

type setupCronRequest struct {
	Schedule     string `json:"schedule"`                // cron expression, e.g. "0 */6 * * *"
	MaxInstances int    `json:"max_instances,omitempty"`  // default 1
}

type cronStatusResponse struct {
	IssueID      int64  `json:"issue_id"`
	Enabled      bool   `json:"enabled"`
	IsTemplate   bool   `json:"is_template"`
	Schedule     string `json:"schedule,omitempty"`
	MaxInstances int    `json:"max_instances,omitempty"`
	LastTriggered string `json:"last_triggered,omitempty"`
}

// setupWorkItemCron enables cron scheduling on an issue (making it a template).
func (h *Handler) setupWorkItemCron(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue_id", "INVALID_PARAM")
		return
	}

	var req setupCronRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_BODY")
		return
	}
	if req.Schedule == "" {
		writeError(w, http.StatusBadRequest, "schedule is required", "MISSING_SCHEDULE")
		return
	}

	issue, err := h.store.GetWorkItem(r.Context(), issueID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	if issue.Metadata == nil {
		issue.Metadata = make(map[string]any)
	}
	issue.Metadata[cronapp.MetaSchedule] = req.Schedule
	issue.Metadata[cronapp.MetaEnabled] = "true"
	issue.Metadata[cronapp.MetaTemplateID] = "true"
	if req.MaxInstances > 0 {
		issue.Metadata[cronapp.MetaMaxInstances] = strconv.Itoa(req.MaxInstances)
	}

	if err := h.store.UpdateWorkItemMetadata(r.Context(), issueID, issue.Metadata); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, cronStatusResponse{
		IssueID:      issueID,
		Enabled:      true,
		IsTemplate:   true,
		Schedule:     req.Schedule,
		MaxInstances: req.MaxInstances,
	})
}

// disableWorkItemCron disables cron scheduling on an issue.
func (h *Handler) disableWorkItemCron(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue_id", "INVALID_PARAM")
		return
	}

	issue, err := h.store.GetWorkItem(r.Context(), issueID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	if issue.Metadata != nil {
		issue.Metadata[cronapp.MetaEnabled] = "false"
		if err := h.store.UpdateWorkItemMetadata(r.Context(), issueID, issue.Metadata); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
	}

	metaTemplateID := ""
	metaSchedule := ""
	if issue.Metadata != nil {
		if v, ok := issue.Metadata[cronapp.MetaTemplateID].(string); ok {
			metaTemplateID = v
		}
		if v, ok := issue.Metadata[cronapp.MetaSchedule].(string); ok {
			metaSchedule = v
		}
	}

	writeJSON(w, http.StatusOK, cronStatusResponse{
		IssueID:    issueID,
		Enabled:    false,
		IsTemplate: metaTemplateID == "true",
		Schedule:   metaSchedule,
	})
}

// getWorkItemCronStatus returns the cron status for an issue.
func (h *Handler) getWorkItemCronStatus(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue_id", "INVALID_PARAM")
		return
	}

	issue, err := h.store.GetWorkItem(r.Context(), issueID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	resp := cronStatusResponse{IssueID: issueID}
	if issue.Metadata != nil {
		if v, ok := issue.Metadata[cronapp.MetaEnabled].(string); ok {
			resp.Enabled = v == "true"
		}
		if v, ok := issue.Metadata[cronapp.MetaTemplateID].(string); ok {
			resp.IsTemplate = v == "true"
		}
		if v, ok := issue.Metadata[cronapp.MetaSchedule].(string); ok {
			resp.Schedule = v
		}
		if v, ok := issue.Metadata[cronapp.MetaLastTriggered].(string); ok {
			resp.LastTriggered = v
		}
		if v, ok := issue.Metadata[cronapp.MetaMaxInstances].(string); ok {
			resp.MaxInstances, _ = strconv.Atoi(v)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// listCronWorkItems lists all issues that are configured as cron templates.
func (h *Handler) listCronWorkItems(w http.ResponseWriter, r *http.Request) {
	archived := false
	issues, err := h.store.ListWorkItems(r.Context(), core.WorkItemFilter{
		Archived: &archived,
		Limit:    200,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var cronWorkItems []cronStatusResponse
	for _, iss := range issues {
		if iss.Metadata == nil {
			continue
		}
		metaTemplateID, _ := iss.Metadata[cronapp.MetaTemplateID].(string)
		if metaTemplateID != "true" {
			continue
		}
		metaEnabled, _ := iss.Metadata[cronapp.MetaEnabled].(string)
		metaSchedule, _ := iss.Metadata[cronapp.MetaSchedule].(string)
		metaLastTriggered, _ := iss.Metadata[cronapp.MetaLastTriggered].(string)

		resp := cronStatusResponse{
			IssueID:       iss.ID,
			Enabled:       metaEnabled == "true",
			IsTemplate:    true,
			Schedule:      metaSchedule,
			LastTriggered: metaLastTriggered,
		}
		if v, ok := iss.Metadata[cronapp.MetaMaxInstances].(string); ok {
			resp.MaxInstances, _ = strconv.Atoi(v)
		}
		cronWorkItems = append(cronWorkItems, resp)
	}
	if cronWorkItems == nil {
		cronWorkItems = []cronStatusResponse{}
	}
	writeJSON(w, http.StatusOK, cronWorkItems)
}
