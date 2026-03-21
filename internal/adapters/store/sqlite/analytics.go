package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/yoke233/zhanggui/internal/core"
)

// ProjectErrorRanking returns projects ordered by failure count.
func (s *Store) ProjectErrorRanking(ctx context.Context, filter core.AnalyticsFilter) ([]core.ProjectErrorRank, error) {
	query := `
		SELECT
			p.id,
			p.name,
			COUNT(DISTINCT i.id) AS total_work_items,
			COUNT(DISTINCT CASE WHEN i.status = 'failed' THEN i.id END) AS failed_work_items,
			CASE WHEN COUNT(DISTINCT i.id) > 0
				THEN CAST(COUNT(DISTINCT CASE WHEN i.status = 'failed' THEN i.id END) AS REAL) / COUNT(DISTINCT i.id)
				ELSE 0 END AS failure_rate,
			COUNT(DISTINCT CASE WHEN e.status = 'failed' THEN e.id END) AS failed_runs
		FROM projects p
		LEFT JOIN work_items i ON i.project_id = p.id
		LEFT JOIN actions st ON st.work_item_id = i.id
		LEFT JOIN runs e ON e.action_id = st.id`

	var conditions []string
	var args []any

	conditions, args = appendTimeConditions(conditions, args, "i.created_at", filter)

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += ` GROUP BY p.id ORDER BY failed_work_items DESC, failure_rate DESC`
	query += limitClause(filter.Limit, 20)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("project error ranking: %w", err)
	}
	defer rows.Close()

	var out []core.ProjectErrorRank
	for rows.Next() {
		var r core.ProjectErrorRank
		if err := rows.Scan(&r.ProjectID, &r.ProjectName, &r.TotalWorkItems,
			&r.FailedWorkItems, &r.FailureRate, &r.FailedRuns); err != nil {
			return nil, fmt.Errorf("scan project error rank: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// WorkItemBottleneckActions returns the slowest or most-failing actions across work items.
func (s *Store) WorkItemBottleneckActions(ctx context.Context, filter core.AnalyticsFilter) ([]core.ActionBottleneck, error) {
	query := `
		SELECT
			st.id,
			st.name,
			st.work_item_id,
			i.title,
			i.project_id,
			COALESCE(AVG(
				CASE WHEN e.started_at IS NOT NULL AND e.finished_at IS NOT NULL
					THEN (julianday(e.finished_at) - julianday(e.started_at)) * 86400
				END
			), 0) AS avg_dur,
			COALESCE(MAX(
				CASE WHEN e.started_at IS NOT NULL AND e.finished_at IS NOT NULL
					THEN (julianday(e.finished_at) - julianday(e.started_at)) * 86400
				END
			), 0) AS max_dur,
			COUNT(e.id) AS run_count,
			COUNT(CASE WHEN e.status = 'failed' THEN 1 END) AS fail_count,
			COALESCE(SUM(e.attempt - 1), 0) AS retry_count,
			CASE WHEN COUNT(e.id) > 0
				THEN CAST(COUNT(CASE WHEN e.status = 'failed' THEN 1 END) AS REAL) / COUNT(e.id)
				ELSE 0 END AS fail_rate
		FROM actions st
		JOIN work_items i ON i.id = st.work_item_id
		LEFT JOIN runs e ON e.action_id = st.id`

	var conditions []string
	var args []any

	if filter.ProjectID != nil {
		conditions = append(conditions, "i.project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	conditions, args = appendTimeConditions(conditions, args, "e.created_at", filter)

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += ` GROUP BY st.id
		HAVING run_count > 0
		ORDER BY avg_dur DESC, fail_rate DESC`
	query += limitClause(filter.Limit, 20)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("work item bottleneck actions: %w", err)
	}
	defer rows.Close()

	var out []core.ActionBottleneck
	for rows.Next() {
		var b core.ActionBottleneck
		if err := rows.Scan(&b.ActionID, &b.ActionName, &b.WorkItemID, &b.WorkItemTitle, &b.ProjectID,
			&b.AvgDurationS, &b.MaxDurationS, &b.RunCount, &b.FailCount,
			&b.RetryCount, &b.FailRate); err != nil {
			return nil, fmt.Errorf("scan action bottleneck: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// RunDurationStats returns per-work-item duration statistics.
func (s *Store) RunDurationStats(ctx context.Context, filter core.AnalyticsFilter) ([]core.WorkItemDurationStat, error) {
	query := `
		WITH exec_dur AS (
			SELECT
				st.work_item_id,
				(julianday(e.finished_at) - julianday(e.started_at)) * 86400 AS dur_s
			FROM runs e
			JOIN actions st ON st.id = e.action_id
			JOIN work_items i ON i.id = st.work_item_id
			WHERE e.started_at IS NOT NULL AND e.finished_at IS NOT NULL
				AND e.status IN ('succeeded', 'failed')`

	var args []any
	if filter.ProjectID != nil {
		query += " AND i.project_id = ?"
		args = append(args, *filter.ProjectID)
	}
	if filter.Since != nil {
		query += " AND e.created_at >= ?"
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		query += " AND e.created_at < ?"
		args = append(args, *filter.Until)
	}

	query += `
		)
		SELECT
			i.id,
			i.title,
			i.project_id,
			COUNT(*) AS exec_count,
			AVG(d.dur_s) AS avg_dur,
			MIN(d.dur_s) AS min_dur,
			MAX(d.dur_s) AS max_dur,
			0 AS p50_dur
		FROM exec_dur d
		JOIN work_items i ON i.id = d.work_item_id
		GROUP BY i.id
		ORDER BY avg_dur DESC`
	query += limitClause(filter.Limit, 20)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("run duration stats: %w", err)
	}
	defer rows.Close()

	var out []core.WorkItemDurationStat
	for rows.Next() {
		var d core.WorkItemDurationStat
		if err := rows.Scan(&d.WorkItemID, &d.WorkItemTitle, &d.ProjectID,
			&d.RunCount, &d.AvgDurationS, &d.MinDurationS, &d.MaxDurationS, &d.P50DurationS); err != nil {
			return nil, fmt.Errorf("scan duration stat: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ErrorBreakdown returns error counts grouped by error_kind.
func (s *Store) ErrorBreakdown(ctx context.Context, filter core.AnalyticsFilter) ([]core.ErrorKindCount, error) {
	query := `
		SELECT
			COALESCE(e.error_kind, 'unknown') AS ek,
			COUNT(*) AS cnt
		FROM runs e
		JOIN actions st ON st.id = e.action_id
		JOIN work_items i ON i.id = st.work_item_id
		WHERE e.status = 'failed'`

	var args []any
	if filter.ProjectID != nil {
		query += " AND i.project_id = ?"
		args = append(args, *filter.ProjectID)
	}
	if filter.Since != nil {
		query += " AND e.created_at >= ?"
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		query += " AND e.created_at < ?"
		args = append(args, *filter.Until)
	}

	query += ` GROUP BY ek ORDER BY cnt DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error breakdown: %w", err)
	}
	defer rows.Close()

	var out []core.ErrorKindCount
	var total int
	for rows.Next() {
		var c core.ErrorKindCount
		if err := rows.Scan(&c.ErrorKind, &c.Count); err != nil {
			return nil, fmt.Errorf("scan error kind: %w", err)
		}
		total += c.Count
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if total > 0 {
			out[i].Pct = float64(out[i].Count) / float64(total)
		}
	}
	return out, nil
}

// RecentFailures returns recent failed runs with full context.
func (s *Store) RecentFailures(ctx context.Context, filter core.AnalyticsFilter) ([]core.FailureRecord, error) {
	query := `
		SELECT
			e.id,
			e.action_id,
			st.name,
			st.work_item_id,
			i.title,
			i.project_id,
			COALESCE(p.name, ''),
			COALESCE(e.error_message, ''),
			COALESCE(e.error_kind, ''),
			e.attempt,
			CASE WHEN e.started_at IS NOT NULL AND e.finished_at IS NOT NULL
				THEN (julianday(e.finished_at) - julianday(e.started_at)) * 86400
				ELSE 0 END AS dur_s,
			COALESCE(e.finished_at, e.created_at) AS failed_at
		FROM runs e
		JOIN actions st ON st.id = e.action_id
		JOIN work_items i ON i.id = st.work_item_id
		LEFT JOIN projects p ON p.id = i.project_id
		WHERE e.status = 'failed'`

	var args []any
	if filter.ProjectID != nil {
		query += " AND i.project_id = ?"
		args = append(args, *filter.ProjectID)
	}
	if filter.Since != nil {
		query += " AND e.created_at >= ?"
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		query += " AND e.created_at < ?"
		args = append(args, *filter.Until)
	}

	query += ` ORDER BY failed_at DESC`
	query += limitClause(filter.Limit, 30)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("recent failures: %w", err)
	}
	defer rows.Close()

	var out []core.FailureRecord
	for rows.Next() {
		var r core.FailureRecord
		var ek sql.NullString
		if err := rows.Scan(&r.RunID, &r.ActionID, &r.ActionName, &r.WorkItemID, &r.WorkItemTitle,
			&r.ProjectID, &r.ProjectName, &r.ErrorMessage, &ek,
			&r.Attempt, &r.DurationS, &r.FailedAt); err != nil {
			return nil, fmt.Errorf("scan failure record: %w", err)
		}
		if ek.Valid {
			r.ErrorKind = core.ErrorKind(ek.String)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// WorkItemStatusDistribution returns work item counts grouped by status.
func (s *Store) WorkItemStatusDistribution(ctx context.Context, filter core.AnalyticsFilter) ([]core.StatusCount, error) {
	query := `
		SELECT status, COUNT(*) AS cnt
		FROM work_items
		WHERE 1=1`

	var args []any
	if filter.ProjectID != nil {
		query += " AND project_id = ?"
		args = append(args, *filter.ProjectID)
	}
	if filter.Since != nil {
		query += " AND created_at >= ?"
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		query += " AND created_at < ?"
		args = append(args, *filter.Until)
	}

	query += ` GROUP BY status ORDER BY cnt DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("work item status distribution: %w", err)
	}
	defer rows.Close()

	var out []core.StatusCount
	for rows.Next() {
		var c core.StatusCount
		if err := rows.Scan(&c.Status, &c.Count); err != nil {
			return nil, fmt.Errorf("scan status count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// helpers

func appendTimeConditions(conditions []string, args []any, col string, filter core.AnalyticsFilter) ([]string, []any) {
	if filter.Since != nil {
		conditions = append(conditions, col+" >= ?")
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		conditions = append(conditions, col+" < ?")
		args = append(args, *filter.Until)
	}
	return conditions, args
}

func limitClause(requested, defaultLimit int) string {
	n := defaultLimit
	if requested > 0 {
		n = requested
	}
	return fmt.Sprintf(" LIMIT %d", n)
}
