package storesqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

var errIssueNotFound = errors.New("issue not found")

func New(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// SQLite does not support concurrent writers; limit to a single connection
	// so that PRAGMA settings (WAL, busy_timeout) apply consistently.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := applyMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) CreateProject(p *core.Project) error {
	_, err := s.db.Exec(
		`INSERT INTO projects (id, name, repo_path, github_owner, github_repo, default_branch) VALUES (?,?,?,?,?,?)`,
		p.ID, p.Name, p.RepoPath, p.GitHubOwner, p.GitHubRepo, p.DefaultBranch,
	)
	return err
}

func (s *SQLiteStore) GetProject(id string) (*core.Project, error) {
	p := &core.Project{}
	err := s.db.QueryRow(
		`SELECT id, name, repo_path, github_owner, github_repo, COALESCE(default_branch, ''), created_at, updated_at FROM projects WHERE id=?`,
		id,
	).Scan(&p.ID, &p.Name, &p.RepoPath, &p.GitHubOwner, &p.GitHubRepo, &p.DefaultBranch, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("project %s not found", id)
	}
	return p, err
}

func (s *SQLiteStore) UpdateProject(p *core.Project) error {
	_, err := s.db.Exec(
		`UPDATE projects SET name=?, repo_path=?, github_owner=?, github_repo=?, default_branch=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		p.Name, p.RepoPath, p.GitHubOwner, p.GitHubRepo, p.DefaultBranch, p.ID,
	)
	return err
}

func (s *SQLiteStore) DeleteProject(id string) error {
	// Child rows are cleaned by ON DELETE CASCADE via runs.project_id foreign key.
	_, err := s.db.Exec(`DELETE FROM projects WHERE id=?`, id)
	return err
}

func (s *SQLiteStore) ListProjects(filter core.ProjectFilter) ([]core.Project, error) {
	query := `SELECT id, name, repo_path, github_owner, github_repo, COALESCE(default_branch, ''), created_at, updated_at FROM projects`
	args := []any{}
	if filter.NameContains != "" {
		query += ` WHERE lower(name) LIKE ?`
		args = append(args, "%"+strings.ToLower(filter.NameContains)+"%")
	}
	query += ` ORDER BY name`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.Project
	for rows.Next() {
		var p core.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.RepoPath, &p.GitHubOwner, &p.GitHubRepo, &p.DefaultBranch, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveRun(p *core.Run) error {
	if p.Artifacts == nil {
		p.Artifacts = map[string]string{}
	}
	if p.Config == nil {
		p.Config = map[string]any{}
	}
	stagesJSON, err := json.Marshal(p.Stages)
	if err != nil {
		return err
	}
	artifactsJSON, err := json.Marshal(p.Artifacts)
	if err != nil {
		return err
	}
	configJSON, err := json.Marshal(p.Config)
	if err != nil {
		return err
	}
	query := `
INSERT INTO runs (
	id, project_id, name, description, template, status, conclusion, current_stage,
	stages_json, artifacts_json, config_json, branch_name, worktree_path,
	error_message, max_total_retries, total_retries, run_count, last_error_type, issue_id,
	queued_at, last_heartbeat_at, started_at, finished_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
	project_id=excluded.project_id,
	name=excluded.name,
	description=excluded.description,
	template=excluded.template,
	status=excluded.status,
	conclusion=excluded.conclusion,
	current_stage=excluded.current_stage,
	stages_json=excluded.stages_json,
	artifacts_json=excluded.artifacts_json,
	config_json=excluded.config_json,
	branch_name=excluded.branch_name,
	worktree_path=excluded.worktree_path,
	error_message=excluded.error_message,
	max_total_retries=excluded.max_total_retries,
	total_retries=excluded.total_retries,
	run_count=excluded.run_count,
	last_error_type=excluded.last_error_type,
	issue_id=excluded.issue_id,
	queued_at=excluded.queued_at,
	last_heartbeat_at=excluded.last_heartbeat_at,
	started_at=excluded.started_at,
	finished_at=excluded.finished_at,
	updated_at=CURRENT_TIMESTAMP`
	_, err = s.db.Exec(query,
		p.ID, p.ProjectID, p.Name, p.Description, p.Template, p.Status, p.Conclusion, p.CurrentStage,
		string(stagesJSON), string(artifactsJSON), string(configJSON), p.BranchName, p.WorktreePath,
		p.ErrorMessage, p.MaxTotalRetries, p.TotalRetries, p.RunCount, p.LastErrorType, nullableString(p.IssueID),
		nullableTime(p.QueuedAt), nullableTime(p.LastHeartbeatAt), nullableTime(p.StartedAt), nullableTime(p.FinishedAt),
	)
	return err
}

func (s *SQLiteStore) GetRun(id string) (*core.Run, error) {
	p := &core.Run{}
	var (
		stagesJSON    string
		artifactsJSON string
		configJSON    string
		queuedAt      sql.NullTime
		lastHeartbeat sql.NullTime
		startedAt     sql.NullTime
		finishedAt    sql.NullTime
	)
	query := `
SELECT id, project_id, name, description, template, status, COALESCE(conclusion, ''), current_stage,
       stages_json, artifacts_json, config_json, branch_name, worktree_path, error_message,
       max_total_retries, total_retries, run_count, last_error_type, COALESCE(issue_id, ''), queued_at, last_heartbeat_at,
	   started_at, finished_at, created_at, updated_at
FROM runs WHERE id=?`
	err := s.db.QueryRow(query,
		id,
	).Scan(
		&p.ID, &p.ProjectID, &p.Name, &p.Description, &p.Template, &p.Status, &p.Conclusion, &p.CurrentStage,
		&stagesJSON, &artifactsJSON, &configJSON, &p.BranchName, &p.WorktreePath, &p.ErrorMessage,
		&p.MaxTotalRetries, &p.TotalRetries, &p.RunCount, &p.LastErrorType, &p.IssueID, &queuedAt, &lastHeartbeat,
		&startedAt, &finishedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("run %s not found", id)
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(stagesJSON), &p.Stages); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(artifactsJSON), &p.Artifacts); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(configJSON), &p.Config); err != nil {
		return nil, err
	}
	if startedAt.Valid {
		p.StartedAt = startedAt.Time
	}
	if queuedAt.Valid {
		p.QueuedAt = queuedAt.Time
	}
	if lastHeartbeat.Valid {
		p.LastHeartbeatAt = lastHeartbeat.Time
	}
	if finishedAt.Valid {
		p.FinishedAt = finishedAt.Time
	}
	return p, nil
}

func (s *SQLiteStore) ListRuns(projectID string, filter core.RunFilter) ([]core.Run, error) {
	query := `SELECT id, project_id, name, template, status, COALESCE(conclusion, ''), current_stage, COALESCE(error_message, ''), COALESCE(issue_id, ''), created_at FROM runs WHERE project_id=?`
	args := []any{projectID}
	if filter.Status != "" {
		query += ` AND status=?`
		args = append(args, filter.Status)
	}
	if filter.Conclusion != "" {
		query += ` AND conclusion=?`
		args = append(args, filter.Conclusion)
	}
	if filter.IssueID != "" {
		query += ` AND issue_id=?`
		args = append(args, filter.IssueID)
	}
	query += ` ORDER BY created_at DESC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.Run
	for rows.Next() {
		var p core.Run
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Template, &p.Status, &p.Conclusion, &p.CurrentStage, &p.ErrorMessage, &p.IssueID, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetActiveRuns() ([]core.Run, error) {
	rows, err := s.db.Query(`SELECT id FROM runs WHERE status IN ('in_progress','action_required')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]core.Run, 0, len(ids))
	for _, id := range ids {
		p, err := s.GetRun(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

func (s *SQLiteStore) ListRunnableRuns(limit int) ([]core.Run, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(`
SELECT id
FROM runs
WHERE status = ?
ORDER BY COALESCE(queued_at, created_at) ASC, created_at ASC
LIMIT ?`,
		core.StatusQueued, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]core.Run, 0, len(ids))
	for _, id := range ids {
		p, err := s.GetRun(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

func (s *SQLiteStore) CountInProgressRunsByProject(projectID string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM runs WHERE project_id=? AND status=?`,
		projectID, core.StatusInProgress,
	).Scan(&count)
	return count, err
}

func (s *SQLiteStore) TryMarkRunInProgress(id string, from ...core.RunStatus) (bool, error) {
	if len(from) == 0 {
		from = []core.RunStatus{core.StatusQueued}
	}

	placeholders := make([]string, len(from))
	args := make([]any, 0, len(from)+2)
	args = append(args, core.StatusInProgress, id)
	for i, status := range from {
		placeholders[i] = "?"
		args = append(args, status)
	}

	query := fmt.Sprintf(`
UPDATE runs
SET status=?, run_count=run_count+1, started_at=COALESCE(started_at, CURRENT_TIMESTAMP), updated_at=CURRENT_TIMESTAMP
WHERE id=? AND status IN (%s)`, strings.Join(placeholders, ","))

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

func (s *SQLiteStore) SaveCheckpoint(cp *core.Checkpoint) error {
	if cp.Artifacts == nil {
		cp.Artifacts = map[string]string{}
	}
	artifactsJSON, err := json.Marshal(cp.Artifacts)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO checkpoints (run_id, stage, status, agent_used, agent_session_id, artifacts_json, tokens_used, retry_count, error_message, started_at, finished_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		cp.RunID, cp.StageName, cp.Status, cp.AgentUsed, cp.AgentSessionID, string(artifactsJSON), cp.TokensUsed, cp.RetryCount, cp.Error, cp.StartedAt, nullableTime(cp.FinishedAt),
	)
	return err
}

func (s *SQLiteStore) GetCheckpoints(RunID string) ([]core.Checkpoint, error) {
	rows, err := s.db.Query(
		`SELECT run_id, stage, status, agent_used, agent_session_id, artifacts_json, tokens_used, retry_count, error_message, started_at, finished_at FROM checkpoints WHERE run_id=? ORDER BY id`,
		RunID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.Checkpoint
	for rows.Next() {
		var (
			cp            core.Checkpoint
			artifactsJSON string
			finishedAt    sql.NullTime
		)
		if err := rows.Scan(&cp.RunID, &cp.StageName, &cp.Status, &cp.AgentUsed, &cp.AgentSessionID, &artifactsJSON, &cp.TokensUsed, &cp.RetryCount, &cp.Error, &cp.StartedAt, &finishedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(artifactsJSON), &cp.Artifacts); err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			cp.FinishedAt = finishedAt.Time
		}
		out = append(out, cp)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetLastSuccessCheckpoint(RunID string) (*core.Checkpoint, error) {
	var (
		cp            core.Checkpoint
		artifactsJSON string
		finishedAt    sql.NullTime
	)
	err := s.db.QueryRow(
		`SELECT run_id, stage, status, agent_used, agent_session_id, artifacts_json, started_at, finished_at
		 FROM checkpoints WHERE run_id=? AND status='success' ORDER BY id DESC LIMIT 1`,
		RunID,
	).Scan(&cp.RunID, &cp.StageName, &cp.Status, &cp.AgentUsed, &cp.AgentSessionID, &artifactsJSON, &cp.StartedAt, &finishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(artifactsJSON), &cp.Artifacts); err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		cp.FinishedAt = finishedAt.Time
	}
	return &cp, nil
}

func (s *SQLiteStore) InvalidateCheckpointsFromStage(RunID string, stage core.StageID) error {
	_, err := s.db.Exec(`
UPDATE checkpoints
SET status=?
WHERE run_id=? AND id >= (
	SELECT MIN(id)
	FROM checkpoints
	WHERE run_id=? AND stage=?
)`, core.CheckpointInvalidated, RunID, RunID, stage)
	return err
}

func (s *SQLiteStore) RecordAction(a core.HumanAction) error {
	_, err := s.db.Exec(
		`INSERT INTO human_actions (run_id, stage, action, message, source, user_id) VALUES (?,?,?,?,?,?)`,
		a.RunID, a.Stage, a.Action, a.Message, a.Source, a.UserID,
	)
	return err
}

func (s *SQLiteStore) GetActions(RunID string) ([]core.HumanAction, error) {
	rows, err := s.db.Query(
		`SELECT id, run_id, stage, action, message, source, user_id, created_at
		 FROM human_actions WHERE run_id=? ORDER BY id`,
		RunID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.HumanAction
	for rows.Next() {
		var a core.HumanAction
		if err := rows.Scan(&a.ID, &a.RunID, &a.Stage, &a.Action, &a.Message, &a.Source, &a.UserID, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) CreateChatSession(session *core.ChatSession) error {
	messagesJSON, err := marshalJSON(session.Messages)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO chat_sessions (id, project_id, agent_session_id, agent_name, messages) VALUES (?,?,?,?,?)`,
		session.ID, session.ProjectID, session.AgentSessionID, session.AgentName, messagesJSON,
	)
	return err
}

func (s *SQLiteStore) GetChatSession(id string) (*core.ChatSession, error) {
	session := &core.ChatSession{}
	var messagesJSON string
	err := s.db.QueryRow(
		`SELECT id, project_id, COALESCE(agent_session_id, ''), COALESCE(agent_name, ''), messages, created_at, updated_at FROM chat_sessions WHERE id=?`,
		id,
	).Scan(&session.ID, &session.ProjectID, &session.AgentSessionID, &session.AgentName, &messagesJSON, &session.CreatedAt, &session.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("chat session %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalJSON(messagesJSON, &session.Messages); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *SQLiteStore) UpdateChatSession(session *core.ChatSession) error {
	messagesJSON, err := marshalJSON(session.Messages)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(
		`UPDATE chat_sessions SET messages=?, agent_session_id=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		messagesJSON, session.AgentSessionID, session.ID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("chat session %s not found", session.ID)
	}
	return nil
}

func (s *SQLiteStore) ListChatSessions(projectID string) ([]core.ChatSession, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, COALESCE(agent_session_id, ''), COALESCE(agent_name, ''), messages, created_at, updated_at
		 FROM chat_sessions
		 WHERE project_id=?
		 ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.ChatSession
	for rows.Next() {
		var (
			session      core.ChatSession
			messagesJSON string
		)
		if err := rows.Scan(&session.ID, &session.ProjectID, &session.AgentSessionID, &session.AgentName, &messagesJSON, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(messagesJSON, &session.Messages); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) AppendChatRunEvent(event core.ChatRunEvent) error {
	trimmedSessionID := strings.TrimSpace(event.SessionID)
	if trimmedSessionID == "" {
		return errors.New("chat run event session_id is required")
	}
	trimmedProjectID := strings.TrimSpace(event.ProjectID)
	if trimmedProjectID == "" {
		return errors.New("chat run event project_id is required")
	}
	trimmedEventType := strings.TrimSpace(event.EventType)
	if trimmedEventType == "" {
		return errors.New("chat run event event_type is required")
	}

	payload := event.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, err := marshalJSON(payload)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		`INSERT INTO chat_run_events (
			chat_session_id, project_id, event_type, update_type, payload_json, created_at
		) VALUES (?,?,?,?,?,COALESCE(?, CURRENT_TIMESTAMP))`,
		trimmedSessionID,
		trimmedProjectID,
		trimmedEventType,
		strings.TrimSpace(event.UpdateType),
		payloadJSON,
		nullableTime(event.CreatedAt),
	)
	return err
}

func (s *SQLiteStore) ListChatRunEvents(sessionID string) ([]core.ChatRunEvent, error) {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return nil, errors.New("chat run event session_id is required")
	}

	rows, err := s.db.Query(
		`SELECT id, chat_session_id, project_id, event_type, update_type, payload_json, created_at
		 FROM chat_run_events
		 WHERE chat_session_id=?
		 ORDER BY id ASC`,
		trimmedSessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]core.ChatRunEvent, 0)
	for rows.Next() {
		var (
			event       core.ChatRunEvent
			payloadJSON string
		)
		if err := rows.Scan(
			&event.ID,
			&event.SessionID,
			&event.ProjectID,
			&event.EventType,
			&event.UpdateType,
			&payloadJSON,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := unmarshalJSONObject(payloadJSON, &event.Payload); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveRunEvent(event core.RunEvent) error {
	runID := strings.TrimSpace(event.RunID)
	if runID == "" {
		return errors.New("run event run_id is required")
	}
	eventType := strings.TrimSpace(event.EventType)
	if eventType == "" {
		return errors.New("run event event_type is required")
	}

	data := event.Data
	if data == nil {
		data = map[string]string{}
	}
	dataJSON, err := marshalJSON(data)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		`INSERT INTO run_events (
			run_id, project_id, issue_id, event_type, stage, agent, data_json, error, created_at
		) VALUES (?,?,?,?,?,?,?,?,COALESCE(?, CURRENT_TIMESTAMP))`,
		runID,
		strings.TrimSpace(event.ProjectID),
		strings.TrimSpace(event.IssueID),
		eventType,
		strings.TrimSpace(event.Stage),
		strings.TrimSpace(event.Agent),
		dataJSON,
		strings.TrimSpace(event.Error),
		nullableTime(event.CreatedAt),
	)
	return err
}

func (s *SQLiteStore) ListRunEvents(runID string) ([]core.RunEvent, error) {
	trimmed := strings.TrimSpace(runID)
	if trimmed == "" {
		return nil, errors.New("run event run_id is required")
	}

	rows, err := s.db.Query(
		`SELECT id, run_id, project_id, issue_id, event_type, stage, agent, data_json, error, created_at
		 FROM run_events
		 WHERE run_id=?
		 ORDER BY id ASC`,
		trimmed,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]core.RunEvent, 0)
	for rows.Next() {
		var (
			event    core.RunEvent
			dataJSON string
		)
		if err := rows.Scan(
			&event.ID,
			&event.RunID,
			&event.ProjectID,
			&event.IssueID,
			&event.EventType,
			&event.Stage,
			&event.Agent,
			&dataJSON,
			&event.Error,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := unmarshalJSONObject(dataJSON, &event.Data); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) DeleteChatSession(id string) error {
	result, err := s.db.Exec(`DELETE FROM chat_sessions WHERE id=?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("chat session %s not found", id)
	}
	return nil
}

func (s *SQLiteStore) CreateIssue(issue *core.Issue) error {
	if err := s.ensureIssueTables(); err != nil {
		return err
	}

	normalized := normalizeIssueForPersist(issue)
	if err := normalized.Validate(); err != nil {
		return err
	}

	labelsJSON, err := marshalJSON(normalized.Labels)
	if err != nil {
		return err
	}
	attachmentsJSON, err := marshalJSON(normalized.Attachments)
	if err != nil {
		return err
	}
	dependsOnJSON, err := marshalJSON(normalized.DependsOn)
	if err != nil {
		return err
	}
	blocksJSON, err := marshalJSON(normalized.Blocks)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		`INSERT INTO issues (
			id, project_id, session_id, title, body, labels, milestone_id, attachments, depends_on, blocks,
			priority, template, auto_merge, merge_retries, triage_instructions, submitted_by, state, status, run_id, version, superseded_by, external_id, fail_policy, parent_id, closed_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		normalized.ID, normalized.ProjectID, nullableString(normalized.SessionID), normalized.Title, normalized.Body, labelsJSON,
		normalized.MilestoneID, attachmentsJSON, dependsOnJSON, blocksJSON, normalized.Priority, normalized.Template,
		normalized.AutoMerge, normalized.MergeRetries, normalized.TriageInstructions, normalized.SubmittedBy, string(normalized.State), string(normalized.Status), nullableString(normalized.RunID), normalized.Version,
		normalized.SupersededBy, normalized.ExternalID, string(normalized.FailPolicy), normalized.ParentID, nullableTimePointer(normalized.ClosedAt),
	)
	if err != nil {
		return err
	}
	return s.bindRunIssueLink(normalized.RunID, normalized.ID)
}

func (s *SQLiteStore) GetIssue(id string) (*core.Issue, error) {
	if err := s.ensureIssueTables(); err != nil {
		return nil, err
	}

	issue := &core.Issue{}
	var (
		labelsJSON      string
		attachmentsJSON string
		dependsOnJSON   string
		blocksJSON      string
		closedAt        sql.NullTime
	)
	err := s.db.QueryRow(
		`SELECT id, project_id, COALESCE(session_id, ''), title, COALESCE(body, ''), COALESCE(labels, '[]'),
		        COALESCE(milestone_id, ''), COALESCE(attachments, '[]'), COALESCE(depends_on, '[]'), COALESCE(blocks, '[]'),
		        priority, template, COALESCE(auto_merge, 1), COALESCE(merge_retries, 0), COALESCE(triage_instructions, ''), COALESCE(submitted_by, ''), state, status, COALESCE(run_id, ''), version, COALESCE(superseded_by, ''),
		        COALESCE(external_id, ''), COALESCE(fail_policy, ''), COALESCE(parent_id, ''), closed_at, created_at, updated_at
		 FROM issues WHERE id=?`,
		id,
	).Scan(
		&issue.ID, &issue.ProjectID, &issue.SessionID, &issue.Title, &issue.Body, &labelsJSON,
		&issue.MilestoneID, &attachmentsJSON, &dependsOnJSON, &blocksJSON, &issue.Priority,
		&issue.Template, &issue.AutoMerge, &issue.MergeRetries, &issue.TriageInstructions, &issue.SubmittedBy, &issue.State, &issue.Status, &issue.RunID, &issue.Version, &issue.SupersededBy,
		&issue.ExternalID, &issue.FailPolicy, &issue.ParentID, &closedAt, &issue.CreatedAt, &issue.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", errIssueNotFound, id)
	}
	if err != nil {
		return nil, err
	}

	if err := unmarshalJSON(labelsJSON, &issue.Labels); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(attachmentsJSON, &issue.Attachments); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(dependsOnJSON, &issue.DependsOn); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(blocksJSON, &issue.Blocks); err != nil {
		return nil, err
	}
	if closedAt.Valid {
		t := closedAt.Time
		issue.ClosedAt = &t
	}
	return issue, nil
}

func (s *SQLiteStore) SaveIssue(issue *core.Issue) error {
	if err := s.ensureIssueTables(); err != nil {
		return err
	}

	normalized := normalizeIssueForPersist(issue)
	if err := normalized.Validate(); err != nil {
		return err
	}

	labelsJSON, err := marshalJSON(normalized.Labels)
	if err != nil {
		return err
	}
	attachmentsJSON, err := marshalJSON(normalized.Attachments)
	if err != nil {
		return err
	}
	dependsOnJSON, err := marshalJSON(normalized.DependsOn)
	if err != nil {
		return err
	}
	blocksJSON, err := marshalJSON(normalized.Blocks)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
INSERT INTO issues (
	id, project_id, session_id, title, body, labels, milestone_id, attachments, depends_on, blocks,
	priority, template, auto_merge, merge_retries, triage_instructions, submitted_by, state, status, run_id, version, superseded_by, external_id, fail_policy, parent_id, closed_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
	project_id=excluded.project_id,
	session_id=excluded.session_id,
	title=excluded.title,
	body=excluded.body,
	labels=excluded.labels,
	milestone_id=excluded.milestone_id,
	attachments=excluded.attachments,
	depends_on=excluded.depends_on,
	blocks=excluded.blocks,
	priority=excluded.priority,
	template=excluded.template,
	auto_merge=excluded.auto_merge,
	merge_retries=excluded.merge_retries,
	triage_instructions=excluded.triage_instructions,
	submitted_by=excluded.submitted_by,
	state=excluded.state,
	status=excluded.status,
	run_id=excluded.run_id,
	version=excluded.version,
	superseded_by=excluded.superseded_by,
	external_id=excluded.external_id,
	fail_policy=excluded.fail_policy,
	parent_id=excluded.parent_id,
	closed_at=excluded.closed_at,
	updated_at=CURRENT_TIMESTAMP`,
		normalized.ID, normalized.ProjectID, nullableString(normalized.SessionID), normalized.Title, normalized.Body, labelsJSON,
		normalized.MilestoneID, attachmentsJSON, dependsOnJSON, blocksJSON, normalized.Priority, normalized.Template,
		normalized.AutoMerge, normalized.MergeRetries, normalized.TriageInstructions, normalized.SubmittedBy, string(normalized.State), string(normalized.Status), nullableString(normalized.RunID), normalized.Version,
		normalized.SupersededBy, normalized.ExternalID, string(normalized.FailPolicy), normalized.ParentID, nullableTimePointer(normalized.ClosedAt),
	)
	if err != nil {
		return err
	}
	return s.bindRunIssueLink(normalized.RunID, normalized.ID)
}

func (s *SQLiteStore) ListIssues(projectID string, filter core.IssueFilter) ([]core.Issue, int, error) {
	if err := s.ensureIssueTables(); err != nil {
		return nil, 0, err
	}

	where := []string{"project_id=?"}
	args := []any{projectID}
	if strings.TrimSpace(filter.Status) != "" {
		where = append(where, "status=?")
		args = append(args, filter.Status)
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		where = append(where, "COALESCE(session_id, '')=?")
		args = append(args, filter.SessionID)
	}
	if strings.TrimSpace(filter.State) != "" {
		where = append(where, "state=?")
		args = append(args, filter.State)
	}
	if strings.TrimSpace(filter.ParentID) != "" {
		where = append(where, "COALESCE(parent_id, '')=?")
		args = append(args, filter.ParentID)
	}

	whereClause := strings.Join(where, " AND ")
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM issues WHERE %s`, whereClause)
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
SELECT id, project_id, COALESCE(session_id, ''), title, COALESCE(body, ''), COALESCE(labels, '[]'),
       COALESCE(milestone_id, ''), COALESCE(attachments, '[]'), COALESCE(depends_on, '[]'), COALESCE(blocks, '[]'),
       priority, template, COALESCE(auto_merge, 1), COALESCE(merge_retries, 0), COALESCE(triage_instructions, ''), COALESCE(submitted_by, ''), state, status, COALESCE(run_id, ''), version, COALESCE(superseded_by, ''),
       COALESCE(external_id, ''), COALESCE(fail_policy, ''), COALESCE(parent_id, ''), closed_at, created_at, updated_at
FROM issues
WHERE %s
ORDER BY created_at DESC`, whereClause)
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]core.Issue, 0)
	for rows.Next() {
		var (
			issue           core.Issue
			labelsJSON      string
			attachmentsJSON string
			dependsOnJSON   string
			blocksJSON      string
			closedAt        sql.NullTime
		)
		if err := rows.Scan(
			&issue.ID, &issue.ProjectID, &issue.SessionID, &issue.Title, &issue.Body, &labelsJSON,
			&issue.MilestoneID, &attachmentsJSON, &dependsOnJSON, &blocksJSON, &issue.Priority,
			&issue.Template, &issue.AutoMerge, &issue.MergeRetries, &issue.TriageInstructions, &issue.SubmittedBy, &issue.State, &issue.Status, &issue.RunID, &issue.Version, &issue.SupersededBy,
			&issue.ExternalID, &issue.FailPolicy, &issue.ParentID, &closedAt, &issue.CreatedAt, &issue.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		if err := unmarshalJSON(labelsJSON, &issue.Labels); err != nil {
			return nil, 0, err
		}
		if err := unmarshalJSON(attachmentsJSON, &issue.Attachments); err != nil {
			return nil, 0, err
		}
		if err := unmarshalJSON(dependsOnJSON, &issue.DependsOn); err != nil {
			return nil, 0, err
		}
		if err := unmarshalJSON(blocksJSON, &issue.Blocks); err != nil {
			return nil, 0, err
		}
		if closedAt.Valid {
			t := closedAt.Time
			issue.ClosedAt = &t
		}
		out = append(out, issue)
	}
	return out, total, rows.Err()
}

func (s *SQLiteStore) GetActiveIssues(projectID string) ([]core.Issue, error) {
	if err := s.ensureIssueTables(); err != nil {
		return nil, err
	}

	query := `
SELECT id
FROM issues
WHERE state='open' AND status IN ('reviewing','queued','ready','executing','merging','decomposing','decomposed')`
	args := []any{}
	if strings.TrimSpace(projectID) != "" {
		query += ` AND project_id=?`
		args = append(args, projectID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]core.Issue, 0, len(ids))
	for _, id := range ids {
		issue, err := s.GetIssue(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *issue)
	}
	return out, nil
}

func (s *SQLiteStore) GetChildIssues(parentID string) ([]core.Issue, error) {
	if err := s.ensureIssueTables(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id FROM issues WHERE parent_id=? ORDER BY priority DESC, created_at ASC`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]core.Issue, 0, len(ids))
	for _, id := range ids {
		issue, err := s.GetIssue(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *issue)
	}
	return out, nil
}

func (s *SQLiteStore) GetIssueByRun(RunID string) (*core.Issue, error) {
	if err := s.ensureIssueTables(); err != nil {
		return nil, err
	}

	var mappedIssueID string
	err := s.db.QueryRow(`SELECT COALESCE(issue_id, '') FROM runs WHERE id=?`, RunID).Scan(&mappedIssueID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if strings.TrimSpace(mappedIssueID) != "" {
		issue, getErr := s.GetIssue(mappedIssueID)
		if getErr == nil {
			return issue, nil
		}
		if !strings.Contains(getErr.Error(), "not found") {
			return nil, getErr
		}
	}

	issue := &core.Issue{}
	var (
		labelsJSON      string
		attachmentsJSON string
		dependsOnJSON   string
		blocksJSON      string
		closedAt        sql.NullTime
	)
	err = s.db.QueryRow(
		`SELECT id, project_id, COALESCE(session_id, ''), title, COALESCE(body, ''), COALESCE(labels, '[]'),
		        COALESCE(milestone_id, ''), COALESCE(attachments, '[]'), COALESCE(depends_on, '[]'), COALESCE(blocks, '[]'),
		        priority, template, COALESCE(auto_merge, 1), COALESCE(merge_retries, 0), COALESCE(triage_instructions, ''), state, status, COALESCE(run_id, ''), version, COALESCE(superseded_by, ''),
		        COALESCE(external_id, ''), COALESCE(fail_policy, ''), COALESCE(parent_id, ''), closed_at, created_at, updated_at
		 FROM issues WHERE run_id=? LIMIT 1`,
		RunID,
	).Scan(
		&issue.ID, &issue.ProjectID, &issue.SessionID, &issue.Title, &issue.Body, &labelsJSON,
		&issue.MilestoneID, &attachmentsJSON, &dependsOnJSON, &blocksJSON, &issue.Priority,
		&issue.Template, &issue.AutoMerge, &issue.MergeRetries, &issue.TriageInstructions, &issue.SubmittedBy, &issue.State, &issue.Status, &issue.RunID, &issue.Version, &issue.SupersededBy,
		&issue.ExternalID, &issue.FailPolicy, &issue.ParentID, &closedAt, &issue.CreatedAt, &issue.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalJSON(labelsJSON, &issue.Labels); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(attachmentsJSON, &issue.Attachments); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(dependsOnJSON, &issue.DependsOn); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(blocksJSON, &issue.Blocks); err != nil {
		return nil, err
	}
	if closedAt.Valid {
		t := closedAt.Time
		issue.ClosedAt = &t
	}
	return issue, nil
}

func (s *SQLiteStore) SaveIssueAttachment(att *core.IssueAttachment) error {
	if err := s.ensureIssueTables(); err != nil {
		return err
	}
	trimmedIssueID := strings.TrimSpace(att.IssueID)
	if trimmedIssueID == "" {
		return errors.New("issue attachment issue_id is required")
	}
	_, err := s.db.Exec(
		`INSERT INTO issue_attachments (issue_id, path, content, source_url, media_type) VALUES (?,?,?,?,?)`,
		trimmedIssueID, att.Path, att.Content, att.SourceURL, att.MediaType,
	)
	return err
}

func (s *SQLiteStore) GetIssueAttachments(issueID string) ([]core.IssueAttachment, error) {
	if err := s.ensureIssueTables(); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT id, issue_id, path, content, source_url, media_type, created_at
		 FROM issue_attachments
		 WHERE issue_id=?
		 ORDER BY id`,
		issueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]core.IssueAttachment, 0)
	for rows.Next() {
		var (
			attachment core.IssueAttachment
			id         int64
		)
		if err := rows.Scan(&id, &attachment.IssueID, &attachment.Path, &attachment.Content, &attachment.SourceURL, &attachment.MediaType, &attachment.CreatedAt); err != nil {
			return nil, err
		}
		attachment.ID = fmt.Sprintf("%d", id)
		out = append(out, attachment)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveIssueChange(change *core.IssueChange) error {
	if err := s.ensureIssueTables(); err != nil {
		return err
	}
	if change == nil {
		return errors.New("issue change is nil")
	}
	if strings.TrimSpace(change.IssueID) == "" {
		return errors.New("issue change issue_id is required")
	}

	_, err := s.db.Exec(
		`INSERT INTO issue_changes (issue_id, field, old_value, new_value, reason, changed_by)
		 VALUES (?,?,?,?,?,?)`,
		change.IssueID, change.Field, change.OldValue, change.NewValue, change.Reason, change.ChangedBy,
	)
	return err
}

func (s *SQLiteStore) GetIssueChanges(issueID string) ([]core.IssueChange, error) {
	if err := s.ensureIssueTables(); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT id, issue_id, field, old_value, new_value, reason, changed_by, created_at
		 FROM issue_changes
		 WHERE issue_id=?
		 ORDER BY id`,
		issueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]core.IssueChange, 0)
	for rows.Next() {
		var (
			change core.IssueChange
			id     int64
		)
		if err := rows.Scan(
			&id, &change.IssueID, &change.Field, &change.OldValue, &change.NewValue,
			&change.Reason, &change.ChangedBy, &change.CreatedAt,
		); err != nil {
			return nil, err
		}
		change.ID = fmt.Sprintf("%d", id)
		out = append(out, change)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveTaskStep(step *core.TaskStep) (core.IssueStatus, error) {
	if err := s.ensureIssueTables(); err != nil {
		return "", err
	}
	if err := s.ensureTaskStepTables(); err != nil {
		return "", err
	}
	if step == nil {
		return "", errors.New("task step is nil")
	}
	if err := step.Validate(); err != nil {
		return "", fmt.Errorf("invalid task step: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	createdAt := step.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	if _, err := tx.Exec(
		`INSERT INTO task_steps (id, issue_id, run_id, agent_id, action, stage_id, input, output, note, ref_id, ref_type, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		step.ID,
		step.IssueID,
		normalizeOptionalText(step.RunID),
		normalizeOptionalText(step.AgentID),
		string(step.Action),
		normalizeOptionalText(string(step.StageID)),
		normalizeOptionalText(step.Input),
		normalizeOptionalText(step.Output),
		normalizeOptionalText(step.Note),
		normalizeOptionalText(step.RefID),
		normalizeOptionalText(step.RefType),
		createdAt,
	); err != nil {
		return "", fmt.Errorf("insert task step: %w", err)
	}

	status, derived := step.Action.DeriveIssueStatus()
	if derived {
		result, err := tx.Exec(`UPDATE issues SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, string(status), step.IssueID)
		if err != nil {
			return "", fmt.Errorf("update issue status: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return "", fmt.Errorf("read updated issue rows: %w", err)
		}
		if affected == 0 {
			return "", fmt.Errorf("issue %s not found", step.IssueID)
		}
	} else {
		status, err = currentIssueStatusTx(tx, step.IssueID)
		if err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit task step tx: %w", err)
	}
	return status, nil
}

func (s *SQLiteStore) ListTaskSteps(issueID string) ([]core.TaskStep, error) {
	if strings.TrimSpace(issueID) == "" {
		return nil, fmt.Errorf("issue id is required")
	}
	if err := s.ensureIssueTables(); err != nil {
		return nil, err
	}
	if err := s.ensureTaskStepTables(); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT id, issue_id, COALESCE(run_id, ''), COALESCE(agent_id, ''), action, COALESCE(stage_id, ''),
		        COALESCE(input, ''), COALESCE(output, ''), COALESCE(note, ''), COALESCE(ref_id, ''), COALESCE(ref_type, ''), created_at
		 FROM task_steps
		 WHERE issue_id = ?
		 ORDER BY created_at ASC, id ASC`,
		issueID,
	)
	if err != nil {
		return nil, fmt.Errorf("query task_steps: %w", err)
	}
	defer rows.Close()

	steps := make([]core.TaskStep, 0)
	for rows.Next() {
		var (
			step      core.TaskStep
			action    string
			createdAt sql.NullTime
		)
		if err := rows.Scan(
			&step.ID,
			&step.IssueID,
			&step.RunID,
			&step.AgentID,
			&action,
			&step.StageID,
			&step.Input,
			&step.Output,
			&step.Note,
			&step.RefID,
			&step.RefType,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan task_step: %w", err)
		}
		step.Action = core.TaskStepAction(action)
		if createdAt.Valid {
			step.CreatedAt = createdAt.Time
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task_steps: %w", err)
	}
	return steps, nil
}

func (s *SQLiteStore) RebuildIssueStatus(issueID string) (core.IssueStatus, error) {
	if strings.TrimSpace(issueID) == "" {
		return "", fmt.Errorf("issue id is required")
	}
	if err := s.ensureIssueTables(); err != nil {
		return "", err
	}
	if err := s.ensureTaskStepTables(); err != nil {
		return "", err
	}

	steps, err := s.ListTaskSteps(issueID)
	if err != nil {
		return "", err
	}

	currentStatus := core.IssueStatusDraft
	if issue, err := s.GetIssue(issueID); err != nil {
		return "", err
	} else if strings.TrimSpace(string(issue.Status)) != "" {
		currentStatus = issue.Status
	}

	status := core.IssueStatusDraft
	derivedSeen := false
	for _, step := range steps {
		if derived, ok := step.Action.DeriveIssueStatus(); ok {
			status = derived
			derivedSeen = true
		}
	}
	if !derivedSeen {
		status = currentStatus
	}

	result, err := s.db.Exec(`UPDATE issues SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, string(status), issueID)
	if err != nil {
		return "", fmt.Errorf("rebuild issue status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("read rebuilt issue rows: %w", err)
	}
	if affected == 0 {
		return "", fmt.Errorf("issue %s not found", issueID)
	}
	return status, nil
}

func (s *SQLiteStore) SaveReviewRecord(record *core.ReviewRecord) error {
	issuesJSON, err := marshalJSON(record.Issues)
	if err != nil {
		return err
	}
	fixesJSON, err := marshalJSON(record.Fixes)
	if err != nil {
		return err
	}
	var score any
	if record.Score != nil {
		score = *record.Score
	}
	result, err := s.db.Exec(
		`INSERT INTO review_records (issue_id, round, reviewer, verdict, summary, raw_output, issues, fixes, score)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		record.IssueID, record.Round, record.Reviewer, record.Verdict, record.Summary, record.RawOutput, issuesJSON, fixesJSON, score,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	record.ID = id
	return nil
}

func (s *SQLiteStore) GetReviewRecords(issueID string) ([]core.ReviewRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, issue_id, round, reviewer, verdict, summary, raw_output, issues, fixes, score, created_at
		 FROM review_records
		 WHERE issue_id=?
		 ORDER BY id`,
		issueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.ReviewRecord
	for rows.Next() {
		var (
			record     core.ReviewRecord
			issuesJSON string
			fixesJSON  string
			score      sql.NullInt64
		)
		if err := rows.Scan(
			&record.ID, &record.IssueID, &record.Round, &record.Reviewer, &record.Verdict,
			&record.Summary, &record.RawOutput, &issuesJSON, &fixesJSON, &score, &record.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(issuesJSON, &record.Issues); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(fixesJSON, &record.Fixes); err != nil {
			return nil, err
		}
		if score.Valid {
			v := int(score.Int64)
			record.Score = &v
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveEvent(event core.UnifiedEvent) error {
	_, err := s.db.Exec(
		`INSERT INTO events (scope, event_type, project_id, run_id, issue_id, session_id, stage, agent, payload_json, error)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		event.Scope, event.EventType, event.ProjectID, event.RunID, event.IssueID,
		event.SessionID, event.Stage, event.Agent, event.PayloadJSON, event.Error,
	)
	return err
}

func (s *SQLiteStore) ListEvents(filter core.EventFilter) ([]core.UnifiedEvent, error) {
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(filter.Scope) != "" {
		where = append(where, "scope=?")
		args = append(args, filter.Scope)
	}
	if strings.TrimSpace(filter.ProjectID) != "" {
		where = append(where, "project_id=?")
		args = append(args, filter.ProjectID)
	}
	if strings.TrimSpace(filter.RunID) != "" {
		where = append(where, "run_id=?")
		args = append(args, filter.RunID)
	}
	if strings.TrimSpace(filter.IssueID) != "" {
		where = append(where, "issue_id=?")
		args = append(args, filter.IssueID)
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		where = append(where, "session_id=?")
		args = append(args, filter.SessionID)
	}
	if strings.TrimSpace(filter.EventType) != "" {
		where = append(where, "event_type=?")
		args = append(args, filter.EventType)
	}

	query := fmt.Sprintf(
		`SELECT id, scope, event_type, project_id, run_id, issue_id, session_id, stage, agent, payload_json, error, created_at
		 FROM events WHERE %s ORDER BY id ASC`, strings.Join(where, " AND "))

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]core.UnifiedEvent, 0)
	for rows.Next() {
		var e core.UnifiedEvent
		if err := rows.Scan(
			&e.ID, &e.Scope, &e.EventType, &e.ProjectID, &e.RunID,
			&e.IssueID, &e.SessionID, &e.Stage, &e.Agent, &e.PayloadJSON,
			&e.Error, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) bindRunIssueLink(RunID, issueID string) error {
	RunID = strings.TrimSpace(RunID)
	issueID = strings.TrimSpace(issueID)
	if RunID == "" || issueID == "" {
		return nil
	}
	query := `
UPDATE runs
SET issue_id = CASE
	WHEN COALESCE(issue_id, '') = '' THEN ?
	ELSE issue_id
END
WHERE id=?`
	_, err := s.db.Exec(query, issueID, RunID)
	return err
}

func (s *SQLiteStore) ensureIssueTables() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS issues (
	id            TEXT PRIMARY KEY,
	project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	session_id    TEXT REFERENCES chat_sessions(id) ON DELETE SET NULL,
	title         TEXT NOT NULL,
	body          TEXT NOT NULL DEFAULT '',
	labels        TEXT NOT NULL DEFAULT '[]',
	milestone_id  TEXT NOT NULL DEFAULT '',
	attachments   TEXT NOT NULL DEFAULT '[]',
	depends_on    TEXT NOT NULL DEFAULT '[]',
	blocks        TEXT NOT NULL DEFAULT '[]',
	priority      INTEGER NOT NULL DEFAULT 0,
	template      TEXT NOT NULL DEFAULT 'standard',
	auto_merge    INTEGER NOT NULL DEFAULT 1,
	merge_retries INTEGER NOT NULL DEFAULT 0,
	triage_instructions TEXT NOT NULL DEFAULT '',
	submitted_by  TEXT NOT NULL DEFAULT '',
	state         TEXT NOT NULL DEFAULT 'open',
	status        TEXT NOT NULL DEFAULT 'draft',
	run_id   TEXT REFERENCES runs(id) ON DELETE SET NULL,
	version       INTEGER NOT NULL DEFAULT 1,
	superseded_by TEXT NOT NULL DEFAULT '',
	external_id   TEXT NOT NULL DEFAULT '',
	fail_policy   TEXT NOT NULL DEFAULT 'block',
	closed_at     DATETIME,
	created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_issues_project ON issues(project_id);
CREATE INDEX IF NOT EXISTS idx_issues_project_status ON issues(project_id, status);
CREATE INDEX IF NOT EXISTS idx_issues_run ON issues(run_id);

CREATE TABLE IF NOT EXISTS issue_attachments (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_id   TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	path       TEXT NOT NULL,
	content    TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_issue_attachments_issue ON issue_attachments(issue_id);

CREATE TABLE IF NOT EXISTS issue_changes (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_id   TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	field      TEXT NOT NULL,
	old_value  TEXT,
	new_value  TEXT,
	reason     TEXT,
	changed_by TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_issue_changes_issue ON issue_changes(issue_id);
`)
	if err != nil {
		return fmt.Errorf("ensure issue tables: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ensureTaskStepTables() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS task_steps (
	id         TEXT PRIMARY KEY,
	issue_id   TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	run_id     TEXT NOT NULL DEFAULT '',
	agent_id   TEXT NOT NULL DEFAULT '',
	action     TEXT NOT NULL,
	stage_id   TEXT NOT NULL DEFAULT '',
	input      TEXT NOT NULL DEFAULT '',
	output     TEXT NOT NULL DEFAULT '',
	note       TEXT NOT NULL DEFAULT '',
	ref_id     TEXT NOT NULL DEFAULT '',
	ref_type   TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_task_steps_issue ON task_steps(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_task_steps_run ON task_steps(run_id, created_at);
`)
	if err != nil {
		return fmt.Errorf("ensure task step tables: %w", err)
	}
	return nil
}

func normalizeOptionalText(value string) string {
	return strings.TrimSpace(value)
}

func currentIssueStatusTx(tx *sql.Tx, issueID string) (core.IssueStatus, error) {
	var status string
	if err := tx.QueryRow(`SELECT status FROM issues WHERE id = ?`, issueID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("issue %s not found", issueID)
		}
		return "", fmt.Errorf("load issue status: %w", err)
	}
	return core.IssueStatus(status), nil
}

func normalizeIssueForPersist(issue *core.Issue) core.Issue {
	if issue == nil {
		return core.Issue{}
	}
	normalized := *issue
	if strings.TrimSpace(normalized.Template) == "" {
		normalized.Template = "standard"
	}
	if strings.TrimSpace(string(normalized.State)) == "" {
		normalized.State = core.IssueStateOpen
	}
	if strings.TrimSpace(string(normalized.Status)) == "" {
		normalized.Status = core.IssueStatusDraft
	}
	if strings.TrimSpace(string(normalized.FailPolicy)) == "" {
		normalized.FailPolicy = core.FailBlock
	}
	if normalized.Labels == nil {
		normalized.Labels = []string{}
	}
	if normalized.Attachments == nil {
		normalized.Attachments = []string{}
	}
	if normalized.DependsOn == nil {
		normalized.DependsOn = []string{}
	}
	if normalized.Blocks == nil {
		normalized.Blocks = []string{}
	}
	if normalized.Version <= 0 {
		normalized.Version = 1
	}
	if normalized.MergeRetries < 0 {
		normalized.MergeRetries = 0
	}
	return normalized
}

func marshalJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalJSON(raw string, target any) error {
	return unmarshalJSONWithDefault(raw, "[]", target)
}

func unmarshalJSONObject(raw string, target any) error {
	return unmarshalJSONWithDefault(raw, "{}", target)
}

func unmarshalJSONWithDefault(raw, fallback string, target any) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = fallback
	}
	return json.Unmarshal([]byte(trimmed), target)
}

func nullableString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullableTimePointer(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return *t
}
