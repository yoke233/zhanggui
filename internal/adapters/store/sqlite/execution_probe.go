package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *Store) CreateExecutionProbe(ctx context.Context, probe *core.ExecutionProbe) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO execution_probes (
			execution_id, issue_id, step_id, agent_context_id, session_id, owner_id,
			trigger_source, question, status, verdict, reply_text, error, sent_at, answered_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		probe.ExecutionID, probe.IssueID, probe.StepID, probe.AgentContextID, probe.SessionID, probe.OwnerID,
		probe.TriggerSource, probe.Question, probe.Status, probe.Verdict, probe.ReplyText, probe.Error, probe.SentAt, probe.AnsweredAt, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert execution probe: %w", err)
	}
	id, _ := res.LastInsertId()
	probe.ID = id
	probe.CreatedAt = now
	return id, nil
}

func (s *Store) GetExecutionProbe(ctx context.Context, id int64) (*core.ExecutionProbe, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, execution_id, issue_id, step_id, agent_context_id, session_id, owner_id,
		        trigger_source, question, status, verdict, reply_text, error, sent_at, answered_at, created_at
		 FROM execution_probes WHERE id = ?`, id,
	)
	return scanExecutionProbe(row)
}

func (s *Store) ListExecutionProbesByExecution(ctx context.Context, executionID int64) ([]*core.ExecutionProbe, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, execution_id, issue_id, step_id, agent_context_id, session_id, owner_id,
		        trigger_source, question, status, verdict, reply_text, error, sent_at, answered_at, created_at
		 FROM execution_probes WHERE execution_id = ? ORDER BY id`, executionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list execution probes: %w", err)
	}
	defer rows.Close()

	var probes []*core.ExecutionProbe
	for rows.Next() {
		probe, err := scanExecutionProbe(rows)
		if err != nil {
			return nil, err
		}
		probes = append(probes, probe)
	}
	return probes, rows.Err()
}

func (s *Store) GetLatestExecutionProbe(ctx context.Context, executionID int64) (*core.ExecutionProbe, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, execution_id, issue_id, step_id, agent_context_id, session_id, owner_id,
		        trigger_source, question, status, verdict, reply_text, error, sent_at, answered_at, created_at
		 FROM execution_probes WHERE execution_id = ? ORDER BY id DESC LIMIT 1`, executionID,
	)
	return scanExecutionProbe(row)
}

func (s *Store) GetActiveExecutionProbe(ctx context.Context, executionID int64) (*core.ExecutionProbe, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, execution_id, issue_id, step_id, agent_context_id, session_id, owner_id,
		        trigger_source, question, status, verdict, reply_text, error, sent_at, answered_at, created_at
		 FROM execution_probes
		 WHERE execution_id = ? AND status IN (?, ?)
		 ORDER BY id DESC LIMIT 1`,
		executionID, core.ExecutionProbePending, core.ExecutionProbeSent,
	)
	return scanExecutionProbe(row)
}

func (s *Store) UpdateExecutionProbe(ctx context.Context, probe *core.ExecutionProbe) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE execution_probes
		 SET agent_context_id = ?, session_id = ?, owner_id = ?, status = ?, verdict = ?, reply_text = ?, error = ?, sent_at = ?, answered_at = ?
		 WHERE id = ?`,
		probe.AgentContextID, probe.SessionID, probe.OwnerID, probe.Status, probe.Verdict, probe.ReplyText, probe.Error, probe.SentAt, probe.AnsweredAt, probe.ID,
	)
	if err != nil {
		return fmt.Errorf("update execution probe: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) GetExecutionProbeRoute(ctx context.Context, executionID int64) (*core.ExecutionProbeRoute, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT e.id, e.issue_id, e.step_id, e.agent_context_id,
		        COALESCE(ac.session_id, ''), COALESCE(ac.worker_id, ''), ac.worker_last_seen_at
		 FROM executions e
		 LEFT JOIN agent_contexts ac ON ac.id = e.agent_context_id
		 WHERE e.id = ?`,
		executionID,
	)

	route := &core.ExecutionProbeRoute{}
	var agentContextID sql.NullInt64
	var sessionID, ownerID sql.NullString
	var ownerLastSeen sql.NullTime
	if err := row.Scan(&route.ExecutionID, &route.IssueID, &route.StepID, &agentContextID, &sessionID, &ownerID, &ownerLastSeen); err != nil {
		if err == sql.ErrNoRows {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get execution probe route: %w", err)
	}
	if agentContextID.Valid {
		id := agentContextID.Int64
		route.AgentContextID = &id
	}
	if sessionID.Valid {
		route.SessionID = sessionID.String
	}
	if ownerID.Valid {
		route.OwnerID = ownerID.String
	}
	if ownerLastSeen.Valid {
		ts := ownerLastSeen.Time
		route.OwnerLastSeenAt = &ts
	}
	return route, nil
}

type executionProbeScanner interface {
	Scan(dest ...any) error
}

func scanExecutionProbe(scanner executionProbeScanner) (*core.ExecutionProbe, error) {
	probe := &core.ExecutionProbe{}
	var agentContextID sql.NullInt64
	var sessionID, ownerID, replyText, probeErr sql.NullString
	var sentAt, answeredAt sql.NullTime
	if err := scanner.Scan(
		&probe.ID, &probe.ExecutionID, &probe.IssueID, &probe.StepID, &agentContextID, &sessionID, &ownerID,
		&probe.TriggerSource, &probe.Question, &probe.Status, &probe.Verdict, &replyText, &probeErr, &sentAt, &answeredAt, &probe.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("scan execution probe: %w", err)
	}
	if agentContextID.Valid {
		id := agentContextID.Int64
		probe.AgentContextID = &id
	}
	if sessionID.Valid {
		probe.SessionID = sessionID.String
	}
	if ownerID.Valid {
		probe.OwnerID = ownerID.String
	}
	if replyText.Valid {
		probe.ReplyText = replyText.String
	}
	if probeErr.Valid {
		probe.Error = probeErr.String
	}
	if sentAt.Valid {
		ts := sentAt.Time
		probe.SentAt = &ts
	}
	if answeredAt.Valid {
		ts := answeredAt.Time
		probe.AnsweredAt = &ts
	}
	return probe, nil
}

