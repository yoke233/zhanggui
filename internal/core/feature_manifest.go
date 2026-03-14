package core

import (
	"context"
	"time"
)

// FeatureStatus represents the verification state of a feature entry.
type FeatureStatus string

const (
	FeaturePending FeatureStatus = "pending"
	FeaturePass    FeatureStatus = "pass"
	FeatureFail    FeatureStatus = "fail"
	FeatureSkipped FeatureStatus = "skipped"
)

// FeatureEntry is a single feature/scenario in a project's feature checklist.
// Entries are append-only from the Agent's perspective (no delete).
type FeatureEntry struct {
	ID          int64          `json:"id"`
	ProjectID   int64          `json:"project_id"`
	Key         string         `json:"key"`
	Description string         `json:"description"`
	Status      FeatureStatus  `json:"status"`
	WorkItemID  *int64         `json:"work_item_id,omitempty"`
	ActionID    *int64         `json:"action_id,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// FeatureEntryFilter constrains FeatureEntry queries.
type FeatureEntryFilter struct {
	ProjectID  int64
	Status     *FeatureStatus
	WorkItemID *int64
	Tags       []string
	Limit      int
	Offset     int
}

// FeatureEntryStore persists FeatureEntry aggregates.
type FeatureEntryStore interface {
	CreateFeatureEntry(ctx context.Context, entry *FeatureEntry) (int64, error)
	GetFeatureEntry(ctx context.Context, id int64) (*FeatureEntry, error)
	GetFeatureEntryByKey(ctx context.Context, projectID int64, key string) (*FeatureEntry, error)
	ListFeatureEntries(ctx context.Context, filter FeatureEntryFilter) ([]*FeatureEntry, error)
	UpdateFeatureEntry(ctx context.Context, entry *FeatureEntry) error
	UpdateFeatureEntryStatus(ctx context.Context, id int64, status FeatureStatus) error
	DeleteFeatureEntry(ctx context.Context, id int64) error

	CountFeatureEntriesByStatus(ctx context.Context, projectID int64) (map[FeatureStatus]int, error)
}
