package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

func (s *Store) CreateFlow(ctx context.Context, f *core.Flow) (int64, error) {
	meta, err := marshalJSON(f.Metadata)
	if err != nil {
		return 0, fmt.Errorf("marshal metadata: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO flows (project_id, name, status, parent_step_id, metadata, archived_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ProjectID, f.Name, f.Status, f.ParentStepID, meta, f.ArchivedAt, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert flow: %w", err)
	}
	id, _ := res.LastInsertId()
	f.ID = id
	f.CreatedAt = now
	f.UpdatedAt = now
	return id, nil
}

func (s *Store) GetFlow(ctx context.Context, id int64) (*core.Flow, error) {
	f := &core.Flow{}
	var meta sql.NullString
	var archivedAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, status, parent_step_id, metadata, archived_at, created_at, updated_at
		 FROM flows WHERE id = ?`, id,
	).Scan(&f.ID, &f.ProjectID, &f.Name, &f.Status, &f.ParentStepID, &meta, &archivedAt, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get flow %d: %w", id, err)
	}
	if meta.Valid {
		_ = json.Unmarshal([]byte(meta.String), &f.Metadata)
	}
	if archivedAt.Valid {
		t := archivedAt.Time
		f.ArchivedAt = &t
	}
	return f, nil
}

func (s *Store) ListFlows(ctx context.Context, filter core.FlowFilter) ([]*core.Flow, error) {
	query := `SELECT id, project_id, name, status, parent_step_id, metadata, archived_at, created_at, updated_at FROM flows`
	var args []any
	var conditions []string
	if filter.ProjectID != nil {
		conditions = append(conditions, `project_id = ?`)
		args = append(args, *filter.ProjectID)
	}
	if filter.Status != nil {
		conditions = append(conditions, `status = ?`)
		args = append(args, *filter.Status)
	}
	if filter.Archived != nil {
		if *filter.Archived {
			conditions = append(conditions, `archived_at IS NOT NULL`)
		} else {
			conditions = append(conditions, `archived_at IS NULL`)
		}
	} else if !filter.IncludeArchived {
		conditions = append(conditions, `archived_at IS NULL`)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY id DESC`
	if filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(` OFFSET %d`, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list flows: %w", err)
	}
	defer rows.Close()

	var flows []*core.Flow
	for rows.Next() {
		f := &core.Flow{}
		var meta sql.NullString
		var archivedAt sql.NullTime
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Name, &f.Status, &f.ParentStepID, &meta, &archivedAt, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan flow: %w", err)
		}
		if meta.Valid {
			_ = json.Unmarshal([]byte(meta.String), &f.Metadata)
		}
		if archivedAt.Valid {
			t := archivedAt.Time
			f.ArchivedAt = &t
		}
		flows = append(flows, f)
	}
	return flows, rows.Err()
}

func (s *Store) UpdateFlowStatus(ctx context.Context, id int64, status core.FlowStatus) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE flows SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update flow status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) SetFlowArchived(ctx context.Context, id int64, archived bool) error {
	now := time.Now().UTC()
	var archivedAt any
	if archived {
		archivedAt = now
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE flows SET archived_at = ?, updated_at = ? WHERE id = ?`,
		archivedAt, now, id,
	)
	if err != nil {
		return fmt.Errorf("set flow archived: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}
