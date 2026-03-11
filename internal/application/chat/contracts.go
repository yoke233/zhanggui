package chat

import "time"

type AvailableCommandInput struct {
	Hint string `json:"hint,omitempty"`
}

type AvailableCommand struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Input       *AvailableCommandInput `json:"input,omitempty"`
}

type ConfigOptionValue struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	GroupID     string `json:"group_id,omitempty"`
	GroupName   string `json:"group_name,omitempty"`
}

type ConfigOption struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	Category     string              `json:"category,omitempty"`
	Type         string              `json:"type"`
	CurrentValue string              `json:"current_value"`
	Options      []ConfigOptionValue `json:"options,omitempty"`
}

// Attachment is an image or file uploaded alongside a chat message.
type Attachment struct {
	// Name is the original filename (e.g. "screenshot.png").
	Name string `json:"name"`
	// MimeType is the MIME type (e.g. "image/png", "text/plain").
	MimeType string `json:"mime_type"`
	// Data is base64-encoded content.
	Data string `json:"data"`
}

// Request is the input for a direct chat message.
type Request struct {
	SessionID   string       `json:"session_id"`
	Message     string       `json:"message"`
	Attachments []Attachment `json:"attachments,omitempty"`
	WorkDir     string       `json:"work_dir,omitempty"`
	ProjectID   int64        `json:"project_id,omitempty"`
	ProjectName string       `json:"project_name,omitempty"`
	ProfileID   string       `json:"profile_id,omitempty"`
	DriverID    string       `json:"driver_id,omitempty"`
}

// Response is the output from a direct chat message.
type Response struct {
	SessionID string `json:"session_id"`
	Reply     string `json:"reply"`
	WSPath    string `json:"ws_path,omitempty"`
}

// AcceptedResponse is returned when a chat request has been accepted for async execution.
type AcceptedResponse struct {
	SessionID string `json:"session_id"`
	WSPath    string `json:"ws_path,omitempty"`
}

// Message is one persisted chat turn in a lead session.
type Message struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

// SessionSummary is the minimal metadata required to render a session list.
type SessionSummary struct {
	SessionID    string    `json:"session_id"`
	Title        string    `json:"title,omitempty"`
	WorkDir      string    `json:"work_dir,omitempty"`
	WSPath       string    `json:"ws_path,omitempty"`
	ProjectID    int64     `json:"project_id,omitempty"`
	ProjectName  string    `json:"project_name,omitempty"`
	ProfileID    string    `json:"profile_id,omitempty"`
	ProfileName  string    `json:"profile_name,omitempty"`
	DriverID     string    `json:"driver_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Status       string    `json:"status"`
	MessageCount int       `json:"message_count"`
}

// SessionDetail is a session summary plus the stored conversation history.
type SessionDetail struct {
	SessionSummary
	Messages          []Message          `json:"messages"`
	AvailableCommands []AvailableCommand `json:"available_commands,omitempty"`
	ConfigOptions     []ConfigOption     `json:"config_options,omitempty"`
}
