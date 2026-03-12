package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateUsageRecord(ctx context.Context, r *core.UsageRecord) (int64, error) {
	model := usageRecordModelFromCore(r)
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert usage record: %w", err)
	}
	r.ID = model.ID
	r.CreatedAt = model.CreatedAt
	return model.ID, nil
}

func (s *Store) GetUsageRecord(ctx context.Context, id int64) (*core.UsageRecord, error) {
	var model UsageRecordModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("usage record %d not found", id)
		}
		return nil, fmt.Errorf("get usage record: %w", err)
	}
	return model.toCore(), nil
}

func (s *Store) GetUsageByExecution(ctx context.Context, executionID int64) (*core.UsageRecord, error) {
	var model UsageRecordModel
	err := s.orm.WithContext(ctx).Where("execution_id = ?", executionID).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("get usage by execution: %w", err)
	}
	return model.toCore(), nil
}

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
