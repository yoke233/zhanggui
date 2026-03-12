package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *Store) CreateExecution(ctx context.Context, e *core.Execution) (int64, error) {
	input, err := marshalJSON(e.Input)
	if err != nil {
		return 0, fmt.Errorf("marshal input: %w", err)
	}
	output, err := marshalJSON(e.Output)
	if err != nil {
		return 0, fmt.Errorf("marshal output: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO executions (step_id, issue_id, status, agent_id, agent_context_id,
		        briefing_snapshot, artifact_id, input, output, error_message, error_kind,
		        attempt, started_at, finished_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.StepID, e.IssueID, e.Status, e.AgentID, e.AgentContextID,
		nullStr(e.BriefingSnapshot), e.ArtifactID, input, output, e.ErrorMessage, nullStr(string(e.ErrorKind)),
		e.Attempt, e.StartedAt, e.FinishedAt, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert execution: %w", err)
	}
	id, _ := res.LastInsertId()
	e.ID = id
	e.CreatedAt = now
	return id, nil
}

func (s *Store) GetExecution(ctx context.Context, id int64) (*core.Execution, error) {
	e := &core.Execution{}
	var input, output, briefing, errorKind sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, step_id, issue_id, status, agent_id, agent_context_id,
		        briefing_snapshot, artifact_id, input, output,
		        error_message, error_kind, attempt, started_at, finished_at, created_at
		 FROM executions WHERE id = ?`, id,
	).Scan(&e.ID, &e.StepID, &e.IssueID, &e.Status, &e.AgentID, &e.AgentContextID,
		&briefing, &e.ArtifactID, &input, &output,
		&e.ErrorMessage, &errorKind, &e.Attempt, &e.StartedAt, &e.FinishedAt, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get execution %d: %w", id, err)
	}
	if briefing.Valid {
		e.BriefingSnapshot = briefing.String
	}
	if errorKind.Valid {
		e.ErrorKind = core.ErrorKind(errorKind.String)
	}
	unmarshalNullJSON(input, &e.Input)
	unmarshalNullJSON(output, &e.Output)
	return e, nil
}

func (s *Store) ListExecutionsByStep(ctx context.Context, stepID int64) ([]*core.Execution, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, step_id, issue_id, status, agent_id, agent_context_id,
		        briefing_snapshot, artifact_id, input, output,
		        error_message, error_kind, attempt, started_at, finished_at, created_at
		 FROM executions WHERE step_id = ? ORDER BY attempt`, stepID,
	)
	if err != nil {
		return nil, fmt.Errorf("list executions by step: %w", err)
	}
	defer rows.Close()

	var execs []*core.Execution
	for rows.Next() {
		e := &core.Execution{}
		var input, output, briefing, errorKind sql.NullString
		if err := rows.Scan(&e.ID, &e.StepID, &e.IssueID, &e.Status, &e.AgentID, &e.AgentContextID,
			&briefing, &e.ArtifactID, &input, &output,
			&e.ErrorMessage, &errorKind, &e.Attempt, &e.StartedAt, &e.FinishedAt, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		if briefing.Valid {
			e.BriefingSnapshot = briefing.String
		}
		if errorKind.Valid {
			e.ErrorKind = core.ErrorKind(errorKind.String)
		}
		unmarshalNullJSON(input, &e.Input)
		unmarshalNullJSON(output, &e.Output)
		execs = append(execs, e)
	}
	return execs, rows.Err()
}

func (s *Store) ListExecutionsByStatus(ctx context.Context, status core.ExecutionStatus) ([]*core.Execution, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, step_id, issue_id, status, agent_id, agent_context_id,
		        briefing_snapshot, artifact_id, input, output,
		        error_message, error_kind, attempt, started_at, finished_at, created_at
		 FROM executions WHERE status = ? ORDER BY id`, status,
	)
	if err != nil {
		return nil, fmt.Errorf("list executions by status: %w", err)
	}
	defer rows.Close()

	var execs []*core.Execution
	for rows.Next() {
		e := &core.Execution{}
		var input, output, briefing, errorKind sql.NullString
		if err := rows.Scan(&e.ID, &e.StepID, &e.IssueID, &e.Status, &e.AgentID, &e.AgentContextID,
			&briefing, &e.ArtifactID, &input, &output,
			&e.ErrorMessage, &errorKind, &e.Attempt, &e.StartedAt, &e.FinishedAt, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		if briefing.Valid {
			e.BriefingSnapshot = briefing.String
		}
		if errorKind.Valid {
			e.ErrorKind = core.ErrorKind(errorKind.String)
		}
		unmarshalNullJSON(input, &e.Input)
		unmarshalNullJSON(output, &e.Output)
		execs = append(execs, e)
	}
	return execs, rows.Err()
}

func (s *Store) UpdateExecution(ctx context.Context, e *core.Execution) error {
	output, err := marshalJSON(e.Output)
	if err != nil {
		return fmt.Errorf("marshal output: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE executions SET status = ?, agent_id = ?, agent_context_id = ?,
		        briefing_snapshot = ?, artifact_id = ?, output = ?,
		        error_message = ?, error_kind = ?, started_at = ?, finished_at = ?
		 WHERE id = ?`,
		e.Status, e.AgentID, e.AgentContextID,
		nullStr(e.BriefingSnapshot), e.ArtifactID, output,
		e.ErrorMessage, nullStr(string(e.ErrorKind)), e.StartedAt, e.FinishedAt,
		e.ID,
	)
	if err != nil {
		return fmt.Errorf("update execution: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

