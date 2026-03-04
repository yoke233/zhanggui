package core

import "context"

// Notifier sends runtime notifications to one or more channels.
type Notifier interface {
	Plugin
	Notify(ctx context.Context, msg Notification) error
}

type Notification struct {
	Level     string
	Title     string
	Body      string
	RunID     string
	ProjectID string
	ActionURL string
}
