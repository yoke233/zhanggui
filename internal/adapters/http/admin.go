package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type systemEventRequest struct {
	Event string         `json:"event"`
	Data  map[string]any `json:"data,omitempty"`
}

type systemEventResponse struct {
	Status string `json:"status"`
}

func (h *Handler) sendSystemEvent(w http.ResponseWriter, r *http.Request) {
	if h.bus == nil {
		writeError(w, http.StatusServiceUnavailable, "event bus is not configured", "BUS_UNAVAILABLE")
		return
	}

	var req systemEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	req.Event = strings.TrimSpace(req.Event)
	if req.Event == "" {
		writeError(w, http.StatusBadRequest, "event is required", "MISSING_EVENT")
		return
	}
	if req.Data == nil {
		req.Data = map[string]any{}
	}

	h.bus.Publish(r.Context(), core.Event{
		Type:      core.EventType("admin.system_event"),
		Data:      map[string]any{"event": req.Event, "data": req.Data},
		Timestamp: time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, systemEventResponse{Status: "ok"})
}

