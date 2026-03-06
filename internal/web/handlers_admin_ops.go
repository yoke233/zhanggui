package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
	ghwebhook "github.com/yoke233/ai-workflow/internal/github"
	"github.com/yoke233/ai-workflow/internal/observability"
)

type adminOpsHandlers struct {
	store       core.Store
	replayer    WebhookDeliveryReplayer
	restartFunc func() // called to trigger graceful server restart; nil = not supported
}

type adminIssueOperationRequest struct {
	IssueID string `json:"issue_id"`
	TaskID  string `json:"task_id,omitempty"`
	TraceID string `json:"trace_id"`
	Reason  string `json:"reason"`
}

type adminReplayRequest struct {
	DeliveryID string `json:"delivery_id"`
	RunID      string `json:"run_id"`
	TraceID    string `json:"trace_id"`
}

type adminAuditLogItem struct {
	ID        int64  `json:"id"`
	ProjectID string `json:"project_id,omitempty"`
	IssueID   string `json:"issue_id,omitempty"`
	RunID     string `json:"run_id"`
	Stage     string `json:"stage,omitempty"`
	Action    string `json:"action"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	UserID    string `json:"user_id"`
	CreatedAt string `json:"created_at"`
}

type adminAuditLogResponse struct {
	Items  []adminAuditLogItem `json:"items"`
	Total  int                 `json:"total"`
	Offset int                 `json:"offset"`
}

type auditRunMeta struct {
	ProjectID string
	IssueID   string
}

func registerAdminOpsRoutes(r chi.Router, store core.Store, replayer WebhookDeliveryReplayer, restartFunc func()) {
	h := &adminOpsHandlers{
		store:       store,
		replayer:    replayer,
		restartFunc: restartFunc,
	}
	r.Post("/admin/ops/force-ready", h.handleForceReady)
	r.Post("/admin/ops/force-unblock", h.handleForceUnblock)
	r.Post("/admin/ops/replay-delivery", h.handleReplayDelivery)
	r.Post("/admin/ops/restart", h.handleRestart)
	r.Get("/admin/audit-log", h.handleListAuditLog)
}

func (h *adminOpsHandlers) handleForceReady(w http.ResponseWriter, r *http.Request) {
	h.handleTaskStateMutation(w, r, "force_ready", core.IssueStatusReady)
}

func (h *adminOpsHandlers) handleForceUnblock(w http.ResponseWriter, r *http.Request) {
	h.handleTaskStateMutation(w, r, "force_unblock", core.IssueStatusReady)
}

func (h *adminOpsHandlers) handleTaskStateMutation(
	w http.ResponseWriter,
	r *http.Request,
	action string,
	targetStatus core.IssueStatus,
) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	var req adminIssueOperationRequest
	if err := decodeJSONBodyStrict(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}
	issueID := strings.TrimSpace(req.IssueID)
	if issueID == "" {
		issueID = strings.TrimSpace(req.TaskID)
	}
	if issueID == "" {
		writeAPIError(w, http.StatusBadRequest, "issue_id is required", "ISSUE_ID_REQUIRED")
		return
	}

	issue, err := h.store.GetIssue(issueID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, "issue not found", "ISSUE_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load issue", "GET_ISSUE_FAILED")
		return
	}

	// Admin force operations intentionally bypass the normal state machine
	// to provide an escape hatch for stuck issues.
	issue.Status = targetStatus
	if err := h.store.SaveIssue(issue); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to update issue", "SAVE_ISSUE_FAILED")
		return
	}

	traceID := resolveAdminTraceID(req.TraceID, r)
	auditMessage := "trace_id=" + traceID
	if reason := strings.TrimSpace(req.Reason); reason != "" {
		auditMessage += " reason=" + reason
	}
	if err := h.store.RecordAction(core.HumanAction{
		RunID:   issue.RunID,
		Action:  action,
		Message: auditMessage,
		Source:  "admin",
		UserID:  "admin",
	}); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to record admin audit", "AUDIT_RECORD_FAILED")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"issue_id": issue.ID,
		"trace_id": traceID,
	})
}

func (h *adminOpsHandlers) handleRestart(w http.ResponseWriter, r *http.Request) {
	if h.restartFunc == nil {
		writeAPIError(w, http.StatusNotImplemented, "restart not supported", "RESTART_NOT_SUPPORTED")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":  "restarting",
		"message": "server restart initiated",
	})
	// Trigger restart after response is flushed.
	go h.restartFunc()
}

func (h *adminOpsHandlers) handleReplayDelivery(w http.ResponseWriter, r *http.Request) {
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
	if RunID := strings.TrimSpace(req.RunID); RunID != "" {
		if err := h.store.RecordAction(core.HumanAction{
			RunID:   RunID,
			Action:  "replay_delivery",
			Message: "trace_id=" + traceID + " delivery_id=" + deliveryID,
			Source:  "admin",
			UserID:  "admin",
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

func (h *adminOpsHandlers) handleListAuditLog(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	limit, offset, err := parseAdminAuditPaginationParams(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_QUERY_PARAM")
		return
	}

	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	actionFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("action")))
	userFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("user")))

	since, err := parseAdminAuditTimeBoundary(r, "since")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_TIME_BOUNDARY")
		return
	}
	until, err := parseAdminAuditTimeBoundary(r, "until")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_TIME_BOUNDARY")
		return
	}

	items, err := h.collectAdminAuditItems(projectID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, "project not found", "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load audit log", "GET_AUDIT_LOG_FAILED")
		return
	}

	filtered := make([]adminAuditLogItem, 0, len(items))
	for i := range items {
		if !matchesAdminAuditFilters(items[i], actionFilter, userFilter, since, until) {
			continue
		}
		filtered = append(filtered, items[i])
	}

	total := len(filtered)
	if offset >= total {
		writeJSON(w, http.StatusOK, adminAuditLogResponse{
			Items:  []adminAuditLogItem{},
			Total:  total,
			Offset: offset,
		})
		return
	}
	end := offset + limit
	if end > total {
		end = total
	}

	writeJSON(w, http.StatusOK, adminAuditLogResponse{
		Items:  filtered[offset:end],
		Total:  total,
		Offset: offset,
	})
}

func (h *adminOpsHandlers) collectAdminAuditItems(projectID string) ([]adminAuditLogItem, error) {
	projects, err := h.resolveAuditProjects(projectID)
	if err != nil {
		return nil, err
	}

	metas := make(map[string]auditRunMeta)
	Runset := make(map[string]struct{})
	for i := range projects {
		issues, err := listAllIssuesForProject(h.store, projects[i].ID)
		if err != nil {
			return nil, err
		}
		for j := range issues {
			RunID := strings.TrimSpace(issues[j].RunID)
			if RunID == "" {
				continue
			}
			if _, exists := metas[RunID]; !exists {
				metas[RunID] = auditRunMeta{
					ProjectID: projects[i].ID,
					IssueID:   issues[j].ID,
				}
			}
			Runset[RunID] = struct{}{}
		}
	}

	RunIDs := make([]string, 0, len(Runset))
	for RunID := range Runset {
		RunIDs = append(RunIDs, RunID)
	}
	sort.Strings(RunIDs)

	items := make([]adminAuditLogItem, 0)
	for i := range RunIDs {
		actions, err := h.store.GetActions(RunIDs[i])
		if err != nil {
			return nil, err
		}
		meta := metas[RunIDs[i]]
		for j := range actions {
			items = append(items, adminAuditLogItem{
				ID:        actions[j].ID,
				ProjectID: meta.ProjectID,
				IssueID:   meta.IssueID,
				RunID:     actions[j].RunID,
				Stage:     actions[j].Stage,
				Action:    actions[j].Action,
				Message:   actions[j].Message,
				Source:    actions[j].Source,
				UserID:    actions[j].UserID,
				CreatedAt: actions[j].CreatedAt,
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		leftTime, leftOK := parseAdminAuditTimestamp(items[i].CreatedAt)
		rightTime, rightOK := parseAdminAuditTimestamp(items[j].CreatedAt)
		switch {
		case leftOK && rightOK && !leftTime.Equal(rightTime):
			return leftTime.After(rightTime)
		case leftOK != rightOK:
			return leftOK
		case items[i].ID != items[j].ID:
			return items[i].ID > items[j].ID
		default:
			return items[i].RunID < items[j].RunID
		}
	})

	return items, nil
}

func (h *adminOpsHandlers) resolveAuditProjects(projectID string) ([]core.Project, error) {
	if strings.TrimSpace(projectID) != "" {
		project, err := h.store.GetProject(strings.TrimSpace(projectID))
		if err != nil {
			return nil, err
		}
		return []core.Project{*project}, nil
	}

	projects, err := h.store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return nil, err
	}
	if projects == nil {
		return []core.Project{}, nil
	}
	return projects, nil
}

func listAllIssuesForProject(store core.Store, projectID string) ([]core.Issue, error) {
	const pageSize = 200
	offset := 0
	all := make([]core.Issue, 0)
	for {
		items, total, err := store.ListIssues(projectID, core.IssueFilter{
			Limit:  pageSize,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
		offset += len(items)
		if offset >= total {
			break
		}
	}
	return all, nil
}

func parseAdminAuditPaginationParams(r *http.Request) (int, int, error) {
	limit := 100
	offset := 0

	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			return 0, 0, fmt.Errorf("limit must be a positive integer")
		}
		limit = parsed
	}
	if rawOffset := strings.TrimSpace(r.URL.Query().Get("offset")); rawOffset != "" {
		parsed, err := strconv.Atoi(rawOffset)
		if err != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = parsed
	}

	return limit, offset, nil
}

func parseAdminAuditTimeBoundary(r *http.Request, key string) (*time.Time, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be RFC3339 timestamp", key)
	}
	return &parsed, nil
}

func matchesAdminAuditFilters(
	item adminAuditLogItem,
	actionFilter string,
	userFilter string,
	since *time.Time,
	until *time.Time,
) bool {
	if actionFilter != "" && strings.ToLower(strings.TrimSpace(item.Action)) != actionFilter {
		return false
	}
	if userFilter != "" && strings.ToLower(strings.TrimSpace(item.UserID)) != userFilter {
		return false
	}
	if since == nil && until == nil {
		return true
	}
	createdAt, ok := parseAdminAuditTimestamp(item.CreatedAt)
	if !ok {
		return false
	}
	if since != nil && createdAt.Before(*since) {
		return false
	}
	if until != nil && createdAt.After(*until) {
		return false
	}
	return true
}

func parseAdminAuditTimestamp(raw string) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for i := range layouts {
		if parsed, err := time.Parse(layouts[i], trimmed); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
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
