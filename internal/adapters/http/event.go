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

func (h *Handler) listWorkItemEvents(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	filter := buildEventFilter(r)
	filter.WorkItemID = &issueID

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
			filter.WorkItemID = &id
		}
	}
	if s := r.URL.Query().Get("step_id"); s != "" {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			filter.ActionID = &id
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

// wsConnState holds per-WebSocket-connection state for dynamic subscriptions.
type wsConnState struct {
	mu        sync.Mutex
	threadIDs map[int64]bool
}

func (s *wsConnState) subscribeThread(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.threadIDs == nil {
		s.threadIDs = make(map[int64]bool)
	}
	s.threadIDs[id] = true
}

func (s *wsConnState) unsubscribeThread(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.threadIDs, id)
}

func (s *wsConnState) isThreadSubscribed(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.threadIDs[id]
}

// isThreadEvent returns true if the event type is a thread-scoped event.
func isThreadEvent(t core.EventType) bool {
	switch t {
	case core.EventThreadMessage,
		core.EventThreadAgentJoined,
		core.EventThreadAgentLeft,
		core.EventThreadAgentOutput,
		core.EventThreadAgentBooted,
		core.EventThreadAgentFailed:
		return true
	}
	return false
}

// threadIDFromEventData extracts thread_id from event data, handling both int64 and float64.
func threadIDFromEventData(data map[string]any) (int64, bool) {
	v, ok := data["thread_id"]
	if !ok {
		return 0, false
	}
	switch id := v.(type) {
	case int64:
		return id, true
	case float64:
		return int64(id), true
	}
	return 0, false
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

	connState := &wsConnState{}

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
			h.handleWSClientMessage(msg, writeJSON, connState)
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
			if issueFilter != 0 && ev.WorkItemID != issueFilter {
				continue
			}
			if sessionFilter != "" {
				eventSessionID, _ := ev.Data["session_id"].(string)
				if strings.TrimSpace(eventSessionID) != sessionFilter {
					continue
				}
			}
			// Thread events are only forwarded to connections subscribed to that thread.
			if isThreadEvent(ev.Type) {
				tid, ok := threadIDFromEventData(ev.Data)
				if !ok || !connState.isThreadSubscribed(tid) {
					continue
				}
			}

			if err := writeJSON(ev); err != nil {
				return
			}
		}
	}
}

func (h *Handler) handleWSClientMessage(msg wsMessage, writeJSON func(v any) error, state *wsConnState) {
	msgType := strings.TrimSpace(msg.Type)
	switch msgType {
	case "chat.send":
		h.handleWSChatSend(msg, writeJSON)
	case "chat.set_config":
		h.handleWSChatSetConfig(msg, writeJSON)
	case "chat.set_mode":
		h.handleWSChatSetMode(msg, writeJSON)
	case "chat.permission_response":
		h.handleWSChatPermissionResponse(msg, writeJSON)
	case "thread.send":
		h.handleWSThreadSend(msg, writeJSON)
	case "subscribe_thread":
		h.handleWSSubscribeThread(msg, writeJSON, state)
	case "unsubscribe_thread":
		h.handleWSUnsubscribeThread(msg, writeJSON, state)
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

func (h *Handler) handleWSChatSetConfig(msg wsMessage, writeJSON func(v any) error) {
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

	var req wsSetConfigRequest
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			_ = writeJSON(wsOutboundMessage{
				Type: "chat.error",
				Data: wsErrorPayload{
					Code:  "BAD_REQUEST",
					Error: "invalid chat.set_config payload",
				},
			})
			return
		}
	}

	reqID := strings.TrimSpace(req.RequestID)
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{
				Code:      "BAD_REQUEST",
				RequestID: reqID,
				Error:     "session_id is required",
			},
		})
		return
	}

	configOptions, err := h.lead.SetConfigOption(context.Background(), sessionID, req.ConfigID, req.Value)
	if err != nil {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{
				Code:      "SET_CONFIG_FAILED",
				RequestID: reqID,
				SessionID: sessionID,
				Error:     err.Error(),
			},
		})
		return
	}

	_ = writeJSON(wsOutboundMessage{
		Type: "chat.config_updated",
		Data: wsConfigUpdatedPayload{
			RequestID:     reqID,
			SessionID:     sessionID,
			ConfigOptions: configOptions,
		},
	})
}

func (h *Handler) handleWSChatSetMode(msg wsMessage, writeJSON func(v any) error) {
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

	var req wsSetModeRequest
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			_ = writeJSON(wsOutboundMessage{
				Type: "chat.error",
				Data: wsErrorPayload{
					Code:  "BAD_REQUEST",
					Error: "invalid chat.set_mode payload",
				},
			})
			return
		}
	}

	reqID := strings.TrimSpace(req.RequestID)
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{
				Code:      "BAD_REQUEST",
				RequestID: reqID,
				Error:     "session_id is required",
			},
		})
		return
	}

	modes, err := h.lead.SetSessionMode(context.Background(), sessionID, req.ModeID)
	if err != nil {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{
				Code:      "SET_MODE_FAILED",
				RequestID: reqID,
				SessionID: sessionID,
				Error:     err.Error(),
			},
		})
		return
	}

	_ = writeJSON(wsOutboundMessage{
		Type: "chat.mode_updated",
		Data: wsModeUpdatedPayload{
			RequestID: reqID,
			SessionID: sessionID,
			Modes:     modes,
		},
	})
}

func (h *Handler) handleWSChatPermissionResponse(msg wsMessage, writeJSON func(v any) error) {
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

	var req wsPermissionResponseRequest
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			_ = writeJSON(wsOutboundMessage{
				Type: "chat.error",
				Data: wsErrorPayload{
					Code:  "BAD_REQUEST",
					Error: "invalid chat.permission_response payload",
				},
			})
			return
		}
	}

	permID := strings.TrimSpace(req.PermissionID)
	if permID == "" {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{
				Code:  "BAD_REQUEST",
				Error: "permission_id is required",
			},
		})
		return
	}

	if err := h.lead.ResolvePermission(permID, req.OptionID, req.Cancel); err != nil {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{
				Code:  "PERMISSION_RESOLVE_FAILED",
				Error: err.Error(),
			},
		})
		return
	}

	_ = writeJSON(wsOutboundMessage{
		Type: "chat.permission_resolved",
		Data: map[string]string{
			"permission_id": permID,
			"status":        "resolved",
		},
	})
}

type wsPermissionResponseRequest struct {
	PermissionID string `json:"permission_id"`
	OptionID     string `json:"option_id,omitempty"`
	Cancel       bool   `json:"cancel,omitempty"`
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
	RequestID   string               `json:"request_id,omitempty"`
	SessionID   string               `json:"session_id,omitempty"`
	Message     string               `json:"message"`
	Attachments []chatapp.Attachment `json:"attachments,omitempty"`
	WorkDir     string               `json:"work_dir,omitempty"`
	ProjectID   int64                `json:"project_id,omitempty"`
	ProjectName string               `json:"project_name,omitempty"`
	ProfileID   string               `json:"profile_id,omitempty"`
	DriverID    string               `json:"driver_id,omitempty"`
}

type wsChatAckPayload struct {
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id"`
	WSPath    string `json:"ws_path,omitempty"`
	Status    string `json:"status"`
}

type wsSetConfigRequest struct {
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id"`
	ConfigID  string `json:"config_id"`
	Value     string `json:"value"`
}

type wsConfigUpdatedPayload struct {
	RequestID     string                 `json:"request_id,omitempty"`
	SessionID     string                 `json:"session_id"`
	ConfigOptions []chatapp.ConfigOption `json:"config_options"`
}

type wsSetModeRequest struct {
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id"`
	ModeID    string `json:"mode_id"`
}

type wsModeUpdatedPayload struct {
	RequestID string                    `json:"request_id,omitempty"`
	SessionID string                    `json:"session_id"`
	Modes     *chatapp.SessionModeState `json:"modes,omitempty"`
}

type wsErrorPayload struct {
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Code      string `json:"code,omitempty"`
	Error     string `json:"error"`
}

// ---------------------------------------------------------------------------
// Thread WebSocket message types
// ---------------------------------------------------------------------------

type wsThreadSendRequest struct {
	RequestID string `json:"request_id,omitempty"`
	ThreadID  int64  `json:"thread_id"`
	Message   string `json:"message"`
	SenderID  string `json:"sender_id,omitempty"`
}

type wsThreadAckPayload struct {
	RequestID string `json:"request_id,omitempty"`
	ThreadID  int64  `json:"thread_id"`
	Status    string `json:"status"`
}

type wsThreadSubscribeRequest struct {
	ThreadID int64 `json:"thread_id"`
}

type wsThreadSubscriptionPayload struct {
	ThreadID int64  `json:"thread_id"`
	Status   string `json:"status"`
}

func (h *Handler) handleWSThreadSend(msg wsMessage, writeJSON func(v any) error) {
	var req wsThreadSendRequest
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			_ = writeJSON(wsOutboundMessage{
				Type: "thread.error",
				Data: wsErrorPayload{
					Code:  "BAD_REQUEST",
					Error: "invalid thread.send payload",
				},
			})
			return
		}
	}

	reqID := strings.TrimSpace(req.RequestID)
	if req.ThreadID <= 0 {
		_ = writeJSON(wsOutboundMessage{
			Type: "thread.error",
			Data: wsErrorPayload{
				Code:      "BAD_REQUEST",
				RequestID: reqID,
				Error:     "thread_id is required",
			},
		})
		return
	}

	// Validate thread exists.
	_, err := h.store.GetThread(context.Background(), req.ThreadID)
	if err != nil {
		if err == core.ErrNotFound {
			_ = writeJSON(wsOutboundMessage{
				Type: "thread.error",
				Data: wsErrorPayload{
					Code:      "THREAD_NOT_FOUND",
					RequestID: reqID,
					Error:     "thread not found",
				},
			})
			return
		}
		_ = writeJSON(wsOutboundMessage{
			Type: "thread.error",
			Data: wsErrorPayload{
				Code:      "THREAD_SEND_FAILED",
				RequestID: reqID,
				Error:     err.Error(),
			},
		})
		return
	}

	// Save human message to store.
	humanMsg := &core.ThreadMessage{
		ThreadID: req.ThreadID,
		SenderID: strings.TrimSpace(req.SenderID),
		Role:     "human",
		Content:  req.Message,
	}
	if _, err := h.store.CreateThreadMessage(context.Background(), humanMsg); err != nil {
		_ = writeJSON(wsOutboundMessage{
			Type: "thread.error",
			Data: wsErrorPayload{
				Code:      "THREAD_SEND_FAILED",
				RequestID: reqID,
				Error:     err.Error(),
			},
		})
		return
	}

	// Publish thread message event for real-time broadcast.
	h.bus.Publish(context.Background(), core.Event{
		Type: core.EventThreadMessage,
		Data: map[string]any{
			"thread_id": req.ThreadID,
			"message":   req.Message,
			"sender_id": strings.TrimSpace(req.SenderID),
			"role":      "human",
		},
		Timestamp: time.Now().UTC(),
	})

	_ = writeJSON(wsOutboundMessage{
		Type: "thread.ack",
		Data: wsThreadAckPayload{
			RequestID: reqID,
			ThreadID:  req.ThreadID,
			Status:    "accepted",
		},
	})

	// Route message to active agents (async, non-blocking).
	if h.threadPool != nil {
		profileIDs := h.threadPool.ActiveAgentProfileIDs(req.ThreadID)
		for _, pid := range profileIDs {
			go func(profileID string) {
				if err := h.threadPool.SendMessage(context.Background(), req.ThreadID, profileID, req.Message); err != nil {
					h.bus.Publish(context.Background(), core.Event{
						Type: core.EventThreadAgentFailed,
						Data: map[string]any{
							"thread_id":  req.ThreadID,
							"profile_id": profileID,
							"error":      err.Error(),
						},
						Timestamp: time.Now().UTC(),
					})
				}
			}(pid)
		}
	}
}

func (h *Handler) handleWSSubscribeThread(msg wsMessage, writeJSON func(v any) error, state *wsConnState) {
	var req wsThreadSubscribeRequest
	if len(msg.Data) > 0 {
		json.Unmarshal(msg.Data, &req)
	}

	if req.ThreadID <= 0 {
		_ = writeJSON(wsOutboundMessage{
			Type: "thread.error",
			Data: wsErrorPayload{
				Code:  "BAD_REQUEST",
				Error: "thread_id is required",
			},
		})
		return
	}

	state.subscribeThread(req.ThreadID)

	_ = writeJSON(wsOutboundMessage{
		Type: "thread.subscribed",
		Data: wsThreadSubscriptionPayload{
			ThreadID: req.ThreadID,
			Status:   "subscribed",
		},
	})
}

func (h *Handler) handleWSUnsubscribeThread(msg wsMessage, writeJSON func(v any) error, state *wsConnState) {
	var req wsThreadSubscribeRequest
	if len(msg.Data) > 0 {
		json.Unmarshal(msg.Data, &req)
	}

	if req.ThreadID <= 0 {
		_ = writeJSON(wsOutboundMessage{
			Type: "thread.error",
			Data: wsErrorPayload{
				Code:  "BAD_REQUEST",
				Error: "thread_id is required",
			},
		})
		return
	}

	state.unsubscribeThread(req.ThreadID)

	_ = writeJSON(wsOutboundMessage{
		Type: "thread.unsubscribed",
		Data: wsThreadSubscriptionPayload{
			ThreadID: req.ThreadID,
			Status:   "unsubscribed",
		},
	})
}
