package storesqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/user/ai-workflow/internal/core"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func New(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if path == ":memory:" {
		// SQLite in-memory DB is connection-scoped; keep a single connection for tests.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
	if err := applyMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) CreateProject(p *core.Project) error {
	_, err := s.db.Exec(
		`INSERT INTO projects (id, name, repo_path, github_owner, github_repo) VALUES (?,?,?,?,?)`,
		p.ID, p.Name, p.RepoPath, p.GitHubOwner, p.GitHubRepo,
	)
	return err
}

func (s *SQLiteStore) GetProject(id string) (*core.Project, error) {
	p := &core.Project{}
	err := s.db.QueryRow(
		`SELECT id, name, repo_path, github_owner, github_repo, created_at, updated_at FROM projects WHERE id=?`,
		id,
	).Scan(&p.ID, &p.Name, &p.RepoPath, &p.GitHubOwner, &p.GitHubRepo, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("project %s not found", id)
	}
	return p, err
}

func (s *SQLiteStore) UpdateProject(p *core.Project) error {
	_, err := s.db.Exec(
		`UPDATE projects SET name=?, repo_path=?, github_owner=?, github_repo=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		p.Name, p.RepoPath, p.GitHubOwner, p.GitHubRepo, p.ID,
	)
	return err
}

func (s *SQLiteStore) DeleteProject(id string) error {
	// Child rows are cleaned by ON DELETE CASCADE via pipelines.project_id foreign key.
	_, err := s.db.Exec(`DELETE FROM projects WHERE id=?`, id)
	return err
}

func (s *SQLiteStore) ListProjects(filter core.ProjectFilter) ([]core.Project, error) {
	query := `SELECT id, name, repo_path, github_owner, github_repo, created_at, updated_at FROM projects`
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
		if err := rows.Scan(&p.ID, &p.Name, &p.RepoPath, &p.GitHubOwner, &p.GitHubRepo, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SavePipeline(p *core.Pipeline) error {
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

	_, err = s.db.Exec(`
INSERT INTO pipelines (
	id, project_id, name, description, template, status, current_stage,
	stages_json, artifacts_json, config_json, branch_name, worktree_path,
	error_message, max_total_retries, total_retries, run_count, last_error_type,
	queued_at, last_heartbeat_at, started_at, finished_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
	project_id=excluded.project_id,
	name=excluded.name,
	description=excluded.description,
	template=excluded.template,
	status=excluded.status,
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
	queued_at=excluded.queued_at,
	last_heartbeat_at=excluded.last_heartbeat_at,
	started_at=excluded.started_at,
	finished_at=excluded.finished_at,
	updated_at=CURRENT_TIMESTAMP`,
		p.ID, p.ProjectID, p.Name, p.Description, p.Template, p.Status, p.CurrentStage,
		string(stagesJSON), string(artifactsJSON), string(configJSON), p.BranchName, p.WorktreePath,
		p.ErrorMessage, p.MaxTotalRetries, p.TotalRetries, p.RunCount, p.LastErrorType,
		nullableTime(p.QueuedAt), nullableTime(p.LastHeartbeatAt), nullableTime(p.StartedAt), nullableTime(p.FinishedAt),
	)
	return err
}

func (s *SQLiteStore) GetPipeline(id string) (*core.Pipeline, error) {
	p := &core.Pipeline{}
	var (
		stagesJSON    string
		artifactsJSON string
		configJSON    string
		queuedAt      sql.NullTime
		lastHeartbeat sql.NullTime
		startedAt     sql.NullTime
		finishedAt    sql.NullTime
	)

	err := s.db.QueryRow(`
SELECT id, project_id, name, description, template, status, current_stage,
       stages_json, artifacts_json, config_json, branch_name, worktree_path, error_message,
       max_total_retries, total_retries, run_count, last_error_type, queued_at, last_heartbeat_at,
	   started_at, finished_at, created_at, updated_at
FROM pipelines WHERE id=?`,
		id,
	).Scan(
		&p.ID, &p.ProjectID, &p.Name, &p.Description, &p.Template, &p.Status, &p.CurrentStage,
		&stagesJSON, &artifactsJSON, &configJSON, &p.BranchName, &p.WorktreePath, &p.ErrorMessage,
		&p.MaxTotalRetries, &p.TotalRetries, &p.RunCount, &p.LastErrorType, &queuedAt, &lastHeartbeat,
		&startedAt, &finishedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("pipeline %s not found", id)
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

func (s *SQLiteStore) ListPipelines(projectID string, filter core.PipelineFilter) ([]core.Pipeline, error) {
	query := `SELECT id, project_id, name, template, status, current_stage, created_at FROM pipelines WHERE project_id=?`
	args := []any{projectID}
	if filter.Status != "" {
		query += ` AND status=?`
		args = append(args, filter.Status)
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

	var out []core.Pipeline
	for rows.Next() {
		var p core.Pipeline
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Template, &p.Status, &p.CurrentStage, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetActivePipelines() ([]core.Pipeline, error) {
	rows, err := s.db.Query(`SELECT id FROM pipelines WHERE status IN ('running','paused','waiting_human')`)
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

	out := make([]core.Pipeline, 0, len(ids))
	for _, id := range ids {
		p, err := s.GetPipeline(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

func (s *SQLiteStore) ListRunnablePipelines(limit int) ([]core.Pipeline, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(`
SELECT id
FROM pipelines
WHERE status = ?
ORDER BY COALESCE(queued_at, created_at) ASC, created_at ASC
LIMIT ?`,
		core.StatusCreated, limit,
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

	out := make([]core.Pipeline, 0, len(ids))
	for _, id := range ids {
		p, err := s.GetPipeline(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

func (s *SQLiteStore) CountRunningPipelinesByProject(projectID string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pipelines WHERE project_id=? AND status=?`,
		projectID, core.StatusRunning,
	).Scan(&count)
	return count, err
}

func (s *SQLiteStore) TryMarkPipelineRunning(id string, from ...core.PipelineStatus) (bool, error) {
	if len(from) == 0 {
		from = []core.PipelineStatus{core.StatusCreated}
	}

	placeholders := make([]string, len(from))
	args := make([]any, 0, len(from)+2)
	args = append(args, core.StatusRunning, id)
	for i, status := range from {
		placeholders[i] = "?"
		args = append(args, status)
	}

	query := fmt.Sprintf(`
UPDATE pipelines
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
		`INSERT INTO checkpoints (pipeline_id, stage, status, agent_used, artifacts_json, tokens_used, retry_count, error_message, started_at, finished_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		cp.PipelineID, cp.StageName, cp.Status, cp.AgentUsed, string(artifactsJSON), cp.TokensUsed, cp.RetryCount, cp.Error, cp.StartedAt, nullableTime(cp.FinishedAt),
	)
	return err
}

func (s *SQLiteStore) GetCheckpoints(pipelineID string) ([]core.Checkpoint, error) {
	rows, err := s.db.Query(
		`SELECT pipeline_id, stage, status, agent_used, artifacts_json, tokens_used, retry_count, error_message, started_at, finished_at FROM checkpoints WHERE pipeline_id=? ORDER BY id`,
		pipelineID,
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
		if err := rows.Scan(&cp.PipelineID, &cp.StageName, &cp.Status, &cp.AgentUsed, &artifactsJSON, &cp.TokensUsed, &cp.RetryCount, &cp.Error, &cp.StartedAt, &finishedAt); err != nil {
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

func (s *SQLiteStore) GetLastSuccessCheckpoint(pipelineID string) (*core.Checkpoint, error) {
	var (
		cp            core.Checkpoint
		artifactsJSON string
		finishedAt    sql.NullTime
	)
	err := s.db.QueryRow(
		`SELECT pipeline_id, stage, status, agent_used, artifacts_json, started_at, finished_at
		 FROM checkpoints WHERE pipeline_id=? AND status='success' ORDER BY id DESC LIMIT 1`,
		pipelineID,
	).Scan(&cp.PipelineID, &cp.StageName, &cp.Status, &cp.AgentUsed, &artifactsJSON, &cp.StartedAt, &finishedAt)
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

func (s *SQLiteStore) InvalidateCheckpointsFromStage(pipelineID string, stage core.StageID) error {
	_, err := s.db.Exec(`
UPDATE checkpoints
SET status=?
WHERE pipeline_id=? AND id >= (
	SELECT MIN(id)
	FROM checkpoints
	WHERE pipeline_id=? AND stage=?
)`, core.CheckpointInvalidated, pipelineID, pipelineID, stage)
	return err
}

func (s *SQLiteStore) AppendLog(entry core.LogEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO logs (pipeline_id, stage, type, agent, content, timestamp) VALUES (?,?,?,?,?,?)`,
		entry.PipelineID, entry.Stage, entry.Type, entry.Agent, entry.Content, entry.Timestamp,
	)
	return err
}

func (s *SQLiteStore) GetLogs(pipelineID string, stage string, limit int, offset int) ([]core.LogEntry, int, error) {
	var total int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM logs WHERE pipeline_id=? AND (? = '' OR stage=?)`,
		pipelineID, stage, stage,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, pipeline_id, stage, type, agent, content, timestamp
		 FROM logs WHERE pipeline_id=? AND (? = '' OR stage=?)
		 ORDER BY id LIMIT ? OFFSET ?`,
		pipelineID, stage, stage, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []core.LogEntry
	for rows.Next() {
		var e core.LogEntry
		if err := rows.Scan(&e.ID, &e.PipelineID, &e.Stage, &e.Type, &e.Agent, &e.Content, &e.Timestamp); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (s *SQLiteStore) RecordAction(a core.HumanAction) error {
	_, err := s.db.Exec(
		`INSERT INTO human_actions (pipeline_id, stage, action, message, source, user_id) VALUES (?,?,?,?,?,?)`,
		a.PipelineID, a.Stage, a.Action, a.Message, a.Source, a.UserID,
	)
	return err
}

func (s *SQLiteStore) GetActions(pipelineID string) ([]core.HumanAction, error) {
	rows, err := s.db.Query(
		`SELECT id, pipeline_id, stage, action, message, source, user_id, created_at
		 FROM human_actions WHERE pipeline_id=? ORDER BY id`,
		pipelineID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.HumanAction
	for rows.Next() {
		var a core.HumanAction
		if err := rows.Scan(&a.ID, &a.PipelineID, &a.Stage, &a.Action, &a.Message, &a.Source, &a.UserID, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
