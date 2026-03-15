package core

import (
	"context"
	"time"
)

// AnalyticsStore provides aggregated analytics queries.
type AnalyticsStore interface {
	// ProjectErrorRanking returns projects ordered by failure count (desc).
	ProjectErrorRanking(ctx context.Context, filter AnalyticsFilter) ([]ProjectErrorRank, error)

	// WorkItemBottleneckActions returns actions that are slowest or fail most within work items.
	WorkItemBottleneckActions(ctx context.Context, filter AnalyticsFilter) ([]ActionBottleneck, error)

	// RunDurationStats returns run duration percentiles per work item.
	RunDurationStats(ctx context.Context, filter AnalyticsFilter) ([]WorkItemDurationStat, error)

	// ErrorBreakdown returns error counts grouped by error_kind.
	ErrorBreakdown(ctx context.Context, filter AnalyticsFilter) ([]ErrorKindCount, error)

	// RecentFailures returns the most recent failed runs with context.
	RecentFailures(ctx context.Context, filter AnalyticsFilter) ([]FailureRecord, error)

	// WorkItemStatusDistribution returns work item counts grouped by status.
	WorkItemStatusDistribution(ctx context.Context, filter AnalyticsFilter) ([]StatusCount, error)
}

// AnalyticsFilter constrains analytics queries.
type AnalyticsFilter struct {
	ProjectID *int64     `json:"project_id,omitempty"`
	Since     *time.Time `json:"since,omitempty"`
	Until     *time.Time `json:"until,omitempty"`
	Limit     int        `json:"limit,omitempty"`
}

// ProjectErrorRank represents a project's error ranking.
type ProjectErrorRank struct {
	ProjectID       int64   `json:"project_id"`
	ProjectName     string  `json:"project_name"`
	TotalWorkItems  int     `json:"total_work_items"`
	FailedWorkItems int     `json:"failed_work_items"`
	FailureRate     float64 `json:"failure_rate"`
	FailedRuns      int     `json:"failed_runs"`
}

// ActionBottleneck represents an action that is a bottleneck in work item execution.
type ActionBottleneck struct {
	ActionID      int64   `json:"action_id"`
	ActionName    string  `json:"action_name"`
	WorkItemID    int64   `json:"work_item_id"`
	WorkItemTitle string  `json:"work_item_title"`
	ProjectID     *int64  `json:"project_id,omitempty"`
	AvgDurationS  float64 `json:"avg_duration_s"`
	MaxDurationS  float64 `json:"max_duration_s"`
	RunCount      int     `json:"run_count"`
	FailCount     int     `json:"fail_count"`
	RetryCount    int     `json:"retry_count"`
	FailRate      float64 `json:"fail_rate"`
}

// WorkItemDurationStat provides duration statistics for a work item.
type WorkItemDurationStat struct {
	WorkItemID    int64   `json:"work_item_id"`
	WorkItemTitle string  `json:"work_item_title"`
	ProjectID     *int64  `json:"project_id,omitempty"`
	RunCount      int     `json:"run_count"`
	AvgDurationS  float64 `json:"avg_duration_s"`
	MinDurationS  float64 `json:"min_duration_s"`
	MaxDurationS  float64 `json:"max_duration_s"`
	P50DurationS  float64 `json:"p50_duration_s"`
}

// ErrorKindCount counts errors by classification.
type ErrorKindCount struct {
	ErrorKind ErrorKind `json:"error_kind"`
	Count     int       `json:"count"`
	Pct       float64   `json:"pct"`
}

// FailureRecord is a recent failed run with context.
type FailureRecord struct {
	RunID         int64     `json:"run_id"`
	ActionID      int64     `json:"action_id"`
	ActionName    string    `json:"action_name"`
	WorkItemID    int64     `json:"work_item_id"`
	WorkItemTitle string    `json:"work_item_title"`
	ProjectID     *int64    `json:"project_id,omitempty"`
	ProjectName   string    `json:"project_name,omitempty"`
	ErrorMessage  string    `json:"error_message"`
	ErrorKind     ErrorKind `json:"error_kind"`
	Attempt       int       `json:"attempt"`
	DurationS     float64   `json:"duration_s"`
	FailedAt      time.Time `json:"failed_at"`
}

// StatusCount counts work items by status.
type StatusCount struct {
	Status WorkItemStatus `json:"status"`
	Count  int            `json:"count"`
}
