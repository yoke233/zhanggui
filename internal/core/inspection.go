package core

import (
	"context"
	"time"
)

// InspectionStatus represents the state of an inspection report.
type InspectionStatus string

const (
	InspectionStatusPending   InspectionStatus = "pending"
	InspectionStatusRunning   InspectionStatus = "running"
	InspectionStatusCompleted InspectionStatus = "completed"
	InspectionStatusFailed    InspectionStatus = "failed"
)

// InspectionTrigger records how the inspection was initiated.
type InspectionTrigger string

const (
	InspectionTriggerCron   InspectionTrigger = "cron"
	InspectionTriggerManual InspectionTrigger = "manual"
)

// FindingSeverity classifies the importance of a finding.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "critical"
	SeverityHigh     FindingSeverity = "high"
	SeverityMedium   FindingSeverity = "medium"
	SeverityLow      FindingSeverity = "low"
	SeverityInfo     FindingSeverity = "info"
)

// FindingCategory classifies what kind of finding it is.
type FindingCategory string

const (
	CategoryBlocker    FindingCategory = "blocker"    // stuck/hung work items
	CategoryFailure    FindingCategory = "failure"    // repeated failures
	CategoryBottleneck FindingCategory = "bottleneck" // slow actions
	CategoryPattern    FindingCategory = "pattern"    // recurring error patterns
	CategoryWaste      FindingCategory = "waste"      // token/resource waste
	CategorySkillGap   FindingCategory = "skill_gap"  // missing skill opportunity
	CategoryDrift      FindingCategory = "drift"      // deviation from expected behavior
)

// InspectionReport is a single inspection run that aggregates system health data,
// identifies findings, and produces evolution insights.
type InspectionReport struct {
	ID        int64             `json:"id"`
	ProjectID *int64            `json:"project_id,omitempty"`
	Status    InspectionStatus  `json:"status"`
	Trigger   InspectionTrigger `json:"trigger"`

	// Time window inspected.
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`

	// Aggregated snapshot at inspection time.
	Snapshot *InspectionSnapshot `json:"snapshot,omitempty"`

	// Findings discovered during inspection.
	Findings []InspectionFinding `json:"findings,omitempty"`

	// Evolution insights: lessons learned and improvement suggestions.
	Insights []InspectionInsight `json:"insights,omitempty"`

	// Summary produced by the inspection engine (human-readable).
	Summary string `json:"summary,omitempty"`

	// Suggested skills to crystallize from the findings.
	SuggestedSkills []SuggestedSkill `json:"suggested_skills,omitempty"`

	// Error message if status == failed.
	ErrorMessage string `json:"error_message,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// InspectionSnapshot captures a quantitative picture of system state at inspection time.
type InspectionSnapshot struct {
	TotalWorkItems     int                `json:"total_work_items"`
	ActiveWorkItems    int                `json:"active_work_items"`
	FailedWorkItems    int                `json:"failed_work_items"`
	BlockedWorkItems   int                `json:"blocked_work_items"`
	SuccessRate        float64            `json:"success_rate"`
	AvgDurationS       float64            `json:"avg_duration_s"`
	TotalRuns          int                `json:"total_runs"`
	FailedRuns         int                `json:"failed_runs"`
	TotalTokens        int64              `json:"total_tokens"`
	TopErrors          []ErrorKindCount   `json:"top_errors,omitempty"`
	TopBottlenecks     []ActionBottleneck `json:"top_bottlenecks,omitempty"`
	StatusDistribution []StatusCount      `json:"status_distribution,omitempty"`
}

// InspectionFinding is a single problem or observation discovered during inspection.
type InspectionFinding struct {
	ID           int64           `json:"id"`
	InspectionID int64           `json:"inspection_id"`
	Category     FindingCategory `json:"category"`
	Severity     FindingSeverity `json:"severity"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Evidence     string          `json:"evidence,omitempty"`

	// References to related entities.
	WorkItemID *int64 `json:"work_item_id,omitempty"`
	ActionID   *int64 `json:"action_id,omitempty"`
	RunID      *int64 `json:"run_id,omitempty"`
	ProjectID  *int64 `json:"project_id,omitempty"`

	// Suggested action for resolution.
	Recommendation string `json:"recommendation,omitempty"`

	// Whether this finding is a recurrence of a previous one.
	Recurring       bool `json:"recurring"`
	OccurrenceCount int  `json:"occurrence_count"`

	CreatedAt time.Time `json:"created_at"`
}

// InspectionInsight is an evolution-oriented lesson learned from inspection findings.
type InspectionInsight struct {
	ID           int64  `json:"id"`
	InspectionID int64  `json:"inspection_id"`
	Type         string `json:"type"` // "lesson", "optimization", "pattern", "prediction"
	Title        string `json:"title"`
	Description  string `json:"description"`

	// What changed compared to previous inspections.
	Trend string `json:"trend,omitempty"` // "improving", "degrading", "stable", "new"

	// Concrete action items.
	ActionItems []string `json:"action_items,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// SuggestedSkill represents a skill that should be created based on recurring patterns.
type SuggestedSkill struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Rationale    string `json:"rationale"`
	SkillMDDraft string `json:"skill_md_draft,omitempty"`
}

// InspectionFilter constrains inspection queries.
type InspectionFilter struct {
	ProjectID *int64            `json:"project_id,omitempty"`
	Status    *InspectionStatus `json:"status,omitempty"`
	Since     *time.Time        `json:"since,omitempty"`
	Until     *time.Time        `json:"until,omitempty"`
	Limit     int               `json:"limit,omitempty"`
	Offset    int               `json:"offset,omitempty"`
}

// InspectionStore persists inspection reports and findings.
type InspectionStore interface {
	CreateInspection(ctx context.Context, report *InspectionReport) (int64, error)
	GetInspection(ctx context.Context, id int64) (*InspectionReport, error)
	ListInspections(ctx context.Context, filter InspectionFilter) ([]*InspectionReport, error)
	UpdateInspection(ctx context.Context, report *InspectionReport) error

	CreateFinding(ctx context.Context, finding *InspectionFinding) (int64, error)
	ListFindingsByInspection(ctx context.Context, inspectionID int64) ([]*InspectionFinding, error)
	ListRecentFindings(ctx context.Context, category FindingCategory, limit int) ([]*InspectionFinding, error)

	CreateInsight(ctx context.Context, insight *InspectionInsight) (int64, error)
	ListInsightsByInspection(ctx context.Context, inspectionID int64) ([]*InspectionInsight, error)

	// GetFindingRecurrenceCount returns how many times a similar finding (same category + same entity)
	// appeared in past inspections.
	GetFindingRecurrenceCount(ctx context.Context, category FindingCategory, workItemID, actionID *int64) (int, error)
}

// Inspection events.
const (
	EventInspectionStarted   EventType = "inspection.started"
	EventInspectionCompleted EventType = "inspection.completed"
	EventInspectionFailed    EventType = "inspection.failed"
)
