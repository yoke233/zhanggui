package acphandler

import "time"

// EventType identifies the kind of event published by the ACP handler.
// These constants originated in the legacy core package and are kept here
// because they are only used within this adapter.
type EventType string

const (
	EventRunUpdate              EventType = "run_update"
	EventTeamLeaderFilesChanged EventType = "team_leader_files_changed"
)

// Event is a lightweight domain event emitted by the ACP handler to
// notify the rest of the system about run-level state changes.
type Event struct {
	Type      EventType         `json:"type"`
	ProjectID string            `json:"project_id"`
	Data      map[string]string `json:"data,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// ChatRunEvent stores one persisted non-message runtime update for a chat session.
type ChatRunEvent struct {
	ID         int64          `json:"id"`
	SessionID  string         `json:"session_id"`
	ProjectID  string         `json:"project_id"`
	EventType  string         `json:"event_type"`
	UpdateType string         `json:"update_type"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  time.Time      `json:"created_at"`
}
