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
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    repo_path      TEXT NOT NULL UNIQUE,
    github_owner   TEXT,
    github_repo    TEXT,
    default_branch TEXT DEFAULT '',
    config_json    TEXT,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS runs (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    description       TEXT,
    template          TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'queued',
    conclusion        TEXT,
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
	merge_retries     INTEGER NOT NULL DEFAULT 0,
	triage_instructions TEXT NOT NULL DEFAULT '',
	submitted_by      TEXT NOT NULL DEFAULT '',
	state             TEXT NOT NULL DEFAULT 'open',
    status            TEXT NOT NULL DEFAULT 'draft',
    run_id       TEXT,
    version           INTEGER NOT NULL DEFAULT 1,
    superseded_by     TEXT NOT NULL DEFAULT '',
    external_id       TEXT,
    fail_policy       TEXT NOT NULL DEFAULT 'block',
    children_mode     TEXT NOT NULL DEFAULT '',
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    closed_at         DATETIME
);

CREATE TABLE IF NOT EXISTS issue_attachments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    content    TEXT NOT NULL,
    source_url TEXT NOT NULL DEFAULT '',
    media_type TEXT NOT NULL DEFAULT '',
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

CREATE TABLE IF NOT EXISTS run_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    issue_id   TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    stage      TEXT NOT NULL DEFAULT '',
    agent      TEXT NOT NULL DEFAULT '',
    data_json  TEXT NOT NULL DEFAULT '{}',
    error      TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    scope        TEXT NOT NULL DEFAULT 'run',
    event_type   TEXT NOT NULL,
    project_id   TEXT NOT NULL DEFAULT '',
    run_id       TEXT NOT NULL DEFAULT '',
    issue_id     TEXT NOT NULL DEFAULT '',
    session_id   TEXT NOT NULL DEFAULT '',
    stage        TEXT NOT NULL DEFAULT '',
    agent        TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    error        TEXT NOT NULL DEFAULT '',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS issue_edges (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    from_id   TEXT NOT NULL,
    to_id     TEXT NOT NULL,
    edge_type TEXT NOT NULL DEFAULT 'depends_on',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(from_id, to_id, edge_type)
);

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
CREATE INDEX IF NOT EXISTS idx_run_events_run_created ON run_events(run_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_events_scope_project ON events(scope, project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_events_run ON events(run_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_events_issue ON events(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type, created_at);
CREATE INDEX IF NOT EXISTS idx_issue_edges_from ON issue_edges(from_id);
CREATE INDEX IF NOT EXISTS idx_issue_edges_to ON issue_edges(to_id);
CREATE INDEX IF NOT EXISTS idx_task_steps_issue ON task_steps(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_task_steps_run ON task_steps(run_id, created_at);
`

// schemaVersion tracks which migrations have been applied.
// Bump this when adding new migrations.
const schemaVersion = 14

func applyMigrations(db *sql.DB) error {
	if _, err := db.Exec(schemaTables); err != nil {
		return fmt.Errorf("exec schema tables: %w", err)
	}

	currentVersion, err := getUserVersion(db)
	if err != nil {
		return fmt.Errorf("get user_version: %w", err)
	}

	if currentVersion < 1 {
		if err := migrateStatusConclusion(db); err != nil {
			return fmt.Errorf("migrate status/conclusion: %w", err)
		}
		if _, err := db.Exec(`DROP TABLE IF EXISTS logs`); err != nil {
			return fmt.Errorf("drop logs table: %w", err)
		}
	}

	if currentVersion < 2 {
		if err := migrateAddDefaultBranch(db); err != nil {
			return fmt.Errorf("migrate default_branch: %w", err)
		}
	}

	if currentVersion < 3 {
		if err := migrateAddParentID(db); err != nil {
			return fmt.Errorf("migrate parent_id: %w", err)
		}
	}
	if currentVersion < 4 {
		if err := migrateAddMergeRetries(db); err != nil {
			return fmt.Errorf("migrate merge_retries: %w", err)
		}
	}
	if currentVersion < 5 {
		if err := migrateAddIssueTriageInstructions(db); err != nil {
			return fmt.Errorf("migrate triage_instructions: %w", err)
		}
	}
	if currentVersion < 6 {
		if err := migrateAddSubmittedBy(db); err != nil {
			return fmt.Errorf("migrate submitted_by: %w", err)
		}
		if err := migrateEventsFromLegacy(db); err != nil {
			return fmt.Errorf("migrate events from legacy: %w", err)
		}
	}
	if currentVersion < 7 {
		if err := migrateAddCheckpointAgentSessionID(db); err != nil {
			return fmt.Errorf("migrate checkpoint agent_session_id: %w", err)
		}
	}
	if currentVersion < 8 {
		if err := migrateAddAttachmentURLFields(db); err != nil {
			return fmt.Errorf("migrate attachment url fields: %w", err)
		}
	}
	if currentVersion < 9 {
		if err := migrateAddChatSessionAgentName(db); err != nil {
			return fmt.Errorf("migration v9 (chat_sessions.agent_name): %w", err)
		}
	}
	if currentVersion < 10 {
		if err := migrateAddTaskSteps(db); err != nil {
			return fmt.Errorf("migration v10 (task_steps): %w", err)
		}
	}
	if currentVersion < 11 {
		if err := migrateAddChildrenMode(db); err != nil {
			return fmt.Errorf("migration v11 (issues.children_mode): %w", err)
		}
	}
	if currentVersion < 12 {
		if err := migrateAddDecisions(db); err != nil {
			return fmt.Errorf("migration v12 (decisions): %w", err)
		}
	}
	if currentVersion < 13 {
		if err := migrateAddGateChecks(db); err != nil {
			return fmt.Errorf("migration v13 (gate_checks): %w", err)
		}
	}
	if currentVersion < 14 {
		if err := migrateHardenGateChecks(db); err != nil {
			return fmt.Errorf("migration v14 (gate_checks constraints): %w", err)
		}
	}
	if err := migrateBackfillLegacyColumns(db); err != nil {
		return err
	}
	if _, err := db.Exec(schemaIndexes); err != nil {
		return fmt.Errorf("exec schema indexes: %w", err)
	}

	if currentVersion < schemaVersion {
		if err := setUserVersion(db, schemaVersion); err != nil {
			return fmt.Errorf("set user_version: %w", err)
		}
	}
	return nil
}

func getUserVersion(db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func setUserVersion(db *sql.DB, version int) error {
	_, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version))
	return err
}

// migrateStatusConclusion adds the conclusion column (if missing) and converts
// legacy status values to the new status+conclusion model.
func migrateStatusConclusion(db *sql.DB) error {
	has, err := hasColumn(db, "runs", "conclusion")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE runs ADD COLUMN conclusion TEXT`); err != nil {
			return fmt.Errorf("add conclusion column: %w", err)
		}
	}

	migrations := []string{
		`UPDATE runs SET status='completed', conclusion='success' WHERE status='done'`,
		`UPDATE runs SET status='completed', conclusion='failure' WHERE status='failed'`,
		`UPDATE runs SET status='completed', conclusion='timed_out' WHERE status='timeout'`,
		`UPDATE runs SET status='action_required' WHERE status='waiting_review'`,
		`UPDATE runs SET status='queued' WHERE status='created'`,
		`UPDATE runs SET status='in_progress' WHERE status='running'`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migrate status: %w", err)
		}
	}
	return nil
}

func migrateAddDefaultBranch(db *sql.DB) error {
	has, err := hasColumn(db, "projects", "default_branch")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE projects ADD COLUMN default_branch TEXT DEFAULT ''`); err != nil {
			return fmt.Errorf("add default_branch column: %w", err)
		}
	}
	return nil
}

func migrateAddParentID(db *sql.DB) error {
	has, err := hasColumn(db, "issues", "parent_id")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE issues ADD COLUMN parent_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add parent_id column: %w", err)
		}
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_issues_parent ON issues(parent_id)`); err != nil {
			return fmt.Errorf("create idx_issues_parent: %w", err)
		}
	}
	return nil
}

func migrateAddMergeRetries(db *sql.DB) error {
	has, err := hasColumn(db, "issues", "merge_retries")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE issues ADD COLUMN merge_retries INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add merge_retries column: %w", err)
	}
	return nil
}

func migrateAddIssueTriageInstructions(db *sql.DB) error {
	has, err := hasColumn(db, "issues", "triage_instructions")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE issues ADD COLUMN triage_instructions TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add triage_instructions column: %w", err)
	}
	return nil
}

func migrateAddTaskSteps(db *sql.DB) error {
	_, err := db.Exec(`
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
	return err
}

func migrateAddDecisions(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS decisions (
	id               TEXT PRIMARY KEY,
	issue_id         TEXT NOT NULL,
	run_id           TEXT NOT NULL DEFAULT '',
	stage_id         TEXT NOT NULL DEFAULT '',
	agent_id         TEXT NOT NULL DEFAULT '',
	type             TEXT NOT NULL,
	prompt_hash      TEXT NOT NULL,
	prompt_preview   TEXT NOT NULL DEFAULT '',
	model            TEXT NOT NULL DEFAULT '',
	template         TEXT NOT NULL DEFAULT '',
	template_version TEXT NOT NULL DEFAULT '',
	input_tokens     INTEGER NOT NULL DEFAULT 0,
	action           TEXT NOT NULL,
	reasoning        TEXT NOT NULL DEFAULT '',
	confidence       REAL NOT NULL DEFAULT 0,
	output_tokens    INTEGER NOT NULL DEFAULT 0,
	output_data      TEXT NOT NULL DEFAULT '{}',
	duration_ms      INTEGER NOT NULL DEFAULT 0,
	created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_decisions_issue ON decisions(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_decisions_type  ON decisions(type, created_at);
`)
	return err
}

func migrateAddGateChecks(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS gate_checks (
	id          TEXT PRIMARY KEY,
	issue_id    TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	gate_name   TEXT NOT NULL,
	gate_type   TEXT NOT NULL,
	attempt     INTEGER NOT NULL DEFAULT 1,
	status      TEXT NOT NULL DEFAULT 'pending',
	reason      TEXT NOT NULL DEFAULT '',
	decision_id TEXT NOT NULL DEFAULT '',
	checked_by  TEXT NOT NULL DEFAULT '',
	created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(issue_id, gate_name, attempt)
);
CREATE INDEX IF NOT EXISTS idx_gate_checks_issue ON gate_checks(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_gate_checks_name  ON gate_checks(issue_id, gate_name);
`)
	return err
}

func migrateHardenGateChecks(db *sql.DB) error {
	exists, err := hasTable(db, "gate_checks")
	if err != nil {
		return err
	}
	if !exists {
		return migrateAddGateChecks(db)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin gate_checks hardening tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`ALTER TABLE gate_checks RENAME TO gate_checks_old`); err != nil {
		return fmt.Errorf("rename gate_checks: %w", err)
	}
	if _, err = tx.Exec(`
DROP INDEX IF EXISTS idx_gate_checks_issue;
DROP INDEX IF EXISTS idx_gate_checks_name;
`); err != nil {
		return fmt.Errorf("drop old gate_checks indexes: %w", err)
	}
	if _, err = tx.Exec(`
CREATE TABLE gate_checks (
	id          TEXT PRIMARY KEY,
	issue_id    TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	gate_name   TEXT NOT NULL,
	gate_type   TEXT NOT NULL,
	attempt     INTEGER NOT NULL DEFAULT 1,
	status      TEXT NOT NULL DEFAULT 'pending',
	reason      TEXT NOT NULL DEFAULT '',
	decision_id TEXT NOT NULL DEFAULT '',
	checked_by  TEXT NOT NULL DEFAULT '',
	created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(issue_id, gate_name, attempt)
);
CREATE INDEX idx_gate_checks_issue ON gate_checks(issue_id, created_at);
CREATE INDEX idx_gate_checks_name  ON gate_checks(issue_id, gate_name);
`); err != nil {
		return fmt.Errorf("recreate gate_checks: %w", err)
	}
	if _, err = tx.Exec(`
INSERT OR IGNORE INTO gate_checks (id, issue_id, gate_name, gate_type, attempt, status, reason, decision_id, checked_by, created_at)
SELECT old.id, old.issue_id, old.gate_name, old.gate_type, old.attempt, old.status, old.reason, old.decision_id, old.checked_by, old.created_at
FROM gate_checks_old old
WHERE EXISTS (SELECT 1 FROM issues WHERE issues.id = old.issue_id)
`); err != nil {
		return fmt.Errorf("copy hardened gate_checks rows: %w", err)
	}
	if _, err = tx.Exec(`DROP TABLE gate_checks_old`); err != nil {
		return fmt.Errorf("drop old gate_checks: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit gate_checks hardening: %w", err)
	}
	return nil
}

func migrateBackfillLegacyColumns(db *sql.DB) error {
	backfills := []struct {
		name string
		run  func(*sql.DB) error
	}{
		{name: "run_events.run_id", run: migrateAddRunEventRunID},
		{name: "chat_sessions.agent_session_id", run: migrateAddChatSessionAgentSessionID},
		{name: "issues.merge_retries", run: migrateAddMergeRetries},
		{name: "issues.triage_instructions", run: migrateAddIssueTriageInstructions},
		{name: "issues.submitted_by", run: migrateAddSubmittedBy},
		{name: "checkpoints.agent_session_id", run: migrateAddCheckpointAgentSessionID},
		{name: "issue_attachments.source_url", run: migrateAddAttachmentURLFields},
		{name: "chat_sessions.agent_name", run: migrateAddChatSessionAgentName},
		{name: "issues.children_mode", run: migrateAddChildrenMode},
	}

	for _, backfill := range backfills {
		if err := backfill.run(db); err != nil {
			return fmt.Errorf("backfill %s: %w", backfill.name, err)
		}
	}
	return nil
}

func migrateAddRunEventRunID(db *sql.DB) error {
	exists, err := hasTable(db, "run_events")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	has, err := hasColumn(db, "run_events", "run_id")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE run_events ADD COLUMN run_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add run_events.run_id column: %w", err)
	}
	return nil
}

func migrateAddChatSessionAgentSessionID(db *sql.DB) error {
	exists, err := hasTable(db, "chat_sessions")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	has, err := hasColumn(db, "chat_sessions", "agent_session_id")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE chat_sessions ADD COLUMN agent_session_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add chat_sessions.agent_session_id column: %w", err)
	}
	return nil
}

func migrateAddSubmittedBy(db *sql.DB) error {
	has, err := hasColumn(db, "issues", "submitted_by")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE issues ADD COLUMN submitted_by TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add submitted_by column: %w", err)
	}
	return nil
}

func migrateEventsFromLegacy(db *sql.DB) error {
	// Copy run_events into unified events table (scope=run).
	hasRunEvents, err := hasTable(db, "run_events")
	if err != nil {
		return err
	}
	if hasRunEvents {
		// Ensure run_id column exists before copying (may be missing on pre-v5 DBs).
		if err := migrateAddRunEventRunID(db); err != nil {
			return fmt.Errorf("ensure run_events.run_id: %w", err)
		}
		_, err := db.Exec(`
			INSERT OR IGNORE INTO events (scope, event_type, project_id, run_id, issue_id, stage, agent, payload_json, error, created_at)
			SELECT 'run', event_type, project_id, run_id, issue_id, stage, agent, data_json, error, created_at
			FROM run_events
		`)
		if err != nil {
			return fmt.Errorf("copy run_events to events: %w", err)
		}
	}

	// Copy chat_run_events into unified events table (scope=chat).
	hasChatEvents, err := hasTable(db, "chat_run_events")
	if err != nil {
		return err
	}
	if hasChatEvents {
		_, err := db.Exec(`
			INSERT OR IGNORE INTO events (scope, event_type, project_id, session_id, payload_json, created_at)
			SELECT 'chat', event_type, project_id, chat_session_id, payload_json, created_at
			FROM chat_run_events
		`)
		if err != nil {
			return fmt.Errorf("copy chat_run_events to events: %w", err)
		}
	}
	return nil
}

func migrateAddCheckpointAgentSessionID(db *sql.DB) error {
	has, err := hasColumn(db, "checkpoints", "agent_session_id")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE checkpoints ADD COLUMN agent_session_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add checkpoints.agent_session_id column: %w", err)
	}
	return nil
}

func migrateAddAttachmentURLFields(db *sql.DB) error {
	for _, col := range []struct{ name, def string }{
		{"source_url", "TEXT NOT NULL DEFAULT ''"},
		{"media_type", "TEXT NOT NULL DEFAULT ''"},
	} {
		has, err := hasColumn(db, "issue_attachments", col.name)
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE issue_attachments ADD COLUMN %s %s`, col.name, col.def)); err != nil {
				return fmt.Errorf("add issue_attachments.%s: %w", col.name, err)
			}
		}
	}
	return nil
}

func migrateAddChatSessionAgentName(db *sql.DB) error {
	has, err := hasColumn(db, "chat_sessions", "agent_name")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE chat_sessions ADD COLUMN agent_name TEXT NOT NULL DEFAULT ''`)
	return err
}

func migrateAddChildrenMode(db *sql.DB) error {
	has, err := hasColumn(db, "issues", "children_mode")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE issues ADD COLUMN children_mode TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add children_mode column: %w", err)
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
