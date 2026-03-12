package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *Store) CreateArtifact(ctx context.Context, a *core.Artifact) (int64, error) {
	meta, err := marshalJSON(a.Metadata)
	if err != nil {
		return 0, fmt.Errorf("marshal metadata: %w", err)
	}
	assets, err := marshalJSON(a.Assets)
	if err != nil {
		return 0, fmt.Errorf("marshal assets: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO artifacts (execution_id, step_id, issue_id, result_markdown, metadata, assets, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ExecutionID, a.StepID, a.IssueID, a.ResultMarkdown, meta, assets, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert artifact: %w", err)
	}
	id, _ := res.LastInsertId()
	a.ID = id
	a.CreatedAt = now
	return id, nil
}

func (s *Store) GetArtifact(ctx context.Context, id int64) (*core.Artifact, error) {
	a := &core.Artifact{}
	var meta, assets sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, execution_id, step_id, issue_id, result_markdown, metadata, assets, created_at
		 FROM artifacts WHERE id = ?`, id,
	).Scan(&a.ID, &a.ExecutionID, &a.StepID, &a.IssueID, &a.ResultMarkdown, &meta, &assets, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get artifact %d: %w", id, err)
	}
	unmarshalNullJSON(meta, &a.Metadata)
	if assets.Valid {
		_ = json.Unmarshal([]byte(assets.String), &a.Assets)
	}
	return a, nil
}

func (s *Store) GetLatestArtifactByStep(ctx context.Context, stepID int64) (*core.Artifact, error) {
	a := &core.Artifact{}
	var meta, assets sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, execution_id, step_id, issue_id, result_markdown, metadata, assets, created_at
		 FROM artifacts WHERE step_id = ? ORDER BY id DESC LIMIT 1`, stepID,
	).Scan(&a.ID, &a.ExecutionID, &a.StepID, &a.IssueID, &a.ResultMarkdown, &meta, &assets, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest artifact for step %d: %w", stepID, err)
	}
	unmarshalNullJSON(meta, &a.Metadata)
	if assets.Valid {
		_ = json.Unmarshal([]byte(assets.String), &a.Assets)
	}
	return a, nil
}

func (s *Store) UpdateArtifact(ctx context.Context, a *core.Artifact) error {
	meta, err := marshalJSON(a.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	assets, err := marshalJSON(a.Assets)
	if err != nil {
		return fmt.Errorf("marshal assets: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE artifacts SET result_markdown = ?, metadata = ?, assets = ? WHERE id = ?`,
		a.ResultMarkdown, meta, assets, a.ID,
	)
	if err != nil {
		return fmt.Errorf("update artifact %d: %w", a.ID, err)
	}
	return nil
}

func (s *Store) ListArtifactsByExecution(ctx context.Context, execID int64) ([]*core.Artifact, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, execution_id, step_id, issue_id, result_markdown, metadata, assets, created_at
		 FROM artifacts WHERE execution_id = ? ORDER BY id`, execID,
	)
	if err != nil {
		return nil, fmt.Errorf("list artifacts by execution: %w", err)
	}
	defer rows.Close()

	var artifacts []*core.Artifact
	for rows.Next() {
		a := &core.Artifact{}
		var meta, assets sql.NullString
		if err := rows.Scan(&a.ID, &a.ExecutionID, &a.StepID, &a.IssueID, &a.ResultMarkdown, &meta, &assets, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan artifact: %w", err)
		}
		unmarshalNullJSON(meta, &a.Metadata)
		if assets.Valid {
			_ = json.Unmarshal([]byte(assets.String), &a.Assets)
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}

