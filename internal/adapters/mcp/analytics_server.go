package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// AnalyticsServer exposes system analytics data as MCP tool calls.
// This is used as a library — the calling code provides the Store,
// and invokes HandleToolCall when an agent calls an MCP tool.
type AnalyticsServer struct {
	store core.AnalyticsStore
}

// NewAnalyticsServer creates a new MCP analytics server.
func NewAnalyticsServer(store core.AnalyticsStore) *AnalyticsServer {
	return &AnalyticsServer{store: store}
}

// ToolDefinition describes an MCP tool.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Tools returns the list of analytics tools this server exposes.
func (s *AnalyticsServer) Tools() []ToolDefinition {
	filterSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "number", "description": "Filter by project ID"},
			"since_hours": map[string]any{"type": "number", "description": "Look back N hours from now (default 24)"},
			"limit":       map[string]any{"type": "number", "description": "Max results (default 20)"},
		},
	}
	return []ToolDefinition{
		{
			Name:        "analytics_project_errors",
			Description: "Get projects ranked by error count. Shows which projects have the most failures.",
			InputSchema: filterSchema,
		},
		{
			Name:        "analytics_bottlenecks",
			Description: "Get step bottleneck analysis. Shows which steps are slowest or fail most.",
			InputSchema: filterSchema,
		},
		{
			Name:        "analytics_duration_stats",
			Description: "Get issue execution duration statistics. Shows avg/min/max execution time per issue.",
			InputSchema: filterSchema,
		},
		{
			Name:        "analytics_error_breakdown",
			Description: "Get error breakdown by kind (transient/permanent/need_help).",
			InputSchema: filterSchema,
		},
		{
			Name:        "analytics_recent_failures",
			Description: "Get most recent failed executions with full context.",
			InputSchema: filterSchema,
		},
		{
			Name:        "analytics_status_distribution",
			Description: "Get issue status distribution (how many pending/running/done/failed).",
			InputSchema: filterSchema,
		},
		{
			Name:        "analytics_summary",
			Description: "Get a full analytics summary combining all analytics data in one call.",
			InputSchema: filterSchema,
		},
	}
}

// HandleToolCall dispatches an MCP tool call to the appropriate analytics query.
func (s *AnalyticsServer) HandleToolCall(ctx context.Context, toolName string, input json.RawMessage) (json.RawMessage, error) {
	filter := parseFilterInput(input)

	var result any
	var err error

	switch toolName {
	case "analytics_project_errors":
		result, err = s.store.ProjectErrorRanking(ctx, filter)
	case "analytics_bottlenecks":
		result, err = s.store.IssueBottleneckSteps(ctx, filter)
	case "analytics_duration_stats":
		result, err = s.store.ExecutionDurationStats(ctx, filter)
	case "analytics_error_breakdown":
		result, err = s.store.ErrorBreakdown(ctx, filter)
	case "analytics_recent_failures":
		result, err = s.store.RecentFailures(ctx, filter)
	case "analytics_status_distribution":
		result, err = s.store.IssueStatusDistribution(ctx, filter)
	case "analytics_summary":
		result, err = s.handleSummary(ctx, filter)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w", toolName, err)
	}

	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return out, nil
}

func (s *AnalyticsServer) handleSummary(ctx context.Context, filter core.AnalyticsFilter) (any, error) {
	type summary struct {
		ProjectErrors  any `json:"project_errors"`
		Bottlenecks    any `json:"bottlenecks"`
		DurationStats  any `json:"duration_stats"`
		ErrorBreakdown any `json:"error_breakdown"`
		RecentFailures any `json:"recent_failures"`
		StatusDist     any `json:"status_distribution"`
	}

	s1, err := s.store.ProjectErrorRanking(ctx, filter)
	if err != nil {
		return nil, err
	}
	s2, err := s.store.IssueBottleneckSteps(ctx, filter)
	if err != nil {
		return nil, err
	}
	s3, err := s.store.ExecutionDurationStats(ctx, filter)
	if err != nil {
		return nil, err
	}
	s4, err := s.store.ErrorBreakdown(ctx, filter)
	if err != nil {
		return nil, err
	}
	s5, err := s.store.RecentFailures(ctx, filter)
	if err != nil {
		return nil, err
	}
	s6, err := s.store.IssueStatusDistribution(ctx, filter)
	if err != nil {
		return nil, err
	}

	return summary{
		ProjectErrors:  s1,
		Bottlenecks:    s2,
		DurationStats:  s3,
		ErrorBreakdown: s4,
		RecentFailures: s5,
		StatusDist:     s6,
	}, nil
}

type filterInput struct {
	ProjectID  *int64  `json:"project_id"`
	SinceHours float64 `json:"since_hours"`
	Limit      int     `json:"limit"`
}

func parseFilterInput(raw json.RawMessage) core.AnalyticsFilter {
	f := core.AnalyticsFilter{}
	if len(raw) == 0 {
		return f
	}
	var input filterInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return f
	}
	f.ProjectID = input.ProjectID
	f.Limit = input.Limit
	if input.SinceHours > 0 {
		since := time.Now().UTC().Add(-time.Duration(input.SinceHours * float64(time.Hour)))
		f.Since = &since
	}
	return f
}
