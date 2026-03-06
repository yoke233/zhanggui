package storesqlite

import (
	"database/sql"
	"testing"
)

func TestMigration_V2Baseline_CreatesIssueRunSchema(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	tables := []string{
		"projects",
		"runs",
		"checkpoints",
		"human_actions",
		"chat_sessions",
		"chat_run_events",
		"run_events",
		"issues",
		"issue_attachments",
		"issue_changes",
		"review_records",
	}
	for _, table := range tables {
		assertTableExists(t, db, table)
	}

	assertTableNotExists(t, db, "task_plans")
	assertTableNotExists(t, db, "task_items")
	assertTableNotExists(t, db, "migration_flags")

	assertColumnExists(t, db, "runs", "issue_id")
	assertColumnExists(t, db, "runs", "run_count")
	assertColumnExists(t, db, "runs", "queued_at")
	assertColumnExists(t, db, "runs", "last_heartbeat_at")
	assertColumnNotExists(t, db, "runs", "task_item_id")

	assertColumnExists(t, db, "issues", "auto_merge")
	assertColumnExists(t, db, "issues", "merge_retries")
	assertColumnExists(t, db, "issues", "triage_instructions")
	assertColumnExists(t, db, "chat_sessions", "agent_session_id")
	assertColumnExists(t, db, "run_events", "run_id")
	assertColumnExists(t, db, "review_records", "issue_id")
	assertColumnExists(t, db, "review_records", "summary")
	assertColumnExists(t, db, "review_records", "raw_output")
	assertColumnNotExists(t, db, "review_records", "plan_id")
}

func TestMigration_V2Baseline_PersistsIssueIDLinks(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-v2-1', 'proj', '/tmp/proj-v2-1')`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO runs (id, project_id, name, template, stages_json, issue_id)
VALUES ('pipe-v2-1', 'proj-v2-1', 'pipe', 'standard', '[]', 'issue-v2-1')
`); err != nil {
		t.Fatalf("insert Run: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO review_records (issue_id, round, reviewer, verdict, summary, raw_output, issues, fixes)
VALUES ('issue-v2-1', 1, 'team-leader', 'approved', 'ok', '{}', '[]', '[]')
`); err != nil {
		t.Fatalf("insert review_record: %v", err)
	}

	var runIssueID string
	if err := db.QueryRow(`SELECT COALESCE(issue_id, '') FROM runs WHERE id='pipe-v2-1'`).Scan(&runIssueID); err != nil {
		t.Fatalf("query runs.issue_id: %v", err)
	}
	if runIssueID != "issue-v2-1" {
		t.Fatalf("expected runs.issue_id=issue-v2-1, got %q", runIssueID)
	}

	var reviewIssueID string
	if err := db.QueryRow(`SELECT issue_id FROM review_records WHERE reviewer='team-leader'`).Scan(&reviewIssueID); err != nil {
		t.Fatalf("query review_records.issue_id: %v", err)
	}
	if reviewIssueID != "issue-v2-1" {
		t.Fatalf("expected review_records.issue_id=issue-v2-1, got %q", reviewIssueID)
	}
}

func TestMigration_V2Baseline_CreatesIndexes(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	indexes := []string{
		"idx_runs_project",
		"idx_runs_status",
		"idx_runs_status_queued_at",
		"idx_runs_project_status",
		"idx_issues_project",
		"idx_issues_project_status",
		"idx_issues_session",
		"idx_issues_run",
		"idx_issue_attachments_issue",
		"idx_issue_changes_issue",
		"idx_review_records_issue",
	}
	for _, index := range indexes {
		assertIndexExists(t, db, index)
	}
}

func TestMigration_AddsIssueMergeRetriesFromV3(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS issues (
	id                TEXT PRIMARY KEY,
	project_id        TEXT NOT NULL,
	session_id        TEXT,
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
	run_id            TEXT,
	version           INTEGER NOT NULL DEFAULT 1,
	superseded_by     TEXT NOT NULL DEFAULT '',
	external_id       TEXT,
	fail_policy       TEXT NOT NULL DEFAULT 'block',
	parent_id         TEXT NOT NULL DEFAULT '',
	created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
	closed_at         DATETIME
)`); err != nil {
		t.Fatalf("create legacy issues table: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 3`); err != nil {
		t.Fatalf("set user_version=3: %v", err)
	}

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	assertColumnExists(t, db, "issues", "merge_retries")
	assertColumnExists(t, db, "issues", "triage_instructions")

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("user_version=%d, want %d", version, schemaVersion)
	}
}

func TestMigration_BackfillsLegacyColumnsEvenWhenVersionAlreadyCurrent(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS chat_sessions (
	id          TEXT PRIMARY KEY,
	project_id  TEXT NOT NULL,
	messages    TEXT NOT NULL DEFAULT '[]',
	created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		t.Fatalf("create legacy chat_sessions table: %v", err)
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS run_events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id TEXT NOT NULL DEFAULT '',
	issue_id   TEXT NOT NULL DEFAULT '',
	event_type TEXT NOT NULL,
	stage      TEXT NOT NULL DEFAULT '',
	agent      TEXT NOT NULL DEFAULT '',
	data_json  TEXT NOT NULL DEFAULT '{}',
	error      TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		t.Fatalf("create legacy run_events table: %v", err)
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS issues (
	id                TEXT PRIMARY KEY,
	project_id        TEXT NOT NULL,
	session_id        TEXT,
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
	run_id            TEXT,
	version           INTEGER NOT NULL DEFAULT 1,
	superseded_by     TEXT NOT NULL DEFAULT '',
	external_id       TEXT,
	fail_policy       TEXT NOT NULL DEFAULT 'block',
	parent_id         TEXT NOT NULL DEFAULT '',
	created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
	closed_at         DATETIME
)`); err != nil {
		t.Fatalf("create legacy issues table: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 5`); err != nil {
		t.Fatalf("set user_version=5: %v", err)
	}

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	assertColumnExists(t, db, "chat_sessions", "agent_session_id")
	assertColumnExists(t, db, "run_events", "run_id")
	assertColumnExists(t, db, "issues", "merge_retries")
	assertColumnExists(t, db, "issues", "triage_instructions")
	assertIndexExists(t, db, "idx_run_events_run_created")
}

func assertColumnExists(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()

	ok, err := hasColumn(db, table, column)
	if err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	if !ok {
		t.Fatalf("expected column %s.%s to exist", table, column)
	}
}

func TestMigration_V6ColdStart_CreatesUnifiedEventsAndIssueEdges(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// Unified events table
	assertTableExists(t, db, "events")
	assertColumnExists(t, db, "events", "scope")
	assertColumnExists(t, db, "events", "event_type")
	assertColumnExists(t, db, "events", "project_id")
	assertColumnExists(t, db, "events", "run_id")
	assertColumnExists(t, db, "events", "issue_id")
	assertColumnExists(t, db, "events", "session_id")
	assertColumnExists(t, db, "events", "payload_json")

	// issue_edges table
	assertTableExists(t, db, "issue_edges")
	assertColumnExists(t, db, "issue_edges", "from_id")
	assertColumnExists(t, db, "issue_edges", "to_id")
	assertColumnExists(t, db, "issue_edges", "edge_type")

	// submitted_by on issues
	assertColumnExists(t, db, "issues", "submitted_by")

	// V6 indexes
	assertIndexExists(t, db, "idx_events_scope_project")
	assertIndexExists(t, db, "idx_events_run")
	assertIndexExists(t, db, "idx_events_issue")
	assertIndexExists(t, db, "idx_events_session")
	assertIndexExists(t, db, "idx_events_type")
	assertIndexExists(t, db, "idx_issue_edges_from")
	assertIndexExists(t, db, "idx_issue_edges_to")

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("user_version=%d, want %d", version, schemaVersion)
	}
}

func TestMigration_V5ToV6_MigratesLegacyEventsToUnified(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	// Create v5 schema: legacy run_events and chat_run_events
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY, name TEXT NOT NULL, repo_path TEXT NOT NULL UNIQUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS runs (
	id TEXT PRIMARY KEY, project_id TEXT NOT NULL, name TEXT NOT NULL, template TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'queued', stages_json TEXT NOT NULL, issue_id TEXT,
	queued_at DATETIME, started_at DATETIME, finished_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS chat_sessions (
	id TEXT PRIMARY KEY, project_id TEXT NOT NULL, messages TEXT NOT NULL DEFAULT '[]',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS run_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id TEXT NOT NULL DEFAULT '', project_id TEXT NOT NULL DEFAULT '', issue_id TEXT NOT NULL DEFAULT '',
	event_type TEXT NOT NULL, stage TEXT NOT NULL DEFAULT '', agent TEXT NOT NULL DEFAULT '',
	data_json TEXT NOT NULL DEFAULT '{}', error TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS chat_run_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_session_id TEXT NOT NULL, project_id TEXT NOT NULL,
	event_type TEXT NOT NULL, update_type TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '{}',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS issues (
	id TEXT PRIMARY KEY, project_id TEXT NOT NULL, session_id TEXT,
	title TEXT NOT NULL, body TEXT NOT NULL DEFAULT '', labels TEXT NOT NULL DEFAULT '[]',
	milestone_id TEXT NOT NULL DEFAULT '', attachments TEXT NOT NULL DEFAULT '[]',
	depends_on TEXT NOT NULL DEFAULT '[]', blocks TEXT NOT NULL DEFAULT '[]',
	priority INTEGER NOT NULL DEFAULT 0, template TEXT NOT NULL DEFAULT 'standard',
	auto_merge INTEGER NOT NULL DEFAULT 1, merge_retries INTEGER NOT NULL DEFAULT 0,
	triage_instructions TEXT NOT NULL DEFAULT '',
	state TEXT NOT NULL DEFAULT 'open', status TEXT NOT NULL DEFAULT 'draft',
	run_id TEXT, version INTEGER NOT NULL DEFAULT 1, superseded_by TEXT NOT NULL DEFAULT '',
	external_id TEXT, fail_policy TEXT NOT NULL DEFAULT 'block', parent_id TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, closed_at DATETIME
)`); err != nil {
		t.Fatalf("create v5 schema: %v", err)
	}

	// Seed data
	if _, err := db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('p1', 'test', '/tmp/p1')`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO chat_sessions (id, project_id) VALUES ('cs1', 'p1')`); err != nil {
		t.Fatalf("seed chat_session: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO run_events (run_id, project_id, issue_id, event_type, stage, agent, data_json, error)
		VALUES ('run-1', 'p1', 'issue-1', 'stage_started', 'build', 'claude', '{"msg":"hello"}', '')
	`); err != nil {
		t.Fatalf("seed run_event: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO chat_run_events (chat_session_id, project_id, event_type, payload_json)
		VALUES ('cs1', 'p1', 'chat_update', '{"text":"world"}')
	`); err != nil {
		t.Fatalf("seed chat_run_event: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 5`); err != nil {
		t.Fatalf("set user_version=5: %v", err)
	}

	// Run migrations
	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// Verify unified events table has data from both sources
	var runEventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE scope='run'`).Scan(&runEventCount); err != nil {
		t.Fatalf("count run events: %v", err)
	}
	if runEventCount != 1 {
		t.Fatalf("expected 1 run event, got %d", runEventCount)
	}

	var chatEventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE scope='chat'`).Scan(&chatEventCount); err != nil {
		t.Fatalf("count chat events: %v", err)
	}
	if chatEventCount != 1 {
		t.Fatalf("expected 1 chat event, got %d", chatEventCount)
	}

	// Verify run event fields
	var scope, eventType, projectID, runID, issueID, payload string
	if err := db.QueryRow(`SELECT scope, event_type, project_id, run_id, issue_id, payload_json FROM events WHERE scope='run'`).
		Scan(&scope, &eventType, &projectID, &runID, &issueID, &payload); err != nil {
		t.Fatalf("scan run event: %v", err)
	}
	if eventType != "stage_started" || projectID != "p1" || runID != "run-1" || issueID != "issue-1" {
		t.Fatalf("unexpected run event: type=%q proj=%q run=%q issue=%q", eventType, projectID, runID, issueID)
	}
	if payload != `{"msg":"hello"}` {
		t.Fatalf("unexpected payload: %q", payload)
	}

	// Verify chat event fields
	var chatScope, chatEventType, chatProjectID, sessionID, chatPayload string
	if err := db.QueryRow(`SELECT scope, event_type, project_id, session_id, payload_json FROM events WHERE scope='chat'`).
		Scan(&chatScope, &chatEventType, &chatProjectID, &sessionID, &chatPayload); err != nil {
		t.Fatalf("scan chat event: %v", err)
	}
	if chatEventType != "chat_update" || chatProjectID != "p1" || sessionID != "cs1" {
		t.Fatalf("unexpected chat event: type=%q proj=%q session=%q", chatEventType, chatProjectID, sessionID)
	}

	// Verify submitted_by was added
	assertColumnExists(t, db, "issues", "submitted_by")

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("user_version=%d, want %d", version, schemaVersion)
	}
}

func TestMigration_V6_IssueEdgesUniqueConstraint(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// Insert an edge
	if _, err := db.Exec(`INSERT INTO issue_edges (from_id, to_id, edge_type) VALUES ('a', 'b', 'depends_on')`); err != nil {
		t.Fatalf("insert edge: %v", err)
	}

	// Duplicate should fail
	_, err := db.Exec(`INSERT INTO issue_edges (from_id, to_id, edge_type) VALUES ('a', 'b', 'depends_on')`)
	if err == nil {
		t.Fatal("expected UNIQUE constraint violation for duplicate edge")
	}

	// Different edge_type should succeed
	if _, err := db.Exec(`INSERT INTO issue_edges (from_id, to_id, edge_type) VALUES ('a', 'b', 'blocks')`); err != nil {
		t.Fatalf("insert different edge_type: %v", err)
	}
}

func TestMigration_BackfillIncludesSubmittedBy(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	// Create issues table WITHOUT submitted_by, at version 6 (triggers backfill path)
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS issues (
	id TEXT PRIMARY KEY, project_id TEXT NOT NULL, session_id TEXT,
	title TEXT NOT NULL, body TEXT NOT NULL DEFAULT '',
	labels TEXT NOT NULL DEFAULT '[]', milestone_id TEXT NOT NULL DEFAULT '',
	attachments TEXT NOT NULL DEFAULT '[]', depends_on TEXT NOT NULL DEFAULT '[]',
	blocks TEXT NOT NULL DEFAULT '[]', priority INTEGER NOT NULL DEFAULT 0,
	template TEXT NOT NULL DEFAULT 'standard', auto_merge INTEGER NOT NULL DEFAULT 1,
	merge_retries INTEGER NOT NULL DEFAULT 0, triage_instructions TEXT NOT NULL DEFAULT '',
	state TEXT NOT NULL DEFAULT 'open', status TEXT NOT NULL DEFAULT 'draft',
	run_id TEXT, version INTEGER NOT NULL DEFAULT 1, superseded_by TEXT NOT NULL DEFAULT '',
	external_id TEXT, fail_policy TEXT NOT NULL DEFAULT 'block', parent_id TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, closed_at DATETIME
)`); err != nil {
		t.Fatalf("create issues without submitted_by: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 6`); err != nil {
		t.Fatalf("set user_version=6: %v", err)
	}

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	assertColumnExists(t, db, "issues", "submitted_by")
}

func assertColumnNotExists(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()

	ok, err := hasColumn(db, table, column)
	if err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	if ok {
		t.Fatalf("expected column %s.%s to be absent", table, column)
	}
}
