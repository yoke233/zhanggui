package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TaskGroupStatus represents the lifecycle state of a ThreadTaskGroup.
type TaskGroupStatus string

const (
	TaskGroupPending TaskGroupStatus = "pending"
	TaskGroupRunning TaskGroupStatus = "running"
	TaskGroupDone    TaskGroupStatus = "done"
	TaskGroupFailed  TaskGroupStatus = "failed"
)

func (s TaskGroupStatus) Valid() bool {
	switch s {
	case TaskGroupPending, TaskGroupRunning, TaskGroupDone, TaskGroupFailed:
		return true
	default:
		return false
	}
}

func ParseTaskGroupStatus(raw string) (TaskGroupStatus, error) {
	status := TaskGroupStatus(strings.TrimSpace(raw))
	if !status.Valid() {
		return "", fmt.Errorf("invalid task group status %q", raw)
	}
	return status, nil
}

func CanTransitionTaskGroupStatus(from, to TaskGroupStatus) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case TaskGroupPending:
		return to == TaskGroupRunning
	case TaskGroupRunning:
		return to == TaskGroupDone || to == TaskGroupFailed
	case TaskGroupDone, TaskGroupFailed:
		return false
	default:
		return false
	}
}

// TaskType distinguishes work vs review tasks.
type TaskType string

const (
	TaskTypeWork   TaskType = "work"
	TaskTypeReview TaskType = "review"
)

func (t TaskType) Valid() bool {
	switch t {
	case TaskTypeWork, TaskTypeReview:
		return true
	default:
		return false
	}
}

func ParseTaskType(raw string) (TaskType, error) {
	tt := TaskType(strings.TrimSpace(raw))
	if !tt.Valid() {
		return "", fmt.Errorf("invalid task type %q", raw)
	}
	return tt, nil
}

// ThreadTaskStatus represents the lifecycle state of a ThreadTask.
type ThreadTaskStatus string

const (
	ThreadTaskPending  ThreadTaskStatus = "pending"
	ThreadTaskReady    ThreadTaskStatus = "ready"
	ThreadTaskRunning  ThreadTaskStatus = "running"
	ThreadTaskDone     ThreadTaskStatus = "done"
	ThreadTaskRejected ThreadTaskStatus = "rejected"
	ThreadTaskFailed   ThreadTaskStatus = "failed"
)

func (s ThreadTaskStatus) Valid() bool {
	switch s {
	case ThreadTaskPending, ThreadTaskReady, ThreadTaskRunning, ThreadTaskDone, ThreadTaskRejected, ThreadTaskFailed:
		return true
	default:
		return false
	}
}

func (s ThreadTaskStatus) Terminal() bool {
	return s == ThreadTaskDone || s == ThreadTaskFailed
}

func ParseThreadTaskStatus(raw string) (ThreadTaskStatus, error) {
	status := ThreadTaskStatus(strings.TrimSpace(raw))
	if !status.Valid() {
		return "", fmt.Errorf("invalid thread task status %q", raw)
	}
	return status, nil
}

func CanTransitionThreadTaskStatus(from, to ThreadTaskStatus) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case ThreadTaskPending:
		return to == ThreadTaskReady
	case ThreadTaskReady:
		return to == ThreadTaskRunning
	case ThreadTaskRunning:
		return to == ThreadTaskDone || to == ThreadTaskRejected || to == ThreadTaskFailed
	case ThreadTaskDone:
		// review reject can reset upstream work task from done → pending
		return to == ThreadTaskPending
	case ThreadTaskRejected:
		return to == ThreadTaskPending
	case ThreadTaskFailed:
		return false
	default:
		return false
	}
}

// ThreadTaskGroup is a single DAG of tasks within a Thread.
type ThreadTaskGroup struct {
	ID               int64           `json:"id"`
	ThreadID         int64           `json:"thread_id"`
	Status           TaskGroupStatus `json:"status"`
	SourceMessageID  *int64          `json:"source_message_id,omitempty"`
	StatusMessageID  *int64          `json:"status_message_id,omitempty"`
	NotifyOnComplete bool            `json:"notify_on_complete"`
	CreatedAt        time.Time       `json:"created_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
}

// ThreadTask is a single node in a ThreadTaskGroup DAG.
type ThreadTask struct {
	ID              int64            `json:"id"`
	GroupID         int64            `json:"group_id"`
	ThreadID        int64            `json:"thread_id"`
	Assignee        string           `json:"assignee"`
	Type            TaskType         `json:"type"`
	Instruction     string           `json:"instruction"`
	DependsOn       []int64          `json:"depends_on"`
	Status          ThreadTaskStatus `json:"status"`
	OutputFilePath  string           `json:"output_file_path,omitempty"`
	OutputMessageID *int64           `json:"output_message_id,omitempty"`
	ReviewFeedback  string           `json:"review_feedback,omitempty"`
	MaxRetries      int              `json:"max_retries"`
	RetryCount      int              `json:"retry_count"`
	CreatedAt       time.Time        `json:"created_at"`
	CompletedAt     *time.Time       `json:"completed_at,omitempty"`
}

// ThreadTaskGroupDetail bundles a group with its tasks for API responses.
type ThreadTaskGroupDetail struct {
	ThreadTaskGroup
	Tasks []*ThreadTask `json:"tasks"`
}

// ThreadTaskGroupFilter constrains ThreadTaskGroup queries.
type ThreadTaskGroupFilter struct {
	ThreadID *int64
	Status   *TaskGroupStatus
	Limit    int
	Offset   int
}

// ThreadTaskStore persists ThreadTaskGroup and ThreadTask aggregates.
type ThreadTaskStore interface {
	CreateThreadTaskGroup(ctx context.Context, group *ThreadTaskGroup) (int64, error)
	GetThreadTaskGroup(ctx context.Context, id int64) (*ThreadTaskGroup, error)
	ListThreadTaskGroups(ctx context.Context, filter ThreadTaskGroupFilter) ([]*ThreadTaskGroup, error)
	UpdateThreadTaskGroup(ctx context.Context, group *ThreadTaskGroup) error
	DeleteThreadTaskGroup(ctx context.Context, id int64) error

	CreateThreadTask(ctx context.Context, task *ThreadTask) (int64, error)
	GetThreadTask(ctx context.Context, id int64) (*ThreadTask, error)
	ListThreadTasksByGroup(ctx context.Context, groupID int64) ([]*ThreadTask, error)
	UpdateThreadTask(ctx context.Context, task *ThreadTask) error
	DeleteThreadTasksByGroup(ctx context.Context, groupID int64) error
}
