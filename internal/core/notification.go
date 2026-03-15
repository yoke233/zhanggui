package core

import (
	"context"
	"time"
)

// NotificationLevel indicates urgency for UI rendering and browser notification behavior.
type NotificationLevel string

const (
	NotificationLevelInfo    NotificationLevel = "info"
	NotificationLevelSuccess NotificationLevel = "success"
	NotificationLevelWarning NotificationLevel = "warning"
	NotificationLevelError   NotificationLevel = "error"
)

// NotificationChannel identifies the delivery mechanism.
type NotificationChannel string

const (
	ChannelBrowser NotificationChannel = "browser" // Web Notification API (desktop + mobile PWA)
	ChannelInApp   NotificationChannel = "in_app"  // In-app toast / notification center
	ChannelWebhook NotificationChannel = "webhook" // External webhook (Slack, Teams, etc.)
	ChannelEmail   NotificationChannel = "email"   // Email delivery
)

// Notification is the core domain entity for user-facing notifications.
type Notification struct {
	ID        int64             `json:"id"`
	Level     NotificationLevel `json:"level"`
	Title     string            `json:"title"`
	Body      string            `json:"body,omitempty"`
	Category  string            `json:"category,omitempty"`   // e.g. "issue", "exec", "chat", "system"
	ActionURL string            `json:"action_url,omitempty"` // deep-link into the UI

	// Scope: which project/issue/exec triggered this notification.
	ProjectID *int64 `json:"project_id,omitempty"`
	IssueID   *int64 `json:"issue_id,omitempty"`
	ExecID    *int64 `json:"exec_id,omitempty"`

	// Delivery tracking.
	Channels  []NotificationChannel `json:"channels,omitempty"`
	Read      bool                  `json:"read"`
	ReadAt    *time.Time            `json:"read_at,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
}

// NotificationFilter constrains notification queries.
type NotificationFilter struct {
	ProjectID *int64
	IssueID   *int64
	Category  string
	Level     *NotificationLevel
	Read      *bool
	Limit     int
	Offset    int
}

// NotificationStore persists Notification records.
type NotificationStore interface {
	CreateNotification(ctx context.Context, n *Notification) (int64, error)
	GetNotification(ctx context.Context, id int64) (*Notification, error)
	ListNotifications(ctx context.Context, filter NotificationFilter) ([]*Notification, error)
	MarkNotificationRead(ctx context.Context, id int64) error
	MarkAllNotificationsRead(ctx context.Context) error
	DeleteNotification(ctx context.Context, id int64) error
	CountUnreadNotifications(ctx context.Context) (int, error)
}

// NotificationSender is the interface for delivering notifications through a specific channel.
// Each channel (browser, webhook, email, etc.) implements this interface.
type NotificationSender interface {
	// Channel returns which delivery channel this sender handles.
	Channel() NotificationChannel
	// Send delivers the notification. Implementations should be non-blocking where possible.
	Send(ctx context.Context, n *Notification) error
}

// NotificationService orchestrates creating, persisting, and dispatching notifications
// through registered senders. Defined in core, implemented in application layer.
type NotificationService interface {
	// Notify creates a notification, persists it, and dispatches to registered channels.
	Notify(ctx context.Context, n *Notification) (*Notification, error)
	// List returns notifications matching the filter.
	List(ctx context.Context, filter NotificationFilter) ([]*Notification, error)
	// MarkRead marks a single notification as read.
	MarkRead(ctx context.Context, id int64) error
	// MarkAllRead marks all notifications as read.
	MarkAllRead(ctx context.Context) error
	// Delete removes a notification.
	Delete(ctx context.Context, id int64) error
	// UnreadCount returns the number of unread notifications.
	UnreadCount(ctx context.Context) (int, error)
}
