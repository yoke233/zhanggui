package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *Store) CreateDAGTemplate(ctx context.Context, t *core.DAGTemplate) (int64, error) {
	tags, err := marshalJSON(t.Tags)
	if err != nil {
		return 0, fmt.Errorf("marshal tags: %w", err)
	}
	meta, err := marshalJSON(t.Metadata)
	if err != nil {
		return 0, fmt.Errorf("marshal metadata: %w", err)
	}
	stepsJSON, err := json.Marshal(t.Steps)
	if err != nil {
		return 0, fmt.Errorf("marshal steps: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO dag_templates (name, description, project_id, tags, metadata, steps, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Description, t.ProjectID, tags, meta, string(stepsJSON), now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert dag_template: %w", err)
	}
	id, _ := res.LastInsertId()
	t.ID = id
	t.CreatedAt = now
	t.UpdatedAt = now
	return id, nil
}

func (s *Store) GetDAGTemplate(ctx context.Context, id int64) (*core.DAGTemplate, error) {
	t := &core.DAGTemplate{}
	var tags, meta, stepsJSON sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, project_id, tags, metadata, steps, created_at, updated_at
		 FROM dag_templates WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.ProjectID, &tags, &meta, &stepsJSON, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get dag_template %d: %w", id, err)
	}
	unmarshalNullJSON(tags, &t.Tags)
	unmarshalNullJSON(meta, &t.Metadata)
	if stepsJSON.Valid {
		_ = json.Unmarshal([]byte(stepsJSON.String), &t.Steps)
	}
	if t.Steps == nil {
		t.Steps = []core.DAGTemplateStep{}
	}
	return t, nil
}

func (s *Store) ListDAGTemplates(ctx context.Context, filter core.DAGTemplateFilter) ([]*core.DAGTemplate, error) {
	query := `SELECT id, name, description, project_id, tags, metadata, steps, created_at, updated_at FROM dag_templates`
	var args []any
	var conditions []string

	if filter.ProjectID != nil {
		conditions = append(conditions, `project_id = ?`)
		args = append(args, *filter.ProjectID)
	}
	if filter.Tag != "" {
		conditions = append(conditions, `tags LIKE ?`)
		args = append(args, `%"`+filter.Tag+`"%`)
	}
	if filter.Search != "" {
		conditions = append(conditions, `(name LIKE ? OR description LIKE ?)`)
		pattern := "%" + filter.Search + "%"
		args = append(args, pattern, pattern)
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
		return nil, fmt.Errorf("list dag_templates: %w", err)
	}
	defer rows.Close()

	var templates []*core.DAGTemplate
	for rows.Next() {
		t := &core.DAGTemplate{}
		var tags, meta, stepsJSON sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.ProjectID, &tags, &meta, &stepsJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan dag_template: %w", err)
		}
		unmarshalNullJSON(tags, &t.Tags)
		unmarshalNullJSON(meta, &t.Metadata)
		if stepsJSON.Valid {
			_ = json.Unmarshal([]byte(stepsJSON.String), &t.Steps)
		}
		if t.Steps == nil {
			t.Steps = []core.DAGTemplateStep{}
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

func (s *Store) UpdateDAGTemplate(ctx context.Context, t *core.DAGTemplate) error {
	tags, _ := marshalJSON(t.Tags)
	meta, _ := marshalJSON(t.Metadata)
	stepsJSON, err := json.Marshal(t.Steps)
	if err != nil {
		return fmt.Errorf("marshal steps: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE dag_templates SET name = ?, description = ?, project_id = ?, tags = ?, metadata = ?, steps = ?, updated_at = ?
		 WHERE id = ?`,
		t.Name, t.Description, t.ProjectID, tags, meta, string(stepsJSON), now, t.ID,
	)
	if err != nil {
		return fmt.Errorf("update dag_template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	t.UpdatedAt = now
	return nil
}

func (s *Store) DeleteDAGTemplate(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM dag_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete dag_template %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}
