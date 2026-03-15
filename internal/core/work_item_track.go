package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// WorkItemTrackStatus represents the lifecycle state of a task incubation track.
type WorkItemTrackStatus string

const (
	WorkItemTrackDraft                WorkItemTrackStatus = "draft"
	WorkItemTrackPlanning             WorkItemTrackStatus = "planning"
	WorkItemTrackReviewing            WorkItemTrackStatus = "reviewing"
	WorkItemTrackAwaitingConfirmation WorkItemTrackStatus = "awaiting_confirmation"
	WorkItemTrackMaterialized         WorkItemTrackStatus = "materialized"
	WorkItemTrackExecuting            WorkItemTrackStatus = "executing"
	WorkItemTrackDone                 WorkItemTrackStatus = "done"
	WorkItemTrackPaused               WorkItemTrackStatus = "paused"
	WorkItemTrackCancelled            WorkItemTrackStatus = "cancelled"
	WorkItemTrackFailed               WorkItemTrackStatus = "failed"
)

func (s WorkItemTrackStatus) Valid() bool {
	switch s {
	case WorkItemTrackDraft,
		WorkItemTrackPlanning,
		WorkItemTrackReviewing,
		WorkItemTrackAwaitingConfirmation,
		WorkItemTrackMaterialized,
		WorkItemTrackExecuting,
		WorkItemTrackDone,
		WorkItemTrackPaused,
		WorkItemTrackCancelled,
		WorkItemTrackFailed:
		return true
	default:
		return false
	}
}

func ParseWorkItemTrackStatus(raw string) (WorkItemTrackStatus, error) {
	status := WorkItemTrackStatus(strings.TrimSpace(raw))
	if !status.Valid() {
		return "", fmt.Errorf("invalid work item track status %q", raw)
	}
	return status, nil
}

func CanTransitionWorkItemTrackStatus(from, to WorkItemTrackStatus) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case WorkItemTrackDraft:
		return to == WorkItemTrackPlanning || to == WorkItemTrackReviewing || to == WorkItemTrackCancelled
	case WorkItemTrackPlanning:
		return to == WorkItemTrackReviewing || to == WorkItemTrackPaused || to == WorkItemTrackCancelled || to == WorkItemTrackFailed
	case WorkItemTrackReviewing:
		return to == WorkItemTrackAwaitingConfirmation || to == WorkItemTrackPlanning || to == WorkItemTrackPaused || to == WorkItemTrackCancelled || to == WorkItemTrackFailed
	case WorkItemTrackAwaitingConfirmation:
		return to == WorkItemTrackPlanning || to == WorkItemTrackMaterialized || to == WorkItemTrackExecuting || to == WorkItemTrackPaused || to == WorkItemTrackCancelled
	case WorkItemTrackMaterialized:
		return to == WorkItemTrackExecuting || to == WorkItemTrackDone || to == WorkItemTrackPaused || to == WorkItemTrackCancelled
	case WorkItemTrackExecuting:
		return to == WorkItemTrackDone || to == WorkItemTrackFailed || to == WorkItemTrackPaused || to == WorkItemTrackCancelled
	case WorkItemTrackPaused:
		return to == WorkItemTrackPlanning || to == WorkItemTrackReviewing || to == WorkItemTrackAwaitingConfirmation || to == WorkItemTrackMaterialized || to == WorkItemTrackExecuting || to == WorkItemTrackCancelled
	case WorkItemTrackDone, WorkItemTrackCancelled, WorkItemTrackFailed:
		return false
	default:
		return false
	}
}

// WorkItemTrackThreadRelation identifies how a Thread participates in a Track.
type WorkItemTrackThreadRelation string

const (
	WorkItemTrackThreadPrimary WorkItemTrackThreadRelation = "primary"
	WorkItemTrackThreadSource  WorkItemTrackThreadRelation = "source"
	WorkItemTrackThreadContext WorkItemTrackThreadRelation = "context"
)

func (r WorkItemTrackThreadRelation) Valid() bool {
	switch r {
	case WorkItemTrackThreadPrimary, WorkItemTrackThreadSource, WorkItemTrackThreadContext:
		return true
	default:
		return false
	}
}

func ParseWorkItemTrackThreadRelation(raw string) (WorkItemTrackThreadRelation, error) {
	relation := WorkItemTrackThreadRelation(strings.TrimSpace(raw))
	if !relation.Valid() {
		return "", fmt.Errorf("invalid work item track thread relation %q", raw)
	}
	return relation, nil
}

// WorkItemTrack is the persisted truth source for task incubation within Threads.
type WorkItemTrack struct {
	ID                       int64               `json:"id"`
	Title                    string              `json:"title"`
	Objective                string              `json:"objective"`
	Status                   WorkItemTrackStatus `json:"status"`
	PrimaryThreadID          *int64              `json:"primary_thread_id,omitempty"`
	WorkItemID               *int64              `json:"work_item_id,omitempty"`
	PlannerStatus            string              `json:"planner_status,omitempty"`
	ReviewerStatus           string              `json:"reviewer_status,omitempty"`
	AwaitingUserConfirmation bool                `json:"awaiting_user_confirmation"`
	LatestSummary            string              `json:"latest_summary,omitempty"`
	PlannerOutput            map[string]any      `json:"planner_output_json,omitempty"`
	ReviewOutput             map[string]any      `json:"review_output_json,omitempty"`
	Metadata                 map[string]any      `json:"metadata_json,omitempty"`
	CreatedBy                string              `json:"created_by,omitempty"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`
}

// WorkItemTrackThread links a Track to a participating Thread.
type WorkItemTrackThread struct {
	ID           int64                       `json:"id"`
	TrackID      int64                       `json:"track_id"`
	ThreadID     int64                       `json:"thread_id"`
	RelationType WorkItemTrackThreadRelation `json:"relation_type"`
	CreatedAt    time.Time                   `json:"created_at"`
}

// WorkItemTrackFilter constrains WorkItemTrack queries.
type WorkItemTrackFilter struct {
	Status          *WorkItemTrackStatus
	PrimaryThreadID *int64
	WorkItemID      *int64
	Limit           int
	Offset          int
}

// WorkItemTrackStore persists WorkItemTrack aggregates.
type WorkItemTrackStore interface {
	CreateWorkItemTrack(ctx context.Context, track *WorkItemTrack) (int64, error)
	GetWorkItemTrack(ctx context.Context, id int64) (*WorkItemTrack, error)
	ListWorkItemTracks(ctx context.Context, filter WorkItemTrackFilter) ([]*WorkItemTrack, error)
	UpdateWorkItemTrack(ctx context.Context, track *WorkItemTrack) error
	UpdateWorkItemTrackStatus(ctx context.Context, id int64, status WorkItemTrackStatus) error
	AttachThreadToWorkItemTrack(ctx context.Context, link *WorkItemTrackThread) (int64, error)
	ListWorkItemTrackThreads(ctx context.Context, trackID int64) ([]*WorkItemTrackThread, error)
	ListWorkItemTracksByThread(ctx context.Context, threadID int64) ([]*WorkItemTrack, error)
	ListWorkItemTracksByWorkItem(ctx context.Context, workItemID int64) ([]*WorkItemTrack, error)
}
