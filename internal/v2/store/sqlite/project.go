package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

func (s *Store) CreateProject(ctx context.Context, p *core.Project) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if p == nil {
		return 0, fmt.Errorf("project is nil")
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return 0, fmt.Errorf("name is required")
	}

	kind := p.Kind
	if kind == "" {
		kind = core.ProjectGeneral
	}

	var metadataJSON []byte
	if len(p.Metadata) > 0 {
		var err error
		metadataJSON, err = json.Marshal(p.Metadata)
		if err != nil {
			return 0, fmt.Errorf("marshal metadata: %w", err)
		}
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (name, kind, description, metadata, created_at, updated_at)
         VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		name, string(kind), p.Description, nullableBytes(metadataJSON))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetProject(ctx context.Context, id int64) (*core.Project, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, kind, description, metadata, created_at, updated_at
         FROM projects WHERE id = ?`, id)

	var p core.Project
	var metadataRaw sql.NullString
	if err := row.Scan(&p.ID, &p.Name, &p.Kind, &p.Description, &metadataRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	if metadataRaw.Valid && metadataRaw.String != "" {
		if err := json.Unmarshal([]byte(metadataRaw.String), &p.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return &p, nil
}

func (s *Store) ListProjects(ctx context.Context, limit, offset int) ([]*core.Project, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, kind, description, metadata, created_at, updated_at
         FROM projects
         ORDER BY id DESC
         LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*core.Project
	for rows.Next() {
		var p core.Project
		var metadataRaw sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.Description, &metadataRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if metadataRaw.Valid && metadataRaw.String != "" {
			if err := json.Unmarshal([]byte(metadataRaw.String), &p.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProject(ctx context.Context, p *core.Project) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not initialized")
	}
	if p == nil {
		return fmt.Errorf("project is nil")
	}

	var metadataJSON []byte
	if len(p.Metadata) > 0 {
		var err error
		metadataJSON, err = json.Marshal(p.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET name = ?, kind = ?, description = ?, metadata = ?, updated_at = CURRENT_TIMESTAMP
         WHERE id = ?`,
		p.Name, string(p.Kind), p.Description, nullableBytes(metadataJSON), p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not initialized")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

// nullableBytes returns nil if b is nil/empty, otherwise the byte slice itself.
// Used for nullable TEXT columns storing JSON.
func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
