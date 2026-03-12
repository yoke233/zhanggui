package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *Store) CreateStep(ctx context.Context, st *core.Step) (int64, error) {
	cfg, err := marshalJSON(st.Config)
	if err != nil {
		return 0, fmt.Errorf("marshal config: %w", err)
	}
	caps, err := marshalJSON(st.RequiredCapabilities)
	if err != nil {
		return 0, fmt.Errorf("marshal required_capabilities: %w", err)
	}
	criteria, err := marshalJSON(st.AcceptanceCriteria)
	if err != nil {
		return 0, fmt.Errorf("marshal acceptance_criteria: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO steps (issue_id, name, description, type, status, position, agent_role,
		        required_capabilities, acceptance_criteria, timeout_ms, config, max_retries, retry_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.IssueID, st.Name, st.Description, st.Type, st.Status, st.Position, st.AgentRole,
		caps, criteria, st.Timeout.Milliseconds(), cfg,
		st.MaxRetries, st.RetryCount, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert step: %w", err)
	}
	id, _ := res.LastInsertId()
	st.ID = id
	st.CreatedAt = now
	st.UpdatedAt = now
	return id, nil
}

func (s *Store) GetStep(ctx context.Context, id int64) (*core.Step, error) {
	st := &core.Step{}
	var cfg, caps, criteria sql.NullString
	var timeoutMs int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, issue_id, name, description, type, status, position, agent_role,
		        required_capabilities, acceptance_criteria, timeout_ms, config,
		        max_retries, retry_count, created_at, updated_at
		 FROM steps WHERE id = ?`, id,
	).Scan(&st.ID, &st.IssueID, &st.Name, &st.Description, &st.Type, &st.Status, &st.Position,
		&st.AgentRole, &caps, &criteria, &timeoutMs, &cfg,
		&st.MaxRetries, &st.RetryCount, &st.CreatedAt, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get step %d: %w", id, err)
	}
	unmarshalNullJSON(cfg, &st.Config)
	unmarshalNullJSON(caps, &st.RequiredCapabilities)
	unmarshalNullJSON(criteria, &st.AcceptanceCriteria)
	st.Timeout = time.Duration(timeoutMs) * time.Millisecond
	return st, nil
}

func (s *Store) ListStepsByIssue(ctx context.Context, issueID int64) ([]*core.Step, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, issue_id, name, description, type, status, position, agent_role,
		        required_capabilities, acceptance_criteria, timeout_ms, config,
		        max_retries, retry_count, created_at, updated_at
		 FROM steps WHERE issue_id = ? ORDER BY position, id`, issueID,
	)
	if err != nil {
		return nil, fmt.Errorf("list steps by issue: %w", err)
	}
	defer rows.Close()

	var steps []*core.Step
	for rows.Next() {
		st := &core.Step{}
		var cfg, caps, criteria sql.NullString
		var timeoutMs int64
		if err := rows.Scan(&st.ID, &st.IssueID, &st.Name, &st.Description, &st.Type, &st.Status, &st.Position,
			&st.AgentRole, &caps, &criteria, &timeoutMs, &cfg,
			&st.MaxRetries, &st.RetryCount, &st.CreatedAt, &st.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		unmarshalNullJSON(cfg, &st.Config)
		unmarshalNullJSON(caps, &st.RequiredCapabilities)
		unmarshalNullJSON(criteria, &st.AcceptanceCriteria)
		st.Timeout = time.Duration(timeoutMs) * time.Millisecond
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

func (s *Store) UpdateStepStatus(ctx context.Context, id int64, status core.StepStatus) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE steps SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update step status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateStep(ctx context.Context, st *core.Step) error {
	cfg, _ := marshalJSON(st.Config)
	caps, _ := marshalJSON(st.RequiredCapabilities)
	criteria, _ := marshalJSON(st.AcceptanceCriteria)
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE steps SET name = ?, description = ?, type = ?, status = ?, position = ?,
		        agent_role = ?, required_capabilities = ?, acceptance_criteria = ?,
		        timeout_ms = ?, config = ?, max_retries = ?, retry_count = ?, updated_at = ?
		 WHERE id = ?`,
		st.Name, st.Description, st.Type, st.Status, st.Position,
		st.AgentRole, caps, criteria,
		st.Timeout.Milliseconds(), cfg, st.MaxRetries, st.RetryCount, now,
		st.ID,
	)
	if err != nil {
		return fmt.Errorf("update step: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	st.UpdatedAt = now
	return nil
}

func (s *Store) DeleteStep(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM steps WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete step %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func unmarshalNullJSON(ns sql.NullString, dest any) {
	if ns.Valid {
		_ = json.Unmarshal([]byte(ns.String), dest)
	}
}
