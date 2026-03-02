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
	error_message, max_total_retries, total_retries, run_count, last_error_type, task_item_id,
	queued_at, last_heartbeat_at, started_at, finished_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
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
	task_item_id=excluded.task_item_id,
	queued_at=excluded.queued_at,
	last_heartbeat_at=excluded.last_heartbeat_at,
	started_at=excluded.started_at,
	finished_at=excluded.finished_at,
	updated_at=CURRENT_TIMESTAMP`,
		p.ID, p.ProjectID, p.Name, p.Description, p.Template, p.Status, p.CurrentStage,
		string(stagesJSON), string(artifactsJSON), string(configJSON), p.BranchName, p.WorktreePath,
		p.ErrorMessage, p.MaxTotalRetries, p.TotalRetries, p.RunCount, p.LastErrorType, nullableString(p.TaskItemID),
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
       max_total_retries, total_retries, run_count, last_error_type, COALESCE(task_item_id, ''), queued_at, last_heartbeat_at,
	   started_at, finished_at, created_at, updated_at
FROM pipelines WHERE id=?`,
		id,
	).Scan(
		&p.ID, &p.ProjectID, &p.Name, &p.Description, &p.Template, &p.Status, &p.CurrentStage,
		&stagesJSON, &artifactsJSON, &configJSON, &p.BranchName, &p.WorktreePath, &p.ErrorMessage,
		&p.MaxTotalRetries, &p.TotalRetries, &p.RunCount, &p.LastErrorType, &p.TaskItemID, &queuedAt, &lastHeartbeat,
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
	query := `SELECT id, project_id, name, template, status, current_stage, COALESCE(task_item_id, ''), created_at FROM pipelines WHERE project_id=?`
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
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Template, &p.Status, &p.CurrentStage, &p.TaskItemID, &p.CreatedAt); err != nil {
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

func (s *SQLiteStore) CreateChatSession(session *core.ChatSession) error {
	messagesJSON, err := marshalJSON(session.Messages)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO chat_sessions (id, project_id, agent_session_id, messages) VALUES (?,?,?,?)`,
		session.ID, session.ProjectID, session.AgentSessionID, messagesJSON,
	)
	return err
}

func (s *SQLiteStore) GetChatSession(id string) (*core.ChatSession, error) {
	session := &core.ChatSession{}
	var messagesJSON string
	err := s.db.QueryRow(
		`SELECT id, project_id, COALESCE(agent_session_id, ''), messages, created_at, updated_at FROM chat_sessions WHERE id=?`,
		id,
	).Scan(&session.ID, &session.ProjectID, &session.AgentSessionID, &messagesJSON, &session.CreatedAt, &session.UpdatedAt)
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
		`SELECT id, project_id, COALESCE(agent_session_id, ''), messages, created_at, updated_at
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
		if err := rows.Scan(&session.ID, &session.ProjectID, &session.AgentSessionID, &messagesJSON, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(messagesJSON, &session.Messages); err != nil {
			return nil, err
		}
		out = append(out, session)
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

func (s *SQLiteStore) CreateTaskPlan(plan *core.TaskPlan) error {
	sourceFilesJSON, err := marshalJSON(plan.SourceFiles)
	if err != nil {
		return err
	}
	fileContentsJSON, err := marshalJSON(plan.FileContents)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		`INSERT INTO task_plans (
			id, project_id, session_id, name, status, wait_reason, fail_policy, review_round,
			spec_profile, contract_version, contract_checksum, source_files, file_contents
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		plan.ID, plan.ProjectID, nullableString(plan.SessionID), plan.Name, plan.Status, plan.WaitReason, plan.FailPolicy, plan.ReviewRound,
		plan.SpecProfile, plan.ContractVersion, plan.ContractChecksum, sourceFilesJSON, fileContentsJSON,
	)
	return err
}

func (s *SQLiteStore) GetTaskPlan(id string) (*core.TaskPlan, error) {
	plan := &core.TaskPlan{}
	var (
		sourceFilesJSON  string
		fileContentsJSON string
	)
	err := s.db.QueryRow(
		`SELECT id, project_id, COALESCE(session_id, ''), name, status, wait_reason, fail_policy, review_round,
		        COALESCE(spec_profile, ''), COALESCE(contract_version, ''), COALESCE(contract_checksum, ''),
		        COALESCE(source_files, '[]'), COALESCE(file_contents, '{}'), created_at, updated_at
		 FROM task_plans WHERE id=?`,
		id,
	).Scan(
		&plan.ID, &plan.ProjectID, &plan.SessionID, &plan.Name, &plan.Status, &plan.WaitReason, &plan.FailPolicy,
		&plan.ReviewRound, &plan.SpecProfile, &plan.ContractVersion, &plan.ContractChecksum,
		&sourceFilesJSON, &fileContentsJSON, &plan.CreatedAt, &plan.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task plan %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalJSON(sourceFilesJSON, &plan.SourceFiles); err != nil {
		return nil, err
	}
	if err := unmarshalJSONObject(fileContentsJSON, &plan.FileContents); err != nil {
		return nil, err
	}
	tasks, err := s.GetTaskItemsByPlan(plan.ID)
	if err != nil {
		return nil, err
	}
	plan.Tasks = tasks
	return plan, nil
}

func (s *SQLiteStore) SaveTaskPlan(plan *core.TaskPlan) error {
	sourceFilesJSON, err := marshalJSON(plan.SourceFiles)
	if err != nil {
		return err
	}
	fileContentsJSON, err := marshalJSON(plan.FileContents)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
INSERT INTO task_plans (
	id, project_id, session_id, name, status, wait_reason, fail_policy, review_round,
	spec_profile, contract_version, contract_checksum, source_files, file_contents
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
	project_id=excluded.project_id,
	session_id=excluded.session_id,
	name=excluded.name,
	status=excluded.status,
	wait_reason=excluded.wait_reason,
	fail_policy=excluded.fail_policy,
	review_round=excluded.review_round,
	spec_profile=excluded.spec_profile,
	contract_version=excluded.contract_version,
	contract_checksum=excluded.contract_checksum,
	source_files=excluded.source_files,
	file_contents=excluded.file_contents,
	updated_at=CURRENT_TIMESTAMP`,
		plan.ID, plan.ProjectID, nullableString(plan.SessionID), plan.Name, plan.Status, plan.WaitReason, plan.FailPolicy, plan.ReviewRound,
		plan.SpecProfile, plan.ContractVersion, plan.ContractChecksum, sourceFilesJSON, fileContentsJSON,
	)
	return err
}

// ReplaceTaskPlanAndItems atomically upserts a plan and replaces all its task_items.
func (s *SQLiteStore) ReplaceTaskPlanAndItems(plan *core.TaskPlan, items []core.TaskItem) error {
	sourceFilesJSON, err := marshalJSON(plan.SourceFiles)
	if err != nil {
		return err
	}
	fileContentsJSON, err := marshalJSON(plan.FileContents)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec(`
INSERT INTO task_plans (
	id, project_id, session_id, name, status, wait_reason, fail_policy, review_round,
	spec_profile, contract_version, contract_checksum, source_files, file_contents
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
	project_id=excluded.project_id,
	session_id=excluded.session_id,
	name=excluded.name,
	status=excluded.status,
	wait_reason=excluded.wait_reason,
	fail_policy=excluded.fail_policy,
	review_round=excluded.review_round,
	spec_profile=excluded.spec_profile,
	contract_version=excluded.contract_version,
	contract_checksum=excluded.contract_checksum,
	source_files=excluded.source_files,
	file_contents=excluded.file_contents,
	updated_at=CURRENT_TIMESTAMP`,
		plan.ID, plan.ProjectID, nullableString(plan.SessionID), plan.Name, plan.Status, plan.WaitReason, plan.FailPolicy, plan.ReviewRound,
		plan.SpecProfile, plan.ContractVersion, plan.ContractChecksum, sourceFilesJSON, fileContentsJSON,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM task_items WHERE plan_id=?`, plan.ID); err != nil {
		return err
	}

	for i := range items {
		item := items[i]
		if err := item.Validate(); err != nil {
			return err
		}

		labelsJSON, err := marshalJSON(item.Labels)
		if err != nil {
			return err
		}
		dependsOnJSON, err := marshalJSON(item.DependsOn)
		if err != nil {
			return err
		}
		inputsJSON, err := marshalJSON(item.Inputs)
		if err != nil {
			return err
		}
		outputsJSON, err := marshalJSON(item.Outputs)
		if err != nil {
			return err
		}
		acceptanceJSON, err := marshalJSON(item.Acceptance)
		if err != nil {
			return err
		}
		constraintsJSON, err := marshalJSON(item.Constraints)
		if err != nil {
			return err
		}
		template := item.Template
		if strings.TrimSpace(template) == "" {
			template = "standard"
		}

		if _, err := tx.Exec(
			`INSERT INTO task_items (
				id, plan_id, title, description, labels, depends_on, inputs, outputs, acceptance, constraints,
				template, pipeline_id, external_id, status
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			item.ID, item.PlanID, item.Title, item.Description, labelsJSON, dependsOnJSON,
			inputsJSON, outputsJSON, acceptanceJSON, constraintsJSON, template,
			nullableString(item.PipelineID), nullableString(item.ExternalID), item.Status,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func (s *SQLiteStore) ListTaskPlans(projectID string, filter core.TaskPlanFilter) ([]core.TaskPlan, error) {
	query := `SELECT id, project_id, COALESCE(session_id, ''), name, status, wait_reason, fail_policy, review_round,
	                 COALESCE(spec_profile, ''), COALESCE(contract_version, ''), COALESCE(contract_checksum, ''),
	                 COALESCE(source_files, '[]'), COALESCE(file_contents, '{}'), created_at, updated_at
	          FROM task_plans WHERE project_id=?`
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

	var out []core.TaskPlan
	for rows.Next() {
		var (
			plan             core.TaskPlan
			sourceFilesJSON  string
			fileContentsJSON string
		)
		if err := rows.Scan(
			&plan.ID, &plan.ProjectID, &plan.SessionID, &plan.Name, &plan.Status, &plan.WaitReason, &plan.FailPolicy,
			&plan.ReviewRound, &plan.SpecProfile, &plan.ContractVersion, &plan.ContractChecksum,
			&sourceFilesJSON, &fileContentsJSON, &plan.CreatedAt, &plan.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(sourceFilesJSON, &plan.SourceFiles); err != nil {
			return nil, err
		}
		if err := unmarshalJSONObject(fileContentsJSON, &plan.FileContents); err != nil {
			return nil, err
		}
		out = append(out, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		tasks, err := s.GetTaskItemsByPlan(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Tasks = tasks
	}
	return out, nil
}

func (s *SQLiteStore) GetActiveTaskPlans() ([]core.TaskPlan, error) {
	rows, err := s.db.Query(
		`SELECT id FROM task_plans
		 WHERE status IN ('reviewing','approved','waiting_human','executing')
		 ORDER BY created_at DESC`,
	)
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

	out := make([]core.TaskPlan, 0, len(ids))
	for _, id := range ids {
		plan, err := s.GetTaskPlan(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *plan)
	}
	return out, nil
}

func (s *SQLiteStore) DeleteTaskItemsByPlan(planID string) error {
	_, err := s.db.Exec(`DELETE FROM task_items WHERE plan_id=?`, planID)
	return err
}

func (s *SQLiteStore) CreateTaskItem(item *core.TaskItem) error {
	if err := item.Validate(); err != nil {
		return err
	}
	labelsJSON, err := marshalJSON(item.Labels)
	if err != nil {
		return err
	}
	dependsOnJSON, err := marshalJSON(item.DependsOn)
	if err != nil {
		return err
	}
	inputsJSON, err := marshalJSON(item.Inputs)
	if err != nil {
		return err
	}
	outputsJSON, err := marshalJSON(item.Outputs)
	if err != nil {
		return err
	}
	acceptanceJSON, err := marshalJSON(item.Acceptance)
	if err != nil {
		return err
	}
	constraintsJSON, err := marshalJSON(item.Constraints)
	if err != nil {
		return err
	}
	template := item.Template
	if strings.TrimSpace(template) == "" {
		template = "standard"
	}
	_, err = s.db.Exec(
		`INSERT INTO task_items (
			id, plan_id, title, description, labels, depends_on, inputs, outputs, acceptance, constraints,
			template, pipeline_id, external_id, status
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.ID, item.PlanID, item.Title, item.Description, labelsJSON, dependsOnJSON,
		inputsJSON, outputsJSON, acceptanceJSON, constraintsJSON, template,
		nullableString(item.PipelineID), nullableString(item.ExternalID), item.Status,
	)
	if err != nil {
		return err
	}
	return s.bindPipelineTaskItemLink(item.PipelineID, item.ID)
}

func (s *SQLiteStore) GetTaskItem(id string) (*core.TaskItem, error) {
	item := &core.TaskItem{}
	var (
		labelsJSON      string
		dependsOnJSON   string
		inputsJSON      string
		outputsJSON     string
		acceptanceJSON  string
		constraintsJSON string
	)
	err := s.db.QueryRow(
		`SELECT id, plan_id, title, description, labels, depends_on, inputs, outputs, acceptance, constraints, template, COALESCE(pipeline_id, ''), COALESCE(external_id, ''), status, created_at, updated_at
		 FROM task_items WHERE id=?`,
		id,
	).Scan(
		&item.ID, &item.PlanID, &item.Title, &item.Description, &labelsJSON, &dependsOnJSON,
		&inputsJSON, &outputsJSON, &acceptanceJSON, &constraintsJSON, &item.Template,
		&item.PipelineID, &item.ExternalID, &item.Status, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task item %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalJSON(labelsJSON, &item.Labels); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(dependsOnJSON, &item.DependsOn); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(inputsJSON, &item.Inputs); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(outputsJSON, &item.Outputs); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(acceptanceJSON, &item.Acceptance); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(constraintsJSON, &item.Constraints); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *SQLiteStore) SaveTaskItem(item *core.TaskItem) error {
	if err := item.Validate(); err != nil {
		return err
	}
	labelsJSON, err := marshalJSON(item.Labels)
	if err != nil {
		return err
	}
	dependsOnJSON, err := marshalJSON(item.DependsOn)
	if err != nil {
		return err
	}
	inputsJSON, err := marshalJSON(item.Inputs)
	if err != nil {
		return err
	}
	outputsJSON, err := marshalJSON(item.Outputs)
	if err != nil {
		return err
	}
	acceptanceJSON, err := marshalJSON(item.Acceptance)
	if err != nil {
		return err
	}
	constraintsJSON, err := marshalJSON(item.Constraints)
	if err != nil {
		return err
	}
	template := item.Template
	if strings.TrimSpace(template) == "" {
		template = "standard"
	}
	_, err = s.db.Exec(`
INSERT INTO task_items (
	id, plan_id, title, description, labels, depends_on, inputs, outputs, acceptance, constraints, template, pipeline_id, external_id, status
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
	plan_id=excluded.plan_id,
	title=excluded.title,
	description=excluded.description,
	labels=excluded.labels,
	depends_on=excluded.depends_on,
	inputs=excluded.inputs,
	outputs=excluded.outputs,
	acceptance=excluded.acceptance,
	constraints=excluded.constraints,
	template=excluded.template,
	pipeline_id=excluded.pipeline_id,
	external_id=excluded.external_id,
	status=excluded.status,
	updated_at=CURRENT_TIMESTAMP`,
		item.ID, item.PlanID, item.Title, item.Description, labelsJSON, dependsOnJSON,
		inputsJSON, outputsJSON, acceptanceJSON, constraintsJSON, template,
		nullableString(item.PipelineID), nullableString(item.ExternalID), item.Status,
	)
	if err != nil {
		return err
	}
	return s.bindPipelineTaskItemLink(item.PipelineID, item.ID)
}

func (s *SQLiteStore) GetTaskItemsByPlan(planID string) ([]core.TaskItem, error) {
	rows, err := s.db.Query(
		`SELECT id, plan_id, title, description, labels, depends_on, inputs, outputs, acceptance, constraints, template, COALESCE(pipeline_id, ''), COALESCE(external_id, ''), status, created_at, updated_at
		 FROM task_items WHERE plan_id=?
		 ORDER BY created_at, id`,
		planID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.TaskItem
	for rows.Next() {
		var (
			item            core.TaskItem
			labelsJSON      string
			dependsOnJSON   string
			inputsJSON      string
			outputsJSON     string
			acceptanceJSON  string
			constraintsJSON string
		)
		if err := rows.Scan(
			&item.ID, &item.PlanID, &item.Title, &item.Description, &labelsJSON, &dependsOnJSON,
			&inputsJSON, &outputsJSON, &acceptanceJSON, &constraintsJSON, &item.Template,
			&item.PipelineID, &item.ExternalID, &item.Status, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(labelsJSON, &item.Labels); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(dependsOnJSON, &item.DependsOn); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(inputsJSON, &item.Inputs); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(outputsJSON, &item.Outputs); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(acceptanceJSON, &item.Acceptance); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(constraintsJSON, &item.Constraints); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetTaskItemByPipeline(pipelineID string) (*core.TaskItem, error) {
	var mappedTaskID string
	err := s.db.QueryRow(`SELECT COALESCE(task_item_id, '') FROM pipelines WHERE id=?`, pipelineID).Scan(&mappedTaskID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if strings.TrimSpace(mappedTaskID) != "" {
		item, getErr := s.GetTaskItem(mappedTaskID)
		if getErr == nil {
			return item, nil
		}
		if !strings.Contains(getErr.Error(), "not found") {
			return nil, getErr
		}
	}

	item := &core.TaskItem{}
	var (
		labelsJSON      string
		dependsOnJSON   string
		inputsJSON      string
		outputsJSON     string
		acceptanceJSON  string
		constraintsJSON string
	)
	err = s.db.QueryRow(
		`SELECT id, plan_id, title, description, labels, depends_on, inputs, outputs, acceptance, constraints, template, COALESCE(pipeline_id, ''), COALESCE(external_id, ''), status, created_at, updated_at
		 FROM task_items WHERE pipeline_id=? LIMIT 1`,
		pipelineID,
	).Scan(
		&item.ID, &item.PlanID, &item.Title, &item.Description, &labelsJSON, &dependsOnJSON,
		&inputsJSON, &outputsJSON, &acceptanceJSON, &constraintsJSON, &item.Template,
		&item.PipelineID, &item.ExternalID, &item.Status, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalJSON(labelsJSON, &item.Labels); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(dependsOnJSON, &item.DependsOn); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(inputsJSON, &item.Inputs); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(outputsJSON, &item.Outputs); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(acceptanceJSON, &item.Acceptance); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(constraintsJSON, &item.Constraints); err != nil {
		return nil, err
	}
	return item, nil
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
		`INSERT INTO review_records (plan_id, round, reviewer, verdict, issues, fixes, score)
		 VALUES (?,?,?,?,?,?,?)`,
		record.PlanID, record.Round, record.Reviewer, record.Verdict, issuesJSON, fixesJSON, score,
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

func (s *SQLiteStore) GetReviewRecords(planID string) ([]core.ReviewRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, plan_id, round, reviewer, verdict, issues, fixes, score, created_at
		 FROM review_records
		 WHERE plan_id=?
		 ORDER BY id`,
		planID,
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
			&record.ID, &record.PlanID, &record.Round, &record.Reviewer, &record.Verdict,
			&issuesJSON, &fixesJSON, &score, &record.CreatedAt,
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

func (s *SQLiteStore) bindPipelineTaskItemLink(pipelineID, taskItemID string) error {
	pipelineID = strings.TrimSpace(pipelineID)
	taskItemID = strings.TrimSpace(taskItemID)
	if pipelineID == "" || taskItemID == "" {
		return nil
	}
	_, err := s.db.Exec(`
UPDATE pipelines
SET task_item_id = CASE
	WHEN COALESCE(task_item_id, '') = '' THEN ?
	ELSE task_item_id
END
WHERE id=?`,
		taskItemID, pipelineID,
	)
	return err
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
