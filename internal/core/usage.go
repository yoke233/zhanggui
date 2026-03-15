package core

import (
	"context"
	"time"
)

// UsageRecord captures token consumption for a single run.
type UsageRecord struct {
	ID               int64     `json:"id"`
	RunID            int64     `json:"run_id"`
	WorkItemID       int64     `json:"work_item_id"`
	ActionID         int64     `json:"action_id"`
	ProjectID        *int64    `json:"project_id,omitempty"`
	AgentID          string    `json:"agent_id"`
	ProfileID        string    `json:"profile_id,omitempty"`
	ModelID          string    `json:"model_id,omitempty"`
	InputTokens      int64     `json:"input_tokens"`
	OutputTokens     int64     `json:"output_tokens"`
	CacheReadTokens  int64     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64     `json:"cache_write_tokens,omitempty"`
	ReasoningTokens  int64     `json:"reasoning_tokens,omitempty"`
	TotalTokens      int64     `json:"total_tokens"`
	DurationMs       int64     `json:"duration_ms,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// UsageStore persists and aggregates token usage records.
type UsageStore interface {
	CreateUsageRecord(ctx context.Context, r *UsageRecord) (int64, error)
	GetUsageRecord(ctx context.Context, id int64) (*UsageRecord, error)
	GetUsageByRun(ctx context.Context, runID int64) (*UsageRecord, error)

	// Aggregation queries
	UsageByProject(ctx context.Context, filter AnalyticsFilter) ([]ProjectUsageSummary, error)
	UsageByAgent(ctx context.Context, filter AnalyticsFilter) ([]AgentUsageSummary, error)
	UsageByProfile(ctx context.Context, filter AnalyticsFilter) ([]ProfileUsageSummary, error)
	UsageTotals(ctx context.Context, filter AnalyticsFilter) (*UsageTotalSummary, error)
}

// ProjectUsageSummary aggregates token usage per project.
type ProjectUsageSummary struct {
	ProjectID        int64  `json:"project_id"`
	ProjectName      string `json:"project_name"`
	RunCount         int    `json:"run_count"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	ReasoningTokens  int64  `json:"reasoning_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// AgentUsageSummary aggregates token usage per agent.
type AgentUsageSummary struct {
	AgentID          string `json:"agent_id"`
	ProjectID        *int64 `json:"project_id,omitempty"`
	ProjectName      string `json:"project_name,omitempty"`
	RunCount         int    `json:"run_count"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	ReasoningTokens  int64  `json:"reasoning_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// ProfileUsageSummary aggregates token usage per profile.
type ProfileUsageSummary struct {
	ProfileID        string `json:"profile_id"`
	AgentID          string `json:"agent_id"`
	ProjectID        *int64 `json:"project_id,omitempty"`
	ProjectName      string `json:"project_name,omitempty"`
	RunCount         int    `json:"run_count"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	ReasoningTokens  int64  `json:"reasoning_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// UsageTotalSummary provides overall token usage totals.
type UsageTotalSummary struct {
	RunCount         int   `json:"run_count"`
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	ReasoningTokens  int64 `json:"reasoning_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// UsageAnalyticsSummary is the composite response for the usage analytics endpoint.
type UsageAnalyticsSummary struct {
	Totals    *UsageTotalSummary    `json:"totals"`
	ByProject []ProjectUsageSummary `json:"by_project"`
	ByAgent   []AgentUsageSummary   `json:"by_agent"`
	ByProfile []ProfileUsageSummary `json:"by_profile"`
}
