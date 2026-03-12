package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	chatapp "github.com/yoke233/ai-workflow/internal/application/chat"
	"github.com/yoke233/ai-workflow/internal/core"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // allow all origins for dev
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	filter := buildEventFilter(r)

	events, err := h.store.ListEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if events == nil {
		events = []*core.Event{}
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) listIssueEvents(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	filter := buildEventFilter(r)
	filter.IssueID = &issueID

	events, err := h.store.ListEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if events == nil {
		events = []*core.Event{}
	}
	writeJSON(w, http.StatusOK, events)
}

func buildEventFilter(r *http.Request) core.EventFilter {
	filter := core.EventFilter{
		Limit:  queryInt(r, "limit", 100),
		Offset: queryInt(r, "offset", 0),
	}

	if s := r.URL.Query().Get("issue_id"); s != "" {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			filter.IssueID = &id
		}
	}
	if s := r.URL.Query().Get("step_id"); s != "" {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			filter.StepID = &id
		}
	}
	if s := r.URL.Query().Get("types"); s != "" {
		for _, t := range strings.Split(s, ",") {
			if t = strings.TrimSpace(t); t != "" {
				filter.Types = append(filter.Types, core.EventType(t))
			}
		}
	}
	filter.SessionID = strings.TrimSpace(r.URL.Query().Get("session_id"))
	return filter
}

// wsEvents upgrades to WebSocket and streams real-time events from the EventBus.
// Query params:
//   - issue_id: optional, filter events to a specific issue
//   - types: optional, comma-separated event types to subscribe to
func (h *Handler) wsEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade writes its own error
	}
	defer conn.Close()

	var writeMu sync.Mutex
	writeJSON := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return conn.WriteJSON(v)
	}

	// Parse subscribe options from query params.
	var types []core.EventType
	if s := r.URL.Query().Get("types"); s != "" {
		for _, t := range strings.Split(s, ",") {
			if t = strings.TrimSpace(t); t != "" {
				types = append(types, core.EventType(t))
			}
		}
	}

	var issueFilter int64
	if s := r.URL.Query().Get("issue_id"); s != "" {
		issueFilter, _ = strconv.ParseInt(s, 10, 64)
	}
	sessionFilter := strings.TrimSpace(r.URL.Query().Get("session_id"))

	sub := h.bus.Subscribe(core.SubscribeOpts{
		Types:      types,
		BufferSize: 64,
	})
	defer sub.Cancel()

	// Read pump: detect client disconnect.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var msg wsMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			h.handleWSClientMessage(msg, writeJSON)
		}
	}()

	for {
		select {
		case <-done:
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			// Apply issue filter if specified.
			if issueFilter != 0 && ev.IssueID != issueFilter {
				continue
			}
			if sessionFilter != "" {
				eventSessionID, _ := ev.Data["session_id"].(string)
				if strings.TrimSpace(eventSessionID) != sessionFilter {
					continue
				}
			}

			if err := writeJSON(ev); err != nil {
				return
			}
		}
	}
}

func (h *Handler) handleWSClientMessage(msg wsMessage, writeJSON func(v any) error) {
	msgType := strings.TrimSpace(msg.Type)
	switch msgType {
	case "chat.send":
		h.handleWSChatSend(msg, writeJSON)
	default:
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{
				Code:  "UNSUPPORTED_MESSAGE_TYPE",
				Error: "unsupported websocket message type",
			},
		})
	}
}

func (h *Handler) handleWSChatSend(msg wsMessage, writeJSON func(v any) error) {
	if h.lead == nil {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{
				Code:  "CHAT_DISABLED",
				Error: "lead chat service is not configured",
			},
		})
		return
	}

	var req wsChatSendRequest
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			_ = writeJSON(wsOutboundMessage{
				Type: "chat.error",
				Data: wsErrorPayload{
					Code:      "BAD_REQUEST",
					RequestID: strings.TrimSpace(req.RequestID),
					Error:     "invalid chat.send payload",
				},
			})
			return
		}
	}

	accepted, err := h.lead.StartChat(context.Background(), chatapp.Request{
		SessionID:   strings.TrimSpace(req.SessionID),
		Message:     req.Message,
		Attachments: req.Attachments,
		WorkDir:     req.WorkDir,
		ProjectID:   req.ProjectID,
		ProjectName: strings.TrimSpace(req.ProjectName),
		ProfileID:   strings.TrimSpace(req.ProfileID),
		DriverID:    strings.TrimSpace(req.DriverID),
	})
	if err != nil {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{
				Code:      "CHAT_FAILED",
				RequestID: strings.TrimSpace(req.RequestID),
				SessionID: strings.TrimSpace(req.SessionID),
				Error:     err.Error(),
			},
		})
		return
	}

	_ = writeJSON(wsOutboundMessage{
		Type: "chat.ack",
		Data: wsChatAckPayload{
			RequestID: strings.TrimSpace(req.RequestID),
			SessionID: accepted.SessionID,
			WSPath:    accepted.WSPath,
			Status:    "accepted",
		},
	})
}

// wsMessage is the WebSocket message envelope (for potential future use).
type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type wsOutboundMessage struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type wsChatSendRequest struct {
	RequestID   string                `json:"request_id,omitempty"`
	SessionID   string                `json:"session_id,omitempty"`
	Message     string                `json:"message"`
	Attachments []chatapp.Attachment  `json:"attachments,omitempty"`
	WorkDir     string                `json:"work_dir,omitempty"`
	ProjectID   int64                 `json:"project_id,omitempty"`
	ProjectName string                `json:"project_name,omitempty"`
	ProfileID   string                `json:"profile_id,omitempty"`
	DriverID    string                `json:"driver_id,omitempty"`
}

type wsChatAckPayload struct {
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id"`
	WSPath    string `json:"ws_path,omitempty"`
	Status    string `json:"status"`
}

type wsErrorPayload struct {
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Code      string `json:"code,omitempty"`
	Error     string `json:"error"`
}
