package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

func (s *Store) CreateResourceBinding(ctx context.Context, rb *core.ResourceBinding) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if rb == nil {
		return 0, fmt.Errorf("resource binding is nil")
	}

	var configJSON []byte
	if len(rb.Config) > 0 {
		var err error
		configJSON, err = json.Marshal(rb.Config)
		if err != nil {
			return 0, fmt.Errorf("marshal config: %w", err)
		}
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO resource_bindings (project_id, kind, uri, config, label, created_at)
         VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rb.ProjectID, rb.Kind, rb.URI, nullableBytes(configJSON), rb.Label)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetResourceBinding(ctx context.Context, id int64) (*core.ResourceBinding, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, kind, uri, config, label, created_at
         FROM resource_bindings WHERE id = ?`, id)

	return scanResourceBinding(row)
}

func (s *Store) ListResourceBindings(ctx context.Context, projectID int64) ([]*core.ResourceBinding, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, kind, uri, config, label, created_at
         FROM resource_bindings WHERE project_id = ?
         ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*core.ResourceBinding
	for rows.Next() {
		var rb core.ResourceBinding
		var configRaw sql.NullString
		if err := rows.Scan(&rb.ID, &rb.ProjectID, &rb.Kind, &rb.URI, &configRaw, &rb.Label, &rb.CreatedAt); err != nil {
			return nil, err
		}
		if configRaw.Valid && configRaw.String != "" {
			if err := json.Unmarshal([]byte(configRaw.String), &rb.Config); err != nil {
				return nil, fmt.Errorf("unmarshal config: %w", err)
			}
		}
		out = append(out, &rb)
	}
	return out, rows.Err()
}

func (s *Store) DeleteResourceBinding(ctx context.Context, id int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not initialized")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM resource_bindings WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func scanResourceBinding(row *sql.Row) (*core.ResourceBinding, error) {
	var rb core.ResourceBinding
	var configRaw sql.NullString
	if err := row.Scan(&rb.ID, &rb.ProjectID, &rb.Kind, &rb.URI, &configRaw, &rb.Label, &rb.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	if configRaw.Valid && configRaw.String != "" {
		if err := json.Unmarshal([]byte(configRaw.String), &rb.Config); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}
	return &rb, nil
}
