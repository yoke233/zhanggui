package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const schemaV1 = `
CREATE TABLE IF NOT EXISTS projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'general',
    description TEXT NOT NULL DEFAULT '',
    metadata TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS resource_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id),
    kind TEXT NOT NULL,
    uri TEXT NOT NULL,
    config TEXT,
    label TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_rb_project ON resource_bindings(project_id);

CREATE TABLE IF NOT EXISTS flows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    parent_step_id INTEGER,
    metadata TEXT,
    archived_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    flow_id INTEGER NOT NULL REFERENCES flows(id),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    depends_on TEXT,
    sub_flow_id INTEGER,
    agent_role TEXT,
    required_capabilities TEXT,
    acceptance_criteria TEXT,
    timeout_ms INTEGER DEFAULT 0,
    config TEXT,
    max_retries INTEGER DEFAULT 0,
    retry_count INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    step_id INTEGER NOT NULL REFERENCES steps(id),
    flow_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'created',
    agent_id TEXT,
    agent_context_id INTEGER,
    briefing_snapshot TEXT,
    artifact_id INTEGER,
    input TEXT,
    output TEXT,
    error_message TEXT,
    error_kind TEXT,
    attempt INTEGER DEFAULT 1,
    started_at DATETIME,
    finished_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS artifacts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    execution_id INTEGER NOT NULL REFERENCES executions(id),
    step_id INTEGER NOT NULL,
    flow_id INTEGER NOT NULL,
    result_markdown TEXT NOT NULL,
    metadata TEXT,
    assets TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS briefings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    step_id INTEGER NOT NULL REFERENCES steps(id),
    objective TEXT NOT NULL,
    context_refs TEXT,
    constraints TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_contexts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT NOT NULL,
    flow_id INTEGER NOT NULL,
    system_prompt TEXT,
    session_id TEXT,
    summary TEXT,
    turn_count INTEGER DEFAULT 0,
    worker_id TEXT NOT NULL DEFAULT '',
    worker_last_seen_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    flow_id INTEGER,
    step_id INTEGER,
    exec_id INTEGER,
    data TEXT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS execution_probes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    execution_id INTEGER NOT NULL REFERENCES executions(id),
    flow_id INTEGER NOT NULL,
    step_id INTEGER NOT NULL,
    agent_context_id INTEGER,
    session_id TEXT NOT NULL DEFAULT '',
    owner_id TEXT NOT NULL DEFAULT '',
    trigger_source TEXT NOT NULL,
    question TEXT NOT NULL,
    status TEXT NOT NULL,
    verdict TEXT NOT NULL,
    reply_text TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    sent_at DATETIME,
    answered_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_drivers (
    id TEXT PRIMARY KEY,
    launch_command TEXT NOT NULL,
    launch_args TEXT,
    env TEXT,
    cap_fs_read INTEGER NOT NULL DEFAULT 0,
    cap_fs_write INTEGER NOT NULL DEFAULT 0,
    cap_terminal INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

 CREATE TABLE IF NOT EXISTS agent_profiles (
     id TEXT PRIMARY KEY,
     name TEXT NOT NULL DEFAULT '',
     driver_id TEXT NOT NULL REFERENCES agent_drivers(id),
     role TEXT NOT NULL,
     capabilities TEXT,
     actions_allowed TEXT,
     prompt_template TEXT NOT NULL DEFAULT '',
     skills TEXT,
     session_reuse INTEGER NOT NULL DEFAULT 0,
     session_max_turns INTEGER NOT NULL DEFAULT 0,
     session_idle_ttl_ms INTEGER NOT NULL DEFAULT 0,
     mcp_enabled INTEGER NOT NULL DEFAULT 0,
     mcp_tools TEXT,
     created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
     updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS issues (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER,
    title       TEXT    NOT NULL,
    body        TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL DEFAULT 'open',
    priority    TEXT    NOT NULL DEFAULT 'medium',
    labels      TEXT,
    flow_id     INTEGER,
    metadata    TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_issues_project ON issues(project_id);
CREATE INDEX IF NOT EXISTS idx_issues_status  ON issues(status);
CREATE INDEX IF NOT EXISTS idx_issues_flow    ON issues(flow_id);

CREATE INDEX IF NOT EXISTS idx_steps_flow ON steps(flow_id);
CREATE INDEX IF NOT EXISTS idx_executions_step ON executions(step_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_exec ON artifacts(execution_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_step ON artifacts(step_id);
CREATE INDEX IF NOT EXISTS idx_briefings_step ON briefings(step_id);
CREATE INDEX IF NOT EXISTS idx_events_flow ON events(flow_id);
CREATE INDEX IF NOT EXISTS idx_agent_contexts_lookup ON agent_contexts(agent_id, flow_id);
CREATE INDEX IF NOT EXISTS idx_flows_project ON flows(project_id);
CREATE INDEX IF NOT EXISTS idx_execution_probes_execution ON execution_probes(execution_id, id);
CREATE INDEX IF NOT EXISTS idx_execution_probes_active ON execution_probes(execution_id, status, id);
`

func runMigrations(db *sql.DB) error {
	if db == nil {
		return errors.New("nil db")
	}

	if _, err := db.Exec(schemaV1); err != nil {
		return fmt.Errorf("run base schema: %w", err)
	}

	// Backward-compatible upgrades for existing DBs that were created before columns/tables existed.
	// SQLite doesn't support ADD COLUMN IF NOT EXISTS in older versions; ignore duplicate-column errors.
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS projects (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            kind TEXT NOT NULL DEFAULT 'general',
            description TEXT NOT NULL DEFAULT '',
            metadata TEXT,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`ALTER TABLE flows ADD COLUMN project_id INTEGER`,
		`ALTER TABLE flows ADD COLUMN archived_at DATETIME`,
		`CREATE INDEX IF NOT EXISTS idx_flows_project ON flows(project_id)`,
		// Upgrade old projects table: add new columns (ignore if already present).
		`ALTER TABLE projects ADD COLUMN kind TEXT NOT NULL DEFAULT 'general'`,
		`ALTER TABLE projects ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE projects ADD COLUMN metadata TEXT`,
		// resource_bindings table is handled by CREATE TABLE IF NOT EXISTS in schemaV1.
		// Add description column to steps (runtime schema upgrade).
		`ALTER TABLE steps ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		// Add skills column to agent_profiles (runtime profile skills).
		`ALTER TABLE agent_profiles ADD COLUMN skills TEXT`,
		`ALTER TABLE agent_contexts ADD COLUMN worker_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agent_contexts ADD COLUMN worker_last_seen_at DATETIME`,
		`CREATE TABLE IF NOT EXISTS execution_probes (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            execution_id INTEGER NOT NULL REFERENCES executions(id),
            flow_id INTEGER NOT NULL,
            step_id INTEGER NOT NULL,
            agent_context_id INTEGER,
            session_id TEXT NOT NULL DEFAULT '',
            owner_id TEXT NOT NULL DEFAULT '',
            trigger_source TEXT NOT NULL,
            question TEXT NOT NULL,
            status TEXT NOT NULL,
            verdict TEXT NOT NULL,
            reply_text TEXT NOT NULL DEFAULT '',
            error TEXT NOT NULL DEFAULT '',
            sent_at DATETIME,
            answered_at DATETIME,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_execution_probes_execution ON execution_probes(execution_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_probes_active ON execution_probes(execution_id, status, id)`,
		// issues table (runtime schema upgrade for existing DBs).
		`CREATE TABLE IF NOT EXISTS issues (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            project_id  INTEGER,
            title       TEXT    NOT NULL,
            body        TEXT    NOT NULL DEFAULT '',
            status      TEXT    NOT NULL DEFAULT 'open',
            priority    TEXT    NOT NULL DEFAULT 'medium',
            labels      TEXT,
            flow_id     INTEGER,
            metadata    TEXT,
            created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_issues_project ON issues(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_issues_status ON issues(status)`,
		`CREATE INDEX IF NOT EXISTS idx_issues_flow ON issues(flow_id)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			// Some SQLite builds return a generic error string for duplicate columns.
			if strings.Contains(err.Error(), "already exists") && strings.Contains(stmt, "ALTER TABLE") {
				continue
			}
			return fmt.Errorf("migration failed: %s: %w", stmt, err)
		}
	}

	return nil
}

