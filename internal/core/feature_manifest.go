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

// FeatureManifest is the top-level container for a project's feature checklist.
// Each project has exactly one manifest.
type FeatureManifest struct {
	ID        int64          `json:"id"`
	ProjectID int64          `json:"project_id"`
	Version   int            `json:"version"`
	Summary   string         `json:"summary,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// FeatureEntry is a single feature/scenario in the manifest.
// Entries are append-only from the Agent's perspective (no delete).
type FeatureEntry struct {
	ID          int64          `json:"id"`
	ManifestID  int64          `json:"manifest_id"`
	Key         string         `json:"key"`
	Description string         `json:"description"`
	Status      FeatureStatus  `json:"status"`
	IssueID     *int64         `json:"issue_id,omitempty"`
	StepID      *int64         `json:"step_id,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// FeatureEntryFilter constrains FeatureEntry queries.
type FeatureEntryFilter struct {
	ManifestID int64
	Status     *FeatureStatus
	IssueID    *int64
	Tags       []string
	Limit      int
	Offset     int
}

// FeatureManifestStore persists FeatureManifest and FeatureEntry aggregates.
type FeatureManifestStore interface {
	CreateFeatureManifest(ctx context.Context, m *FeatureManifest) (int64, error)
	GetFeatureManifest(ctx context.Context, id int64) (*FeatureManifest, error)
	GetFeatureManifestByProject(ctx context.Context, projectID int64) (*FeatureManifest, error)
	UpdateFeatureManifest(ctx context.Context, m *FeatureManifest) error
	DeleteFeatureManifest(ctx context.Context, id int64) error

	CreateFeatureEntry(ctx context.Context, entry *FeatureEntry) (int64, error)
	GetFeatureEntry(ctx context.Context, id int64) (*FeatureEntry, error)
	GetFeatureEntryByKey(ctx context.Context, manifestID int64, key string) (*FeatureEntry, error)
	ListFeatureEntries(ctx context.Context, filter FeatureEntryFilter) ([]*FeatureEntry, error)
	UpdateFeatureEntry(ctx context.Context, entry *FeatureEntry) error
	UpdateFeatureEntryStatus(ctx context.Context, id int64, status FeatureStatus) error
	DeleteFeatureEntry(ctx context.Context, id int64) error

	CountFeatureEntriesByStatus(ctx context.Context, manifestID int64) (map[FeatureStatus]int, error)
}
