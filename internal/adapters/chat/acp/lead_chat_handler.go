package acp

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	acphandler "github.com/yoke233/ai-workflow/internal/adapters/agent/acp"
	"github.com/yoke233/ai-workflow/internal/core"
)

const defaultPermissionTimeout = 60 * time.Second

// leadChatHandler wraps ACPHandler and intercepts RequestPermission so it can
// be forwarded to the frontend for interactive approval.
type leadChatHandler struct {
	*acphandler.ACPHandler

	broker *permissionBroker
	bus    core.EventBus

	mu        sync.RWMutex
	sessionID string // public session ID, set after NewSession
}

func newLeadChatHandler(workDir string, bus core.EventBus, broker *permissionBroker) *leadChatHandler {
	return &leadChatHandler{
		ACPHandler: acphandler.NewACPHandler(workDir, "", nil),
		broker:     broker,
		bus:        bus,
	}
}

func (h *leadChatHandler) SetSessionID(id string) {
	h.mu.Lock()
	h.sessionID = strings.TrimSpace(id)
	h.mu.Unlock()
	h.ACPHandler.SetSessionID(id)
}

func (h *leadChatHandler) getSessionID() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sessionID
}

// RequestPermission overrides ACPHandler.RequestPermission to forward the
// request to the frontend via the event bus, then block until the user
// responds or a timeout fires.
func (h *leadChatHandler) RequestPermission(ctx context.Context, req acpproto.RequestPermissionRequest) (acpproto.RequestPermissionResponse, error) {
	if h.broker == nil || h.bus == nil {
		// Fallback to ACPHandler's default policy-based resolution.
		return h.ACPHandler.RequestPermission(ctx, req)
	}

	permID := h.broker.NextID()
	sessionID := h.getSessionID()

	// Build a frontend-friendly payload from the ACP permission request.
	payload := buildPermissionPayload(permID, sessionID, req)

	// Publish to event bus so the WS handler forwards it to the frontend.
	h.bus.Publish(ctx, core.Event{
		Type: core.EventChatPermissionRequest,
		Data: payload,
		Timestamp: time.Now().UTC(),
	})

	slog.Debug("lead chat: permission request forwarded to frontend",
		"perm_id", permID, "session_id", sessionID)

	// Block until the user responds or timeout.
	return h.broker.Submit(ctx, permID, req, defaultPermissionTimeout)
}

// buildPermissionPayload converts an ACP permission request into a map
// suitable for JSON serialisation and frontend consumption.
func buildPermissionPayload(permID, sessionID string, req acpproto.RequestPermissionRequest) map[string]any {
	options := make([]map[string]any, 0, len(req.Options))
	for _, opt := range req.Options {
		o := map[string]any{
			"option_id": string(opt.OptionId),
			"kind":      string(opt.Kind),
			"name":      opt.Name,
		}
		options = append(options, o)
	}

	toolCall := map[string]any{}
	if req.ToolCall.ToolCallId != "" {
		toolCall["tool_call_id"] = string(req.ToolCall.ToolCallId)
	}
	if req.ToolCall.Kind != nil {
		toolCall["kind"] = string(*req.ToolCall.Kind)
	}
	if req.ToolCall.Title != nil {
		toolCall["title"] = *req.ToolCall.Title
	}
	if len(req.ToolCall.Locations) > 0 {
		locs := make([]map[string]string, 0, len(req.ToolCall.Locations))
		for _, loc := range req.ToolCall.Locations {
			locs = append(locs, map[string]string{"path": loc.Path})
		}
		toolCall["locations"] = locs
	}
	if req.ToolCall.RawInput != nil {
		if raw, err := json.Marshal(req.ToolCall.RawInput); err == nil {
			toolCall["raw_input"] = json.RawMessage(raw)
		}
	}

	return map[string]any{
		"permission_id": permID,
		"session_id":    sessionID,
		"tool_call":     toolCall,
		"options":       options,
	}
}
