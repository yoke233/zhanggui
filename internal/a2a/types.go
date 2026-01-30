package a2a

import "time"

const (
	TaskStateUnspecified   = "TASK_STATE_UNSPECIFIED"
	TaskStateSubmitted     = "TASK_STATE_SUBMITTED"
	TaskStateWorking       = "TASK_STATE_WORKING"
	TaskStateCompleted     = "TASK_STATE_COMPLETED"
	TaskStateFailed        = "TASK_STATE_FAILED"
	TaskStateCanceled      = "TASK_STATE_CANCELED"
	TaskStateInputRequired = "TASK_STATE_INPUT_REQUIRED"
	TaskStateRejected      = "TASK_STATE_REJECTED"
	TaskStateAuthRequired  = "TASK_STATE_AUTH_REQUIRED"

	RoleUser  = "ROLE_USER"
	RoleAgent = "ROLE_AGENT"
)

type SendMessageConfiguration struct {
	AcceptedOutputModes    []string       `json:"acceptedOutputModes,omitempty"`
	PushNotificationConfig map[string]any `json:"pushNotificationConfig,omitempty"`
	HistoryLength          *int32         `json:"historyLength,omitempty"`
	Blocking               bool           `json:"blocking,omitempty"`
}

type SendMessageRequest struct {
	Message       Message                   `json:"message"`
	Configuration *SendMessageConfiguration `json:"configuration,omitempty"`
	Metadata      map[string]any            `json:"metadata,omitempty"`
	Tenant        string                    `json:"tenant,omitempty"`
}

type Part struct {
	Text      *string        `json:"text,omitempty"`
	Raw       *string        `json:"raw,omitempty"`
	URL       *string        `json:"url,omitempty"`
	Data      any            `json:"data,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Filename  string         `json:"filename,omitempty"`
	MediaType string         `json:"mediaType,omitempty"`
}

type Message struct {
	MessageID        string         `json:"messageId"`
	ContextID        string         `json:"contextId,omitempty"`
	TaskID           string         `json:"taskId,omitempty"`
	Role             string         `json:"role"`
	Parts            []Part         `json:"parts"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Extensions       []string       `json:"extensions,omitempty"`
	ReferenceTaskIDs []string       `json:"referenceTaskIds,omitempty"`
}

type TaskStatus struct {
	State     string   `json:"state"`
	Message   *Message `json:"message,omitempty"`
	Timestamp string   `json:"timestamp,omitempty"`
}

type Artifact struct {
	ArtifactID  string         `json:"artifactId"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parts       []Part         `json:"parts"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Extensions  []string       `json:"extensions,omitempty"`
}

type Task struct {
	ID        string         `json:"id"`
	ContextID string         `json:"contextId"`
	Status    TaskStatus     `json:"status"`
	Artifacts []Artifact     `json:"artifacts,omitempty"`
	History   []Message      `json:"history,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type TaskStatusUpdateEvent struct {
	TaskID    string         `json:"taskId"`
	ContextID string         `json:"contextId"`
	Status    TaskStatus     `json:"status"`
	Final     bool           `json:"final,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type TaskArtifactUpdateEvent struct {
	TaskID    string         `json:"taskId"`
	ContextID string         `json:"contextId"`
	Artifact  Artifact       `json:"artifact"`
	Append    bool           `json:"append,omitempty"`
	LastChunk bool           `json:"lastChunk,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type StreamResponse struct {
	Task           *Task                    `json:"task,omitempty"`
	Message        *Message                 `json:"message,omitempty"`
	StatusUpdate   *TaskStatusUpdateEvent   `json:"statusUpdate,omitempty"`
	ArtifactUpdate *TaskArtifactUpdateEvent `json:"artifactUpdate,omitempty"`
}

type ListTasksResponse struct {
	Tasks         []Task `json:"tasks"`
	NextPageToken string `json:"nextPageToken"`
	PageSize      int    `json:"pageSize"`
	TotalSize     int    `json:"totalSize"`
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
