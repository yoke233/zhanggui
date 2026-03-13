package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type executionAuditTimelineResponse struct {
	ExecutionID int64                       `json:"execution_id"`
	Items       []executionAuditTimelineRow `json:"items"`
}

type executionAuditTimelineRow struct {
	Source          string              `json:"source"`
	Kind            string              `json:"kind"`
	Timestamp       time.Time           `json:"timestamp"`
	WorkItemID      int64               `json:"work_item_id,omitempty"`
	ActionID        int64               `json:"action_id,omitempty"`
	RunID           int64               `json:"run_id,omitempty"`
	Status          string              `json:"status,omitempty"`
	Summary         string              `json:"summary,omitempty"`
	EventID         int64               `json:"event_id,omitempty"`
	ProbeID         int64               `json:"probe_id,omitempty"`
	SignalID        int64               `json:"signal_id,omitempty"`
	ToolCallAuditID int64               `json:"tool_call_audit_id,omitempty"`
	Event           *core.Event         `json:"event,omitempty"`
	Probe           *core.RunProbe      `json:"probe,omitempty"`
	Signal          *core.ActionSignal  `json:"signal,omitempty"`
	ToolCall        *core.ToolCallAudit `json:"tool_call,omitempty"`
}

func (h *Handler) getExecutionAuditTimeline(w http.ResponseWriter, r *http.Request) {
	execID, ok := urlParamInt64(r, "execID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution ID", "BAD_ID")
		return
	}

	filter := core.EventFilter{
		RunID:  &execID,
		Limit:  queryInt(r, "limit", 500),
		Offset: queryInt(r, "offset", 0),
	}
	events, err := h.store.ListEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "EVENT_LIST_ERROR")
		return
	}
	probes, err := h.store.ListRunProbesByRun(r.Context(), execID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "PROBE_LIST_ERROR")
		return
	}
	toolCalls, err := h.store.ListToolCallAuditsByRun(r.Context(), execID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "TOOL_CALL_AUDIT_LIST_ERROR")
		return
	}
	runRec, err := h.store.GetRun(r.Context(), execID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "execution not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "RUN_GET_ERROR")
		return
	}
	signals, err := h.store.ListActionSignals(r.Context(), runRec.ActionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "SIGNAL_LIST_ERROR")
		return
	}

	items := make([]executionAuditTimelineRow, 0, len(events)+len(probes)+len(toolCalls)+len(signals))
	for _, event := range events {
		if event == nil {
			continue
		}
		items = append(items, executionAuditTimelineRow{
			Source:     "event",
			Kind:       string(event.Type),
			Timestamp:  event.Timestamp,
			WorkItemID: event.WorkItemID,
			ActionID:   event.ActionID,
			RunID:      event.RunID,
			Status:     timelineEventStatus(event),
			Summary:    timelineEventSummary(event),
			EventID:    event.ID,
			Event:      event,
		})
	}
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		items = append(items, executionAuditTimelineRow{
			Source:     "probe",
			Kind:       "execution.probe",
			Timestamp:  timelineProbeTimestamp(probe),
			WorkItemID: probe.WorkItemID,
			ActionID:   probe.ActionID,
			RunID:      probe.RunID,
			Status:     string(probe.Status),
			Summary:    timelineProbeSummary(probe),
			ProbeID:    probe.ID,
			Probe:      probe,
		})
	}
	for _, auditItem := range toolCalls {
		if auditItem == nil {
			continue
		}
		items = append(items, executionAuditTimelineRow{
			Source:          "tool_call",
			Kind:            "tool.call",
			Timestamp:       timelineToolCallTimestamp(auditItem),
			WorkItemID:      auditItem.WorkItemID,
			ActionID:        auditItem.ActionID,
			RunID:           auditItem.RunID,
			Status:          auditItem.Status,
			Summary:         timelineToolCallSummary(auditItem),
			ToolCallAuditID: auditItem.ID,
			ToolCall:        auditItem,
		})
	}
	for _, signal := range signals {
		if signal == nil || signal.RunID != execID {
			continue
		}
		items = append(items, executionAuditTimelineRow{
			Source:     "signal",
			Kind:       "action.signal",
			Timestamp:  signal.CreatedAt,
			WorkItemID: signal.WorkItemID,
			ActionID:   signal.ActionID,
			RunID:      signal.RunID,
			Status:     string(signal.Type),
			Summary:    timelineSignalSummary(signal),
			SignalID:   signal.ID,
			Signal:     signal,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Timestamp.Equal(items[j].Timestamp) {
			if items[i].Source == items[j].Source {
				return items[i].Kind < items[j].Kind
			}
			return items[i].Source < items[j].Source
		}
		return items[i].Timestamp.Before(items[j].Timestamp)
	})

	writeJSON(w, http.StatusOK, executionAuditTimelineResponse{
		ExecutionID: execID,
		Items:       items,
	})
}

func timelineEventStatus(event *core.Event) string {
	if event == nil || event.Data == nil {
		return ""
	}
	if status, ok := event.Data["status"].(string); ok {
		return strings.TrimSpace(status)
	}
	if status, ok := event.Data["state"].(string); ok {
		return strings.TrimSpace(status)
	}
	return ""
}

func timelineEventSummary(event *core.Event) string {
	if event == nil {
		return ""
	}
	if event.Type == core.EventExecutionAudit {
		kind, _ := event.Data["kind"].(string)
		status, _ := event.Data["status"].(string)
		if strings.TrimSpace(kind) != "" && strings.TrimSpace(status) != "" {
			return strings.TrimSpace(kind) + " " + strings.TrimSpace(status)
		}
		if strings.TrimSpace(kind) != "" {
			return strings.TrimSpace(kind)
		}
	}
	if event.Data != nil {
		if content, ok := event.Data["content"].(string); ok && strings.TrimSpace(content) != "" {
			return strings.TrimSpace(content)
		}
		if errText, ok := event.Data["error"].(string); ok && strings.TrimSpace(errText) != "" {
			return strings.TrimSpace(errText)
		}
	}
	return string(event.Type)
}

func timelineProbeTimestamp(probe *core.RunProbe) time.Time {
	if probe == nil {
		return time.Time{}
	}
	if probe.AnsweredAt != nil {
		return *probe.AnsweredAt
	}
	if probe.SentAt != nil {
		return *probe.SentAt
	}
	return probe.CreatedAt
}

func timelineProbeSummary(probe *core.RunProbe) string {
	if probe == nil {
		return ""
	}
	if strings.TrimSpace(probe.ReplyText) != "" {
		return strings.TrimSpace(probe.ReplyText)
	}
	if strings.TrimSpace(probe.Error) != "" {
		return strings.TrimSpace(probe.Error)
	}
	if strings.TrimSpace(probe.Question) != "" {
		return strings.TrimSpace(probe.Question)
	}
	return string(probe.Verdict)
}

func timelineToolCallTimestamp(item *core.ToolCallAudit) time.Time {
	if item == nil {
		return time.Time{}
	}
	if item.FinishedAt != nil {
		return *item.FinishedAt
	}
	if item.StartedAt != nil {
		return *item.StartedAt
	}
	return item.CreatedAt
}

func timelineToolCallSummary(item *core.ToolCallAudit) string {
	if item == nil {
		return ""
	}
	if strings.TrimSpace(item.ToolName) != "" {
		return strings.TrimSpace(item.ToolName)
	}
	return strings.TrimSpace(item.ToolCallID)
}

func timelineSignalSummary(signal *core.ActionSignal) string {
	if signal == nil {
		return ""
	}
	if strings.TrimSpace(signal.Summary) != "" {
		return strings.TrimSpace(signal.Summary)
	}
	if strings.TrimSpace(signal.Content) != "" {
		return strings.TrimSpace(signal.Content)
	}
	if signal.Payload != nil {
		if reason, ok := signal.Payload["reason"].(string); ok && strings.TrimSpace(reason) != "" {
			return strings.TrimSpace(reason)
		}
		if summary, ok := signal.Payload["summary"].(string); ok && strings.TrimSpace(summary) != "" {
			return strings.TrimSpace(summary)
		}
	}
	return string(signal.Type)
}
