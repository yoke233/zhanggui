package core

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

type Task struct {
	ID         string
	ContextID  string
	Status     TaskStatus
	Artifacts  []Artifact
	History    []Message
	Activities []Activity
	Metadata   map[string]any
}

type TaskStatus struct {
	State     string
	Message   *Message
	Timestamp time.Time
}

type Message struct {
	MessageID        string
	ContextID        string
	TaskID           string
	Role             string
	Parts            []Part
	Metadata         map[string]any
	Extensions       []string
	ReferenceTaskIDs []string
}

type Part struct {
	Text      *string
	Raw       []byte
	URL       *string
	Data      any
	Metadata  map[string]any
	Filename  string
	MediaType string
}

type Artifact struct {
	ArtifactID  string
	Name        string
	Description string
	Parts       []Part
	Metadata    map[string]any
	Extensions  []string
}

type TaskStatusUpdateEvent struct {
	TaskID    string
	ContextID string
	Status    TaskStatus
	Metadata  map[string]any
}

type TaskArtifactUpdateEvent struct {
	TaskID    string
	ContextID string
	Artifact  Artifact
	Append    bool
	LastChunk bool
	Metadata  map[string]any
}

type StreamEvent struct {
	Task           *Task
	Message        *Message
	StatusUpdate   *TaskStatusUpdateEvent
	ArtifactUpdate *TaskArtifactUpdateEvent
}

type Activity struct {
	ActivityType string
	Content      map[string]any
	Timestamp    time.Time
}

type Action struct {
	Name              string
	SurfaceID         string
	SourceComponentID string
	Timestamp         time.Time
	Context           map[string]any
}

type Interrupt struct {
	ID        string
	Reason    string
	Payload   map[string]any
	CreatedAt time.Time
}
