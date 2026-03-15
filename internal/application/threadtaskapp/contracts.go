package threadtaskapp

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Store is the persistence contract for ThreadTask operations.
type Store interface {
	core.ThreadTaskStore

	GetThread(ctx context.Context, id int64) (*core.Thread, error)
	CreateThreadMessage(ctx context.Context, msg *core.ThreadMessage) (int64, error)
}

// WorkItemMaterializer creates a WorkItem from a completed task group.
type WorkItemMaterializer interface {
	MaterializeWorkItem(ctx context.Context, input MaterializeInput) (*MaterializeResult, error)
}

// MaterializeInput describes the data needed to create a WorkItem from a task group.
type MaterializeInput struct {
	ThreadID    int64
	GroupID     int64
	Title       string
	Body        string
	OutputFiles []string
}

// MaterializeResult holds the IDs of the created WorkItem and link.
type MaterializeResult struct {
	WorkItemID int64
	LinkID     int64
}

// EventPublisher publishes domain events.
type EventPublisher interface {
	Publish(ctx context.Context, event core.Event)
}

// NotificationSender sends user notifications.
type NotificationSender interface {
	Notify(ctx context.Context, n *core.Notification) (*core.Notification, error)
}

// AgentDispatcher dispatches tasks to agent runtime.
// AgentDispatcher dispatches tasks to agent runtime.
type AgentDispatcher interface {
	InviteAgent(ctx context.Context, threadID int64, profileID string) (*core.ThreadMember, error)
	WaitAgentReady(ctx context.Context, threadID int64, profileID string) error
	SendMessage(ctx context.Context, threadID int64, profileID string, message string) error
}

// CreateTaskGroupInput is the request for creating a new TaskGroup with tasks.
type CreateTaskGroupInput struct {
	ThreadID              int64
	SourceMessageID       *int64
	NotifyOnComplete      bool
	MaterializeToWorkItem bool
	Tasks                 []CreateTaskInput
}

// CreateTaskInput describes a single task within a CreateTaskGroupInput.
type CreateTaskInput struct {
	Assignee       string `json:"assignee"`
	Type           string `json:"type"`
	Instruction    string `json:"instruction"`
	DependsOnIndex []int  `json:"depends_on_index"`
	MaxRetries     *int   `json:"max_retries,omitempty"`
	OutputFileName string `json:"output_file_name"`
}

// SignalInput is the request for signaling a task completion or rejection.
type SignalInput struct {
	TaskID         int64
	Action         string // "complete" or "reject"
	OutputFilePath string
	Feedback       string
}
