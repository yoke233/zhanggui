package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yoke233/ai-workflow/internal/core"
)

const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = 30 * time.Second
	wsMaxMessage = 1 << 20
)

type WSMessage struct {
	Type      string         `json:"type"`
	RunID     string         `json:"run_id,omitempty"`
	ProjectID string         `json:"project_id,omitempty"`
	IssueID   string         `json:"issue_id,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

type wsClientMessage struct {
	Type    string `json:"type"`
	RunID   string `json:"run_id,omitempty"`
	IssueID string `json:"issue_id,omitempty"`
}

type Hub struct {
	upgrader websocket.Upgrader

	mu      sync.RWMutex
	clients map[*wsClient]struct{}
}

type wsClient struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte

	subMu     sync.RWMutex
	Runsubs   map[string]struct{}
	issueSubs map[string]struct{}
}

func NewHub() *Hub {
	return &Hub{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
		clients: make(map[*wsClient]struct{}),
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &wsClient{
		hub:       h,
		conn:      conn,
		send:      make(chan []byte, 64),
		Runsubs:   make(map[string]struct{}),
		issueSubs: make(map[string]struct{}),
	}

	h.register(client)
	go client.writePump()
	client.readPump()
}

func (h *Hub) register(client *wsClient) {
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(client *wsClient) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()
}

func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) Broadcast(msg WSMessage) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if !client.shouldReceive(msg) {
			continue
		}
		select {
		case client.send <- payload:
		default:
		}
	}
}

func (h *Hub) BroadcastCoreEvent(evt core.Event) {
	data := map[string]any{
		"timestamp": evt.Timestamp.UTC().Format(time.RFC3339Nano),
	}
	if evt.Stage != "" {
		data["stage"] = evt.Stage
	}
	if evt.Agent != "" {
		data["agent"] = evt.Agent
	}
	if len(evt.Data) > 0 {
		for k, v := range evt.Data {
			switch k {
			case "acp_content_json":
				continue
			case "acp_update_json":
				trimmed := strings.TrimSpace(v)
				if trimmed == "" {
					continue
				}
				var acpPayload any
				if err := json.Unmarshal([]byte(trimmed), &acpPayload); err == nil {
					data["acp"] = acpPayload
				}
			default:
				data[k] = v
			}
		}
	}
	if evt.Error != "" {
		data["error"] = evt.Error
	}
	issueID := strings.TrimSpace(evt.IssueID)
	if issueID == "" && evt.Data != nil {
		issueID = strings.TrimSpace(evt.Data["issue_id"])
	}

	h.Broadcast(WSMessage{
		Type:      string(evt.Type),
		RunID:     evt.RunID,
		ProjectID: evt.ProjectID,
		IssueID:   issueID,
		Data:      data,
	})
}

func (c *wsClient) readPump() {
	defer func() {
		c.hub.unregister(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(wsMaxMessage)
	_ = c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	for {
		var msg wsClientMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			return
		}
		c.handleClientMessage(msg)
	}
}

func (c *wsClient) writePump() {
	ticker := time.NewTicker(wsPingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case payload, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *wsClient) handleClientMessage(msg wsClientMessage) {
	switch msg.Type {
	case "subscribe_run":
		id := strings.TrimSpace(msg.RunID)
		if id == "" {
			c.sendError("run_id is required")
			return
		}
		c.subMu.Lock()
		c.Runsubs[id] = struct{}{}
		c.subMu.Unlock()
		c.sendJSON(WSMessage{Type: "subscribed", RunID: id})
	case "unsubscribe_run":
		id := strings.TrimSpace(msg.RunID)
		if id == "" {
			c.sendError("run_id is required")
			return
		}
		c.subMu.Lock()
		delete(c.Runsubs, id)
		c.subMu.Unlock()
		c.sendJSON(WSMessage{Type: "unsubscribed", RunID: id})
	case "subscribe_issue":
		id := strings.TrimSpace(msg.IssueID)
		if id == "" {
			c.sendError("issue_id is required")
			return
		}
		c.subMu.Lock()
		c.issueSubs[id] = struct{}{}
		c.subMu.Unlock()
		c.sendJSON(WSMessage{Type: "subscribed", IssueID: id})
	case "unsubscribe_issue":
		id := strings.TrimSpace(msg.IssueID)
		if id == "" {
			c.sendError("issue_id is required")
			return
		}
		c.subMu.Lock()
		delete(c.issueSubs, id)
		c.subMu.Unlock()
		c.sendJSON(WSMessage{Type: "unsubscribed", IssueID: id})
	default:
		c.sendError("unsupported message type")
	}
}

func (c *wsClient) shouldReceive(msg WSMessage) bool {
	if msg.Type == "" {
		return false
	}

	if msg.Type == "agent_output" {
		if msg.RunID == "" {
			return false
		}
		c.subMu.RLock()
		_, ok := c.Runsubs[msg.RunID]
		c.subMu.RUnlock()
		return ok
	}

	eventType := core.EventType(msg.Type)
	if core.IsAlwaysBroadcastIssueEvent(eventType) {
		return true
	}
	if core.IsIssueScopedEvent(eventType) {
		if msg.IssueID == "" {
			return false
		}
		c.subMu.RLock()
		_, ok := c.issueSubs[msg.IssueID]
		c.subMu.RUnlock()
		return ok
	}

	return true
}

func (c *wsClient) sendError(message string) {
	c.sendJSON(WSMessage{
		Type: "error",
		Data: map[string]any{
			"message": message,
		},
	})
}

func (c *wsClient) sendJSON(msg WSMessage) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case c.send <- payload:
	default:
	}
}
