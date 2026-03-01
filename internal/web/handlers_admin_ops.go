package web

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/user/ai-workflow/internal/core"
	ghwebhook "github.com/user/ai-workflow/internal/github"
	"github.com/user/ai-workflow/internal/observability"
)

type adminOpsHandlers struct {
	store      core.Store
	adminToken string
	replayer   WebhookDeliveryReplayer
}

type adminTaskOperationRequest struct {
	TaskID  string `json:"task_id"`
	TraceID string `json:"trace_id"`
	Reason  string `json:"reason"`
}

type adminReplayRequest struct {
	DeliveryID string `json:"delivery_id"`
	PipelineID string `json:"pipeline_id"`
	TraceID    string `json:"trace_id"`
}

func registerAdminOpsRoutes(r chi.Router, store core.Store, adminToken string, replayer WebhookDeliveryReplayer) {
	h := &adminOpsHandlers{
		store:      store,
		adminToken: strings.TrimSpace(adminToken),
		replayer:   replayer,
	}
	r.Post("/admin/ops/force-ready", h.handleForceReady)
	r.Post("/admin/ops/force-unblock", h.handleForceUnblock)
	r.Post("/admin/ops/replay-delivery", h.handleReplayDelivery)
}

func (h *adminOpsHandlers) handleForceReady(w http.ResponseWriter, r *http.Request) {
	h.handleTaskStateMutation(w, r, "force_ready", core.ItemReady)
}

func (h *adminOpsHandlers) handleForceUnblock(w http.ResponseWriter, r *http.Request) {
	h.handleTaskStateMutation(w, r, "force_unblock", core.ItemReady)
}

func (h *adminOpsHandlers) handleTaskStateMutation(
	w http.ResponseWriter,
	r *http.Request,
	action string,
	targetStatus core.TaskItemStatus,
) {
	if !h.isAuthorized(r) {
		writeAPIError(w, http.StatusUnauthorized, "admin operation unauthorized", "ADMIN_UNAUTHORIZED")
		return
	}
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	var req adminTaskOperationRequest
	if err := decodeJSONBodyStrict(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		writeAPIError(w, http.StatusBadRequest, "task_id is required", "TASK_ID_REQUIRED")
		return
	}

	task, err := h.store.GetTaskItem(taskID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, "task item not found", "TASK_ITEM_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load task item", "GET_TASK_ITEM_FAILED")
		return
	}

	task.Status = targetStatus
	if err := h.store.SaveTaskItem(task); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to update task item", "SAVE_TASK_ITEM_FAILED")
		return
	}

	traceID := resolveAdminTraceID(req.TraceID, r)
	auditMessage := "trace_id=" + traceID
	if reason := strings.TrimSpace(req.Reason); reason != "" {
		auditMessage += " reason=" + reason
	}
	if err := h.store.RecordAction(core.HumanAction{
		PipelineID: task.PipelineID,
		Action:     action,
		Message:    auditMessage,
		Source:     "admin",
		UserID:     "admin",
	}); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to record admin audit", "AUDIT_RECORD_FAILED")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"task_id":  task.ID,
		"trace_id": traceID,
	})
}

func (h *adminOpsHandlers) handleReplayDelivery(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorized(r) {
		writeAPIError(w, http.StatusUnauthorized, "admin operation unauthorized", "ADMIN_UNAUTHORIZED")
		return
	}
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}
	if h.replayer == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "webhook dispatcher is not configured", "DISPATCHER_UNAVAILABLE")
		return
	}

	var req adminReplayRequest
	if err := decodeJSONBodyStrict(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}
	deliveryID := strings.TrimSpace(req.DeliveryID)
	if deliveryID == "" {
		writeAPIError(w, http.StatusBadRequest, "delivery_id is required", "DELIVERY_ID_REQUIRED")
		return
	}

	replayed, err := h.replayer.ReplayByDeliveryID(r.Context(), deliveryID)
	if err != nil {
		if errors.Is(err, ghwebhook.ErrDLQEntryNotFound) {
			writeAPIError(w, http.StatusNotFound, "delivery id not found in dlq", "DELIVERY_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to replay delivery", "REPLAY_DELIVERY_FAILED")
		return
	}

	traceID := resolveAdminTraceID(req.TraceID, r)
	if pipelineID := strings.TrimSpace(req.PipelineID); pipelineID != "" {
		if err := h.store.RecordAction(core.HumanAction{
			PipelineID: pipelineID,
			Action:     "replay_delivery",
			Message:    "trace_id=" + traceID + " delivery_id=" + deliveryID,
			Source:     "admin",
			UserID:     "admin",
		}); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to record admin audit", "AUDIT_RECORD_FAILED")
			return
		}
	}

	status := "replayed"
	if !replayed {
		status = "already_replayed"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      status,
		"delivery_id": deliveryID,
		"trace_id":    traceID,
	})
}

func (h *adminOpsHandlers) isAuthorized(r *http.Request) bool {
	if h == nil {
		return false
	}
	headerToken := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	if headerToken != "" && h.adminToken != "" {
		if subtle.ConstantTimeCompare([]byte(headerToken), []byte(h.adminToken)) == 1 {
			return true
		}
	}
	host := remoteHost(r.RemoteAddr)
	return isLoopbackHost(host)
}

func resolveAdminTraceID(raw string, r *http.Request) string {
	traceID := strings.TrimSpace(raw)
	if traceID != "" {
		return traceID
	}
	if headerTraceID := strings.TrimSpace(r.Header.Get("X-Trace-ID")); headerTraceID != "" {
		return headerTraceID
	}
	return observability.NewTraceID()
}

func decodeJSONBodyStrict(r *http.Request, out any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func remoteHost(remoteAddr string) string {
	trimmed := strings.TrimSpace(remoteAddr)
	if trimmed == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(trimmed)
	if err != nil {
		return trimmed
	}
	return host
}

func isLoopbackHost(host string) bool {
	normalized := strings.TrimSpace(strings.ToLower(host))
	if normalized == "localhost" {
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}
