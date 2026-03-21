package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	cronapp "github.com/yoke233/zhanggui/internal/application/cron"
	"github.com/yoke233/zhanggui/internal/core"
)

type setupCronRequest struct {
	Schedule     string `json:"schedule"`                // cron expression, e.g. "0 */6 * * *"
	MaxInstances int    `json:"max_instances,omitempty"` // default 1
}

type cronStatusResponse struct {
	WorkItemID    int64  `json:"work_item_id"`
	Enabled       bool   `json:"enabled"`
	IsTemplate    bool   `json:"is_template"`
	Schedule      string `json:"schedule,omitempty"`
	MaxInstances  int    `json:"max_instances,omitempty"`
	LastTriggered string `json:"last_triggered,omitempty"`
}

// setupWorkItemCron enables cron scheduling on a work item (making it a template).
func (h *Handler) setupWorkItemCron(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work_item_id", "INVALID_PARAM")
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

	workItem, err := h.store.GetWorkItem(r.Context(), workItemID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "work item not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	if workItem.Metadata == nil {
		workItem.Metadata = make(map[string]any)
	}
	workItem.Metadata[cronapp.MetaSchedule] = req.Schedule
	workItem.Metadata[cronapp.MetaEnabled] = "true"
	workItem.Metadata[cronapp.MetaTemplateID] = "true"
	if req.MaxInstances > 0 {
		workItem.Metadata[cronapp.MetaMaxInstances] = strconv.Itoa(req.MaxInstances)
	}

	if err := h.store.UpdateWorkItemMetadata(r.Context(), workItemID, workItem.Metadata); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, cronStatusResponse{
		WorkItemID:   workItemID,
		Enabled:      true,
		IsTemplate:   true,
		Schedule:     req.Schedule,
		MaxInstances: req.MaxInstances,
	})
}

// disableWorkItemCron disables cron scheduling on a work item.
func (h *Handler) disableWorkItemCron(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work_item_id", "INVALID_PARAM")
		return
	}

	workItem, err := h.store.GetWorkItem(r.Context(), workItemID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "work item not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	if workItem.Metadata != nil {
		workItem.Metadata[cronapp.MetaEnabled] = "false"
		if err := h.store.UpdateWorkItemMetadata(r.Context(), workItemID, workItem.Metadata); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
			return
		}
	}

	metaTemplateID := ""
	metaSchedule := ""
	if workItem.Metadata != nil {
		if v, ok := workItem.Metadata[cronapp.MetaTemplateID].(string); ok {
			metaTemplateID = v
		}
		if v, ok := workItem.Metadata[cronapp.MetaSchedule].(string); ok {
			metaSchedule = v
		}
	}

	writeJSON(w, http.StatusOK, cronStatusResponse{
		WorkItemID: workItemID,
		Enabled:    false,
		IsTemplate: metaTemplateID == "true",
		Schedule:   metaSchedule,
	})
}

// getWorkItemCronStatus returns the cron status for a work item.
func (h *Handler) getWorkItemCronStatus(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work_item_id", "INVALID_PARAM")
		return
	}

	workItem, err := h.store.GetWorkItem(r.Context(), workItemID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "work item not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	resp := cronStatusResponse{WorkItemID: workItemID}
	if workItem.Metadata != nil {
		if v, ok := workItem.Metadata[cronapp.MetaEnabled].(string); ok {
			resp.Enabled = v == "true"
		}
		if v, ok := workItem.Metadata[cronapp.MetaTemplateID].(string); ok {
			resp.IsTemplate = v == "true"
		}
		if v, ok := workItem.Metadata[cronapp.MetaSchedule].(string); ok {
			resp.Schedule = v
		}
		if v, ok := workItem.Metadata[cronapp.MetaLastTriggered].(string); ok {
			resp.LastTriggered = v
		}
		if v, ok := workItem.Metadata[cronapp.MetaMaxInstances].(string); ok {
			resp.MaxInstances, _ = strconv.Atoi(v)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// listCronWorkItems lists all work items that are configured as cron templates.
func (h *Handler) listCronWorkItems(w http.ResponseWriter, r *http.Request) {
	archived := false
	workItems, err := h.store.ListWorkItems(r.Context(), core.WorkItemFilter{
		Archived: &archived,
		Limit:    200,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var cronWorkItems []cronStatusResponse
	for _, workItem := range workItems {
		if workItem.Metadata == nil {
			continue
		}
		metaTemplateID, _ := workItem.Metadata[cronapp.MetaTemplateID].(string)
		if metaTemplateID != "true" {
			continue
		}
		metaEnabled, _ := workItem.Metadata[cronapp.MetaEnabled].(string)
		metaSchedule, _ := workItem.Metadata[cronapp.MetaSchedule].(string)
		metaLastTriggered, _ := workItem.Metadata[cronapp.MetaLastTriggered].(string)

		resp := cronStatusResponse{
			WorkItemID:    workItem.ID,
			Enabled:       metaEnabled == "true",
			IsTemplate:    true,
			Schedule:      metaSchedule,
			LastTriggered: metaLastTriggered,
		}
		if v, ok := workItem.Metadata[cronapp.MetaMaxInstances].(string); ok {
			resp.MaxInstances, _ = strconv.Atoi(v)
		}
		cronWorkItems = append(cronWorkItems, resp)
	}
	if cronWorkItems == nil {
		cronWorkItems = []cronStatusResponse{}
	}
	writeJSON(w, http.StatusOK, cronWorkItems)
}
