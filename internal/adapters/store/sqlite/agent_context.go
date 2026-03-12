package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *Store) CreateAgentContext(ctx context.Context, ac *core.AgentContext) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_contexts (agent_id, issue_id, system_prompt, session_id, summary, turn_count, worker_id, worker_last_seen_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ac.AgentID, ac.IssueID, ac.SystemPrompt, ac.SessionID, ac.Summary, ac.TurnCount, ac.WorkerID, ac.WorkerLastSeenAt, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert agent_context: %w", err)
	}
	id, _ := res.LastInsertId()
	ac.ID = id
	ac.CreatedAt = now
	ac.UpdatedAt = now
	return id, nil
}

func (s *Store) GetAgentContext(ctx context.Context, id int64) (*core.AgentContext, error) {
	ac := &core.AgentContext{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, issue_id, system_prompt, session_id, summary, turn_count, worker_id, worker_last_seen_at, created_at, updated_at
		 FROM agent_contexts WHERE id = ?`, id,
	).Scan(&ac.ID, &ac.AgentID, &ac.IssueID, &ac.SystemPrompt, &ac.SessionID,
		&ac.Summary, &ac.TurnCount, &ac.WorkerID, &ac.WorkerLastSeenAt, &ac.CreatedAt, &ac.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get agent_context %d: %w", id, err)
	}
	return ac, nil
}

func (s *Store) FindAgentContext(ctx context.Context, agentID string, issueID int64) (*core.AgentContext, error) {
	ac := &core.AgentContext{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, issue_id, system_prompt, session_id, summary, turn_count, worker_id, worker_last_seen_at, created_at, updated_at
		 FROM agent_contexts WHERE agent_id = ? AND issue_id = ?`, agentID, issueID,
	).Scan(&ac.ID, &ac.AgentID, &ac.IssueID, &ac.SystemPrompt, &ac.SessionID,
		&ac.Summary, &ac.TurnCount, &ac.WorkerID, &ac.WorkerLastSeenAt, &ac.CreatedAt, &ac.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find agent_context: %w", err)
	}
	return ac, nil
}

func (s *Store) UpdateAgentContext(ctx context.Context, ac *core.AgentContext) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE agent_contexts SET system_prompt = ?, session_id = ?, summary = ?, turn_count = ?, worker_id = ?, worker_last_seen_at = ?, updated_at = ?
		 WHERE id = ?`,
		ac.SystemPrompt, ac.SessionID, ac.Summary, ac.TurnCount, ac.WorkerID, ac.WorkerLastSeenAt, now, ac.ID,
	)
	if err != nil {
		return fmt.Errorf("update agent_context: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	ac.UpdatedAt = now
	return nil
}

