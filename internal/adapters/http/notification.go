package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

// ── Request / Response types ──

type createNotificationRequest struct {
	Level     string   `json:"level,omitempty"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Category  string   `json:"category,omitempty"`
	ActionURL string   `json:"action_url,omitempty"`
	ProjectID *int64   `json:"project_id,omitempty"`
	IssueID   *int64   `json:"issue_id,omitempty"`
	ExecID    *int64   `json:"exec_id,omitempty"`
	Channels  []string `json:"channels,omitempty"`
}

type unreadCountResponse struct {
	Count int `json:"count"`
}

// ── Route registration ──

func registerNotificationRoutes(r chi.Router, h *Handler) {
	r.Get("/notifications", h.listNotifications)
	r.Post("/notifications", h.createNotification)
	r.Get("/notifications/unread-count", h.getUnreadCount)
	r.Post("/notifications/read-all", h.markAllRead)
	r.Get("/notifications/{notificationID}", h.getNotification)
	r.Post("/notifications/{notificationID}/read", h.markNotificationRead)
	r.Delete("/notifications/{notificationID}", h.deleteNotification)
}

// ── Handlers ──

func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	filter := core.NotificationFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	if cat := r.URL.Query().Get("category"); cat != "" {
		filter.Category = cat
	}
	if lvl := r.URL.Query().Get("level"); lvl != "" {
		level := core.NotificationLevel(lvl)
		filter.Level = &level
	}
	if readStr := r.URL.Query().Get("read"); readStr != "" {
		readVal := readStr == "true"
		filter.Read = &readVal
	}
	if pid, ok := queryInt64(r, "project_id"); ok {
		filter.ProjectID = &pid
	}
	if iid, ok := queryInt64(r, "issue_id"); ok {
		filter.IssueID = &iid
	}

	notifications, err := h.store.ListNotifications(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if notifications == nil {
		notifications = []*core.Notification{}
	}
	writeJSON(w, http.StatusOK, notifications)
}

func (h *Handler) createNotification(w http.ResponseWriter, r *http.Request) {
	var req createNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "MISSING_TITLE")
		return
	}

	level := core.NotificationLevel(strings.TrimSpace(req.Level))
	if level == "" {
		level = core.NotificationLevelInfo
	}

	channels := make([]core.NotificationChannel, 0, len(req.Channels))
	for _, ch := range req.Channels {
		channels = append(channels, core.NotificationChannel(ch))
	}
	if len(channels) == 0 {
		channels = []core.NotificationChannel{core.ChannelInApp, core.ChannelBrowser}
	}

	n := &core.Notification{
		Level:     level,
		Title:     title,
		Body:      req.Body,
		Category:  req.Category,
		ActionURL: req.ActionURL,
		ProjectID: req.ProjectID,
		IssueID:   req.IssueID,
		ExecID:    req.ExecID,
		Channels:  channels,
		CreatedAt: time.Now().UTC(),
	}

	id, err := h.store.CreateNotification(r.Context(), n)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	n.ID = id

	// Broadcast via EventBus so WebSocket clients receive real-time notification.
	h.bus.Publish(r.Context(), core.Event{
		Type:      core.EventNotificationCreated,
		Timestamp: n.CreatedAt,
		Data: map[string]any{
			"notification": n,
		},
	})

	writeJSON(w, http.StatusCreated, n)
}

func (h *Handler) getNotification(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "notificationID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid notification ID", "BAD_REQUEST")
		return
	}
	n, err := h.store.GetNotification(r.Context(), id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "notification not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (h *Handler) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "notificationID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid notification ID", "BAD_REQUEST")
		return
	}
	if err := h.store.MarkNotificationRead(r.Context(), id); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "notification not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	h.bus.Publish(r.Context(), core.Event{
		Type:      core.EventNotificationRead,
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"notification_id": id},
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) markAllRead(w http.ResponseWriter, r *http.Request) {
	if err := h.store.MarkAllNotificationsRead(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	h.bus.Publish(r.Context(), core.Event{
		Type:      core.EventNotificationAllRead,
		Timestamp: time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) deleteNotification(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "notificationID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid notification ID", "BAD_REQUEST")
		return
	}
	if err := h.store.DeleteNotification(r.Context(), id); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "notification not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getUnreadCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.store.CountUnreadNotifications(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, unreadCountResponse{Count: count})
}
