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

CREATE TABLE IF NOT EXISTS runs (
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
    issue_id          TEXT,
    queued_at         DATETIME,
    last_heartbeat_at DATETIME,
    started_at        DATETIME,
    finished_at       DATETIME,
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS checkpoints (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id    TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
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

CREATE INDEX IF NOT EXISTS idx_checkpoints_run ON checkpoints(run_id);

CREATE TABLE IF NOT EXISTS logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    stage       TEXT NOT NULL,
    type        TEXT NOT NULL,
    agent       TEXT,
    content     TEXT NOT NULL,
    timestamp   DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_logs_run_stage ON logs(run_id, stage);
CREATE INDEX IF NOT EXISTS idx_logs_id ON logs(id);

CREATE TABLE IF NOT EXISTS human_actions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    stage       TEXT NOT NULL,
    action      TEXT NOT NULL,
    message     TEXT,
    source      TEXT NOT NULL,
    user_id     TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_human_actions_run ON human_actions(run_id);

CREATE TABLE IF NOT EXISTS chat_sessions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    agent_session_id TEXT NOT NULL DEFAULT '',
    messages    TEXT NOT NULL DEFAULT '[]',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chat_sessions_project ON chat_sessions(project_id);

CREATE TABLE IF NOT EXISTS chat_run_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event_type      TEXT NOT NULL,
    update_type     TEXT NOT NULL DEFAULT '',
    payload_json    TEXT NOT NULL DEFAULT '{}',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chat_run_events_session_created
ON chat_run_events(chat_session_id, created_at, id);

CREATE TABLE IF NOT EXISTS issues (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id        TEXT REFERENCES chat_sessions(id) ON DELETE SET NULL,
    title             TEXT NOT NULL,
    body              TEXT NOT NULL DEFAULT '',
    labels            TEXT NOT NULL DEFAULT '[]',
    milestone_id      TEXT NOT NULL DEFAULT '',
    attachments       TEXT NOT NULL DEFAULT '[]',
    depends_on        TEXT NOT NULL DEFAULT '[]',
    blocks            TEXT NOT NULL DEFAULT '[]',
    priority          INTEGER NOT NULL DEFAULT 0,
    template          TEXT NOT NULL DEFAULT 'standard',
    auto_merge        INTEGER NOT NULL DEFAULT 1,
    state             TEXT NOT NULL DEFAULT 'open',
    status            TEXT NOT NULL DEFAULT 'draft',
    run_id       TEXT,
    version           INTEGER NOT NULL DEFAULT 1,
    superseded_by     TEXT NOT NULL DEFAULT '',
    external_id       TEXT,
    fail_policy       TEXT NOT NULL DEFAULT 'block',
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    closed_at         DATETIME
);

CREATE TABLE IF NOT EXISTS issue_attachments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS issue_changes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    field      TEXT NOT NULL,
    old_value  TEXT,
    new_value  TEXT,
    reason     TEXT NOT NULL DEFAULT '',
    changed_by TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS review_records (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   TEXT NOT NULL,
    round      INTEGER NOT NULL,
    reviewer   TEXT NOT NULL,
    verdict    TEXT NOT NULL,
    summary    TEXT NOT NULL DEFAULT '',
    raw_output TEXT NOT NULL DEFAULT '',
    issues     TEXT DEFAULT '[]',
    fixes      TEXT DEFAULT '[]',
    score      INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const schemaIndexes = `
CREATE INDEX IF NOT EXISTS idx_runs_project ON runs(project_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_status_queued_at ON runs(status, queued_at, created_at);
CREATE INDEX IF NOT EXISTS idx_runs_project_status ON runs(project_id, status);
CREATE INDEX IF NOT EXISTS idx_issues_project ON issues(project_id);
CREATE INDEX IF NOT EXISTS idx_issues_project_status ON issues(project_id, status);
CREATE INDEX IF NOT EXISTS idx_issues_session ON issues(session_id);
CREATE INDEX IF NOT EXISTS idx_issues_run ON issues(run_id);
CREATE INDEX IF NOT EXISTS idx_issue_attachments_issue ON issue_attachments(issue_id);
CREATE INDEX IF NOT EXISTS idx_issue_changes_issue ON issue_changes(issue_id);
CREATE INDEX IF NOT EXISTS idx_review_records_issue ON review_records(issue_id);
`

func applyMigrations(db *sql.DB) error {
	if _, err := db.Exec(schemaTables); err != nil {
		return fmt.Errorf("exec schema tables: %w", err)
	}
	if _, err := db.Exec(schemaIndexes); err != nil {
		return fmt.Errorf("exec schema indexes: %w", err)
	}
	return nil
}

func hasTable(db *sql.DB, table string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
		return false, fmt.Errorf("check table %s: %w", table, err)
	}
	return count > 0, nil
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
