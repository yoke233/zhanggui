package workitemtrackapp

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
)

type ThreadReader interface {
	GetThread(ctx context.Context, id int64) (*core.Thread, error)
}

type WorkItemReader interface {
	GetWorkItem(ctx context.Context, id int64) (*core.WorkItem, error)
}

type ActionReader interface {
	ListActionsByWorkItem(ctx context.Context, workItemID int64) ([]*core.Action, error)
}

type TrackStore interface {
	core.WorkItemTrackStore
}

type WorkItemWriter interface {
	CreateWorkItem(ctx context.Context, workItem *core.WorkItem) (int64, error)
}

type ActionWriter interface {
	CreateAction(ctx context.Context, action *core.Action) (int64, error)
}

type ThreadLinkWriter interface {
	CreateThreadWorkItemLink(ctx context.Context, link *core.ThreadWorkItemLink) (int64, error)
}

type Store interface {
	ThreadReader
	WorkItemReader
	ActionReader
	TrackStore
	WorkItemWriter
	ActionWriter
	ThreadLinkWriter
}

type TxStore interface {
	Store
}

type Tx interface {
	InTx(ctx context.Context, fn func(ctx context.Context, store TxStore) error) error
}

type EventPublisher interface {
	Publish(ctx context.Context, event core.Event)
}

type WorkItemExecutor interface {
	RunWorkItem(ctx context.Context, workItemID int64) error
}

type StartTrackInput struct {
	ThreadID  int64
	Title     string
	Objective string
	CreatedBy string
	Metadata  map[string]any
}

type AttachThreadContextInput struct {
	TrackID      int64
	ThreadID     int64
	RelationType string
}

type MaterializeWorkItemInput struct {
	TrackID   int64
	ProjectID *int64
}

type SubmitForReviewInput struct {
	TrackID       int64
	LatestSummary string
	PlannerOutput map[string]any
}

type ApproveReviewInput struct {
	TrackID       int64
	LatestSummary string
	ReviewOutput  map[string]any
}

type RejectReviewInput struct {
	TrackID       int64
	LatestSummary string
	ReviewOutput  map[string]any
}

type PauseTrackInput struct {
	TrackID int64
}

type CancelTrackInput struct {
	TrackID int64
}

type ConfirmExecutionInput struct {
	TrackID   int64
	ProjectID *int64
}

type MaterializeWorkItemResult struct {
	Track    *core.WorkItemTrack        `json:"track"`
	WorkItem *core.WorkItem             `json:"work_item"`
	Links    []*core.ThreadWorkItemLink `json:"links"`
}

type ConfirmExecutionResult struct {
	Track    *core.WorkItemTrack `json:"track"`
	WorkItem *core.WorkItem      `json:"work_item"`
	Status   string              `json:"status"`
}
