package storesqlite

import (
	"database/sql"
	"fmt"
)

const schemaTables = `
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS projects (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    repo_path    TEXT NOT NULL UNIQUE,
    github_owner TEXT,
    github_repo  TEXT,
    config_json  TEXT,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS pipelines (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    description       TEXT,
    template          TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'created',
    current_stage     TEXT,
    stages_json       TEXT NOT NULL,
    artifacts_json    TEXT DEFAULT '{}',
    config_json       TEXT DEFAULT '{}',
    issue_number      INTEGER,
    pr_number         INTEGER,
    branch_name       TEXT,
    worktree_path     TEXT,
    error_message     TEXT,
    max_total_retries INTEGER DEFAULT 5,
    total_retries     INTEGER DEFAULT 0,
    run_count         INTEGER DEFAULT 0,
    last_error_type   TEXT,
    task_item_id      TEXT,
    queued_at         DATETIME,
    last_heartbeat_at DATETIME,
    started_at        DATETIME,
    finished_at       DATETIME,
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS checkpoints (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id    TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    stage          TEXT NOT NULL,
    status         TEXT NOT NULL,
    agent_used     TEXT,
    artifacts_json TEXT DEFAULT '{}',
    tokens_used    INTEGER DEFAULT 0,
    retry_count    INTEGER DEFAULT 0,
    error_message  TEXT,
    started_at     DATETIME NOT NULL,
    finished_at    DATETIME,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_checkpoints_pipeline ON checkpoints(pipeline_id);

CREATE TABLE IF NOT EXISTS logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    stage       TEXT NOT NULL,
    type        TEXT NOT NULL,
    agent       TEXT,
    content     TEXT NOT NULL,
    timestamp   DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_logs_pipeline_stage ON logs(pipeline_id, stage);
CREATE INDEX IF NOT EXISTS idx_logs_id ON logs(id);

CREATE TABLE IF NOT EXISTS human_actions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    stage       TEXT NOT NULL,
    action      TEXT NOT NULL,
    message     TEXT,
    source      TEXT NOT NULL,
    user_id     TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_human_actions_pipeline ON human_actions(pipeline_id);

CREATE TABLE IF NOT EXISTS chat_sessions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    agent_session_id TEXT NOT NULL DEFAULT '',
    messages    TEXT NOT NULL DEFAULT '[]',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chat_sessions_project ON chat_sessions(project_id);

CREATE TABLE IF NOT EXISTS task_plans (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id        TEXT REFERENCES chat_sessions(id) ON DELETE SET NULL,
    name              TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'draft',
    wait_reason       TEXT NOT NULL DEFAULT '',
    fail_policy       TEXT NOT NULL DEFAULT 'block',
    review_round      INTEGER DEFAULT 0,
    spec_profile      TEXT NOT NULL DEFAULT '',
    contract_version  TEXT NOT NULL DEFAULT '',
    contract_checksum TEXT NOT NULL DEFAULT '',
    source_files      TEXT NOT NULL DEFAULT '[]',
    file_contents     TEXT NOT NULL DEFAULT '{}',
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_task_plans_project ON task_plans(project_id);
CREATE INDEX IF NOT EXISTS idx_task_plans_status ON task_plans(status);

CREATE TABLE IF NOT EXISTS task_items (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES task_plans(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    labels      TEXT DEFAULT '[]',
    depends_on  TEXT DEFAULT '[]',
    inputs      TEXT DEFAULT '[]',
    outputs     TEXT DEFAULT '[]',
    acceptance  TEXT DEFAULT '[]',
    constraints TEXT DEFAULT '[]',
    template    TEXT NOT NULL DEFAULT 'standard',
    pipeline_id TEXT REFERENCES pipelines(id) ON DELETE SET NULL,
    external_id TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_task_items_plan ON task_items(plan_id);
CREATE INDEX IF NOT EXISTS idx_task_items_status ON task_items(status);

CREATE TABLE IF NOT EXISTS review_records (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id    TEXT NOT NULL REFERENCES task_plans(id) ON DELETE CASCADE,
    round      INTEGER NOT NULL,
    reviewer   TEXT NOT NULL,
    verdict    TEXT NOT NULL,
    issues     TEXT DEFAULT '[]',
    fixes      TEXT DEFAULT '[]',
    score      INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_review_records_plan ON review_records(plan_id);
`

const schemaIndexes = `
CREATE INDEX IF NOT EXISTS idx_pipelines_project ON pipelines(project_id);
CREATE INDEX IF NOT EXISTS idx_pipelines_status ON pipelines(status);
CREATE INDEX IF NOT EXISTS idx_pipelines_status_queued_at ON pipelines(status, queued_at, created_at);
CREATE INDEX IF NOT EXISTS idx_pipelines_project_status ON pipelines(project_id, status);
`

func applyMigrations(db *sql.DB) error {
	if _, err := db.Exec(schemaTables); err != nil {
		return fmt.Errorf("exec schema tables: %w", err)
	}

	// Keep older local sqlite files backward-compatible when new columns are introduced.
	if err := ensureColumns(db, "pipelines", map[string]string{
		"run_count":         "run_count INTEGER DEFAULT 0",
		"last_error_type":   "last_error_type TEXT",
		"queued_at":         "queued_at DATETIME",
		"last_heartbeat_at": "last_heartbeat_at DATETIME",
		"task_item_id":      "task_item_id TEXT",
	}); err != nil {
		return err
	}
	if err := ensureColumns(db, "task_plans", map[string]string{
		"spec_profile":      "spec_profile TEXT NOT NULL DEFAULT ''",
		"contract_version":  "contract_version TEXT NOT NULL DEFAULT ''",
		"contract_checksum": "contract_checksum TEXT NOT NULL DEFAULT ''",
		"source_files":      "source_files TEXT NOT NULL DEFAULT '[]'",
		"file_contents":     "file_contents TEXT NOT NULL DEFAULT '{}'",
	}); err != nil {
		return err
	}
	if err := ensureColumns(db, "task_items", map[string]string{
		"inputs":      "inputs TEXT DEFAULT '[]'",
		"outputs":     "outputs TEXT DEFAULT '[]'",
		"acceptance":  "acceptance TEXT DEFAULT '[]'",
		"constraints": "constraints TEXT DEFAULT '[]'",
	}); err != nil {
		return err
	}
	if err := ensureColumns(db, "chat_sessions", map[string]string{
		"agent_session_id": "agent_session_id TEXT NOT NULL DEFAULT ''",
	}); err != nil {
		return err
	}
	if err := applyWave2Cutover(db); err != nil {
		return err
	}
	if err := backfillPipelineTaskItemID(db); err != nil {
		return err
	}

	// Create indexes after backward-compatible column adds.
	// Otherwise, older DB files missing new columns (e.g. pipelines.queued_at) will fail when creating indexes.
	if _, err := db.Exec(schemaIndexes); err != nil {
		return fmt.Errorf("exec schema indexes: %w", err)
	}
	return nil
}

func ensureColumns(db *sql.DB, table string, columns map[string]string) error {
	for column, ddl := range columns {
		exists, err := hasColumn(db, table, column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + ddl); err != nil {
			return fmt.Errorf("add %s.%s: %w", table, column, err)
		}
	}
	return nil
}

func backfillPipelineTaskItemID(db *sql.DB) error {
	_, err := db.Exec(`
UPDATE pipelines
SET task_item_id = (
	SELECT ti.id
	FROM task_items ti
	WHERE ti.pipeline_id = pipelines.id
	ORDER BY ti.created_at ASC, ti.id ASC
	LIMIT 1
)
WHERE COALESCE(task_item_id, '') = ''
  AND EXISTS (SELECT 1 FROM task_items ti2 WHERE ti2.pipeline_id = pipelines.id)
`)
	if err != nil {
		return fmt.Errorf("backfill pipelines.task_item_id: %w", err)
	}
	return nil
}

func applyWave2Cutover(db *sql.DB) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS migration_flags (
	flag_key   TEXT PRIMARY KEY,
	flag_value TEXT NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		return fmt.Errorf("ensure migration_flags: %w", err)
	}

	var done int
	if err := db.QueryRow(`SELECT COUNT(*) FROM migration_flags WHERE flag_key='wave2_destructive_cutover_done' AND flag_value='1'`).Scan(&done); err != nil {
		return fmt.Errorf("query wave2 cutover flag: %w", err)
	}
	if done > 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin wave2 cutover: %w", err)
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec(`
DELETE FROM task_items
WHERE lower(trim(status)) <> 'done'
`); err != nil {
		return fmt.Errorf("purge legacy task_items: %w", err)
	}

	if _, err := tx.Exec(`
DELETE FROM task_plans
WHERE lower(trim(status)) <> 'done'
`); err != nil {
		return fmt.Errorf("purge legacy task_plans: %w", err)
	}

	if _, err := tx.Exec(`
DELETE FROM pipelines
WHERE lower(trim(status)) <> 'done'
`); err != nil {
		return fmt.Errorf("purge legacy pipelines: %w", err)
	}

	if _, err := tx.Exec(`
UPDATE task_items
SET pipeline_id=NULL
WHERE pipeline_id IS NOT NULL
  AND trim(pipeline_id) <> ''
  AND pipeline_id NOT IN (SELECT id FROM pipelines WHERE lower(trim(status))='done')
`); err != nil {
		return fmt.Errorf("reset dangling task_items.pipeline_id: %w", err)
	}

	if _, err := tx.Exec(`
INSERT INTO migration_flags(flag_key, flag_value)
VALUES ('wave2_destructive_cutover_done', '1')
ON CONFLICT(flag_key) DO UPDATE SET
	flag_value='1',
	updated_at=CURRENT_TIMESTAMP
`); err != nil {
		return fmt.Errorf("set wave2 cutover flag: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit wave2 cutover: %w", err)
	}
	rollback = false
	return nil
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", table)
	rows, err := db.Query(query)
	if err != nil {
		return false, fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dfltValue, &pk); err != nil {
			return false, fmt.Errorf("scan table_info(%s): %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate table_info(%s): %w", table, err)
	}
	return false, nil
}
