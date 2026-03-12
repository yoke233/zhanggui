package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// CreateUsageRecord inserts a usage record for an execution.
func (s *Store) CreateUsageRecord(ctx context.Context, r *core.UsageRecord) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO usage_records (
			execution_id, issue_id, step_id, project_id,
			agent_id, profile_id, model_id,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			reasoning_tokens, total_tokens, duration_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ExecutionID, r.IssueID, r.StepID, r.ProjectID,
		r.AgentID, r.ProfileID, r.ModelID,
		r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheWriteTokens,
		r.ReasoningTokens, r.TotalTokens, r.DurationMs,
	)
	if err != nil {
		return 0, fmt.Errorf("insert usage record: %w", err)
	}
	return res.LastInsertId()
}

// GetUsageRecord returns a usage record by ID.
func (s *Store) GetUsageRecord(ctx context.Context, id int64) (*core.UsageRecord, error) {
	r := &core.UsageRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, execution_id, issue_id, step_id, project_id,
			agent_id, profile_id, model_id,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			reasoning_tokens, total_tokens, duration_ms, created_at
		FROM usage_records WHERE id = ?`, id,
	).Scan(
		&r.ID, &r.ExecutionID, &r.IssueID, &r.StepID, &r.ProjectID,
		&r.AgentID, &r.ProfileID, &r.ModelID,
		&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens,
		&r.ReasoningTokens, &r.TotalTokens, &r.DurationMs, &r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("usage record %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get usage record: %w", err)
	}
	return r, nil
}

// GetUsageByExecution returns the usage record for a specific execution.
func (s *Store) GetUsageByExecution(ctx context.Context, executionID int64) (*core.UsageRecord, error) {
	r := &core.UsageRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, execution_id, issue_id, step_id, project_id,
			agent_id, profile_id, model_id,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			reasoning_tokens, total_tokens, duration_ms, created_at
		FROM usage_records WHERE execution_id = ?`, executionID,
	).Scan(
		&r.ID, &r.ExecutionID, &r.IssueID, &r.StepID, &r.ProjectID,
		&r.AgentID, &r.ProfileID, &r.ModelID,
		&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens,
		&r.ReasoningTokens, &r.TotalTokens, &r.DurationMs, &r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get usage by execution: %w", err)
	}
	return r, nil
}

// UsageByProject aggregates token usage per project.
func (s *Store) UsageByProject(ctx context.Context, filter core.AnalyticsFilter) ([]core.ProjectUsageSummary, error) {
	query := `
		SELECT
			COALESCE(u.project_id, 0),
			COALESCE(p.name, '(no project)'),
			COUNT(*) AS exec_count,
			SUM(u.input_tokens),
			SUM(u.output_tokens),
			SUM(u.cache_read_tokens),
			SUM(u.cache_write_tokens),
			SUM(u.reasoning_tokens),
			SUM(u.total_tokens)
		FROM usage_records u
		LEFT JOIN projects p ON p.id = u.project_id`

	conditions, args := usageFilterConditions(filter)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " GROUP BY u.project_id ORDER BY SUM(u.total_tokens) DESC"
	query += limitClause(filter.Limit, 50)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage by project: %w", err)
	}
	defer rows.Close()

	var out []core.ProjectUsageSummary
	for rows.Next() {
		var r core.ProjectUsageSummary
		if err := rows.Scan(
			&r.ProjectID, &r.ProjectName, &r.ExecutionCount,
			&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens,
			&r.ReasoningTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan project usage: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UsageByAgent aggregates token usage per agent.
func (s *Store) UsageByAgent(ctx context.Context, filter core.AnalyticsFilter) ([]core.AgentUsageSummary, error) {
	query := `
		SELECT
			u.agent_id,
			u.project_id,
			COALESCE(p.name, ''),
			COUNT(*) AS exec_count,
			SUM(u.input_tokens),
			SUM(u.output_tokens),
			SUM(u.cache_read_tokens),
			SUM(u.cache_write_tokens),
			SUM(u.reasoning_tokens),
			SUM(u.total_tokens)
		FROM usage_records u
		LEFT JOIN projects p ON p.id = u.project_id`

	conditions, args := usageFilterConditions(filter)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " GROUP BY u.agent_id, u.project_id ORDER BY SUM(u.total_tokens) DESC"
	query += limitClause(filter.Limit, 50)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage by agent: %w", err)
	}
	defer rows.Close()

	var out []core.AgentUsageSummary
	for rows.Next() {
		var r core.AgentUsageSummary
		if err := rows.Scan(
			&r.AgentID, &r.ProjectID, &r.ProjectName, &r.ExecutionCount,
			&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens,
			&r.ReasoningTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan agent usage: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UsageByProfile aggregates token usage per profile.
func (s *Store) UsageByProfile(ctx context.Context, filter core.AnalyticsFilter) ([]core.ProfileUsageSummary, error) {
	query := `
		SELECT
			u.profile_id,
			u.agent_id,
			u.project_id,
			COALESCE(p.name, ''),
			COUNT(*) AS exec_count,
			SUM(u.input_tokens),
			SUM(u.output_tokens),
			SUM(u.cache_read_tokens),
			SUM(u.cache_write_tokens),
			SUM(u.reasoning_tokens),
			SUM(u.total_tokens)
		FROM usage_records u
		LEFT JOIN projects p ON p.id = u.project_id`

	conditions, args := usageFilterConditions(filter)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " GROUP BY u.profile_id, u.agent_id, u.project_id ORDER BY SUM(u.total_tokens) DESC"
	query += limitClause(filter.Limit, 50)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage by profile: %w", err)
	}
	defer rows.Close()

	var out []core.ProfileUsageSummary
	for rows.Next() {
		var r core.ProfileUsageSummary
		if err := rows.Scan(
			&r.ProfileID, &r.AgentID, &r.ProjectID, &r.ProjectName, &r.ExecutionCount,
			&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens,
			&r.ReasoningTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan profile usage: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UsageTotals returns overall token usage totals.
func (s *Store) UsageTotals(ctx context.Context, filter core.AnalyticsFilter) (*core.UsageTotalSummary, error) {
	query := `
		SELECT
			COUNT(*),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read_tokens), 0),
			COALESCE(SUM(cache_write_tokens), 0),
			COALESCE(SUM(reasoning_tokens), 0),
			COALESCE(SUM(total_tokens), 0)
		FROM usage_records u`

	conditions, args := usageFilterConditions(filter)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	r := &core.UsageTotalSummary{}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&r.ExecutionCount,
		&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens,
		&r.ReasoningTokens, &r.TotalTokens,
	)
	if err != nil {
		return nil, fmt.Errorf("usage totals: %w", err)
	}
	return r, nil
}

func usageFilterConditions(filter core.AnalyticsFilter) ([]string, []any) {
	var conditions []string
	var args []any
	if filter.ProjectID != nil {
		conditions = append(conditions, "u.project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.Since != nil {
		conditions = append(conditions, "u.created_at >= ?")
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		conditions = append(conditions, "u.created_at < ?")
		args = append(args, *filter.Until)
	}
	return conditions, args
}
