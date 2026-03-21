package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yoke233/zhanggui/internal/application/workitemapp"
	"github.com/yoke233/zhanggui/internal/core"
)

type createWorkItemRequest struct {
	ProjectID         *int64         `json:"project_id,omitempty"`
	ResourceSpaceID *int64         `json:"resource_space_id,omitempty"`
	Title             string         `json:"title"`
	Body              string         `json:"body,omitempty"`
	Priority          string         `json:"priority,omitempty"`
	Labels            []string       `json:"labels,omitempty"`
	DependsOn         []int64        `json:"depends_on,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type updateWorkItemRequest struct {
	ProjectID         *int64         `json:"project_id,omitempty"`
	ResourceSpaceID *int64         `json:"resource_space_id,omitempty"`
	Title             *string        `json:"title,omitempty"`
	Body              *string        `json:"body,omitempty"`
	Status            *string        `json:"status,omitempty"`
	Priority          *string        `json:"priority,omitempty"`
	Labels            *[]string      `json:"labels,omitempty"`
	DependsOn         *[]int64       `json:"depends_on,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

func (h *Handler) createWorkItem(w http.ResponseWriter, r *http.Request) {
	var req createWorkItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	workItem, err := h.workItemService().CreateWorkItem(r.Context(), workitemapp.CreateWorkItemInput{
		ProjectID:         req.ProjectID,
		ResourceSpaceID: req.ResourceSpaceID,
		Title:             req.Title,
		Body:              req.Body,
		Priority:          req.Priority,
		Labels:            req.Labels,
		DependsOn:         req.DependsOn,
		Metadata:          req.Metadata,
	})
	if err != nil {
		if writeWorkItemAppError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	writeJSON(w, http.StatusCreated, workItem)
}

func (h *Handler) getWorkItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}
	workItem, err := h.store.GetWorkItem(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "work item not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, workItem)
}

func (h *Handler) listWorkItems(w http.ResponseWriter, r *http.Request) {
	filter := core.WorkItemFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	if projectID, ok := queryInt64(r, "project_id"); ok {
		filter.ProjectID = &projectID
	}
	if s := r.URL.Query().Get("status"); s != "" {
		status := core.WorkItemStatus(s)
		filter.Status = &status
	}
	if s := r.URL.Query().Get("priority"); s != "" {
		priority := core.WorkItemPriority(s)
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

	workItems, err := h.store.ListWorkItems(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if workItems == nil {
		workItems = []*core.WorkItem{}
	}
	writeJSON(w, http.StatusOK, workItems)
}

func (h *Handler) updateWorkItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}

	var req updateWorkItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	updated, err := h.workItemService().UpdateWorkItem(r.Context(), workitemapp.UpdateWorkItemInput{
		ID:                id,
		ProjectID:         req.ProjectID,
		ResourceSpaceID: req.ResourceSpaceID,
		Title:             req.Title,
		Body:              req.Body,
		Status:            req.Status,
		Priority:          req.Priority,
		Labels:            req.Labels,
		DependsOn:         req.DependsOn,
		Metadata:          req.Metadata,
	})
	if err != nil {
		if writeWorkItemAppError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteWorkItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}

	if err := h.workItemService().DeleteWorkItem(r.Context(), id); err != nil {
		if writeWorkItemAppError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) archiveWorkItem(w http.ResponseWriter, r *http.Request) {
	h.setWorkItemArchived(w, r, true)
}

func (h *Handler) unarchiveWorkItem(w http.ResponseWriter, r *http.Request) {
	h.setWorkItemArchived(w, r, false)
}

func (h *Handler) setWorkItemArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	id, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}

	workItem, err := h.workItemService().SetArchived(r.Context(), id, archived)
	if err != nil {
		if writeWorkItemAppError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, workItem)
}

// runWorkItem triggers an async run for a work item. Returns immediately.
// If a scheduler is configured, the work item is queued; otherwise it starts directly.
func (h *Handler) runWorkItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}

	result, err := h.workItemService().RunWorkItem(r.Context(), id)
	if err != nil {
		if writeWorkItemAppError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "SCHEDULER_ERROR")
		return
	}
	if result.Queued {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"work_item_id": id,
			"status":       "queued",
			"message":      result.Message,
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"work_item_id": id,
		"status":       "accepted",
		"message":      result.Message,
	})
}

func (h *Handler) cancelWorkItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}

	if err := h.workItemService().CancelWorkItem(r.Context(), id); err != nil {
		if writeWorkItemAppError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "ENGINE_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"work_item_id": id,
		"status":       "cancelled",
	})
}
