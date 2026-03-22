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

type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type SessionModeState struct {
	AvailableModes []SessionMode `json:"available_modes"`
	CurrentModeId  string        `json:"current_mode_id"`
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
	LLMConfigID string       `json:"llm_config_id,omitempty"`
	// UseWorktree controls whether to create a git worktree for this session.
	// nil = default behaviour (auto-detect based on project git binding),
	// true = force worktree, false = run directly in project directory.
	UseWorktree *bool `json:"use_worktree,omitempty"`
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
	// Status is "accepted" for immediate execution or "queued" when the session is busy.
	Status string `json:"status"`
}

// PendingMessage holds a queued message waiting for a busy session to become idle.
type PendingMessage struct {
	Message     string       `json:"message"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Message is one persisted chat turn in a lead session.
type Message struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

// GitStats holds lightweight git diff statistics for a session's working directory.
type GitStats struct {
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	FilesChanged int    `json:"files_changed"`
	Merged       bool   `json:"merged"`
	PrURL        string `json:"pr_url,omitempty"`
	PrNumber     int    `json:"pr_number,omitempty"`
	PrState      string `json:"pr_state,omitempty"`
}

// SessionSummary is the minimal metadata required to render a session list.
type SessionSummary struct {
	SessionID    string    `json:"session_id"`
	Title        string    `json:"title,omitempty"`
	WorkDir      string    `json:"work_dir,omitempty"`
	Branch       string    `json:"branch,omitempty"`
	WSPath       string    `json:"ws_path,omitempty"`
	ProjectID    int64     `json:"project_id,omitempty"`
	ProjectName  string    `json:"project_name,omitempty"`
	ProfileID    string    `json:"profile_id,omitempty"`
	ProfileName  string    `json:"profile_name,omitempty"`
	DriverID     string    `json:"driver_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Status       string    `json:"status"`
	Archived     bool      `json:"archived,omitempty"`
	MessageCount int       `json:"message_count"`
	Git          *GitStats `json:"git,omitempty"`
}

// SessionDetail is a session summary plus the stored conversation history.
type SessionDetail struct {
	SessionSummary
	Messages          []Message          `json:"messages"`
	AvailableCommands []AvailableCommand `json:"available_commands,omitempty"`
	ConfigOptions     []ConfigOption     `json:"config_options,omitempty"`
	Modes             *SessionModeState  `json:"modes,omitempty"`
}
