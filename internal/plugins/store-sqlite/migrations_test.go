package storesqlite

import (
	"database/sql"
	"testing"
)

func TestMigration_AddsTaskContractColumns_BackwardCompatible(t *testing.T) {
	db := openLegacySQLite(t)
	defer db.Close()

	if err := seedLegacySchema(db); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-mig-1', 'proj', '/tmp/proj-mig-1')`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_plans (id, project_id, name, status) VALUES ('plan-mig-done', 'proj-mig-1', 'done plan', 'done')`); err != nil {
		t.Fatalf("insert task_plan: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_items (id, plan_id, title, description, status) VALUES ('task-mig-done', 'plan-mig-done', 'task', 'desc', 'done')`); err != nil {
		t.Fatalf("insert task_item: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO pipelines (id, project_id, name, template, status) VALUES ('pipe-mig-done', 'proj-mig-1', 'pipe', 'standard', 'done')`); err != nil {
		t.Fatalf("insert pipeline: %v", err)
	}

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	assertColumnExists(t, db, "pipelines", "task_item_id")
	assertColumnExists(t, db, "task_plans", "spec_profile")
	assertColumnExists(t, db, "task_plans", "contract_version")
	assertColumnExists(t, db, "task_plans", "contract_checksum")
	assertColumnExists(t, db, "task_plans", "source_files")
	assertColumnExists(t, db, "task_plans", "file_contents")
	assertColumnExists(t, db, "task_items", "inputs")
	assertColumnExists(t, db, "task_items", "outputs")
	assertColumnExists(t, db, "task_items", "acceptance")
	assertColumnExists(t, db, "task_items", "constraints")

	var plans int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_plans WHERE id='plan-mig-done'`).Scan(&plans); err != nil {
		t.Fatalf("count migrated task_plan: %v", err)
	}
	if plans != 1 {
		t.Fatalf("expected migrated task_plan to be preserved, got count=%d", plans)
	}
}

func TestMigration_BackfillPipelineTaskItemID_FromLegacyTaskItems(t *testing.T) {
	db := openLegacySQLite(t)
	defer db.Close()

	if err := seedLegacySchema(db); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES ('proj-mig-2', 'proj', '/tmp/proj-mig-2')`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_plans (id, project_id, name, status) VALUES ('plan-mig-2', 'proj-mig-2', 'done plan', 'done')`); err != nil {
		t.Fatalf("insert task_plan: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO pipelines (id, project_id, name, template, status) VALUES ('pipe-mig-2', 'proj-mig-2', 'pipe', 'standard', 'done')`); err != nil {
		t.Fatalf("insert pipeline: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO task_items (id, plan_id, title, description, pipeline_id, status, created_at)
VALUES
	('task-early', 'plan-mig-2', 'early', 'early', 'pipe-mig-2', 'done', '2026-03-01T00:00:00Z'),
	('task-late', 'plan-mig-2', 'late', 'late', 'pipe-mig-2', 'done', '2026-03-01T01:00:00Z')
`); err != nil {
		t.Fatalf("insert task_items: %v", err)
	}

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var taskItemID string
	if err := db.QueryRow(`SELECT COALESCE(task_item_id, '') FROM pipelines WHERE id='pipe-mig-2'`).Scan(&taskItemID); err != nil {
		t.Fatalf("query pipelines.task_item_id: %v", err)
	}
	if taskItemID != "task-early" {
		t.Fatalf("expected deterministic backfill task_item_id=task-early, got %q", taskItemID)
	}
}

func TestMigration_AddsQueuedAtColumnBeforeCreatingIndexes(t *testing.T) {
	db := openLegacySQLite(t)
	defer db.Close()

	// Simulate an older local DB: pipelines exists but lacks queued_at, while applyMigrations wants
	// to create idx_pipelines_status_queued_at. This must not fail.
	if _, err := db.Exec(`
CREATE TABLE pipelines (
	id                TEXT PRIMARY KEY,
	project_id        TEXT NOT NULL,
	name              TEXT NOT NULL,
	template          TEXT NOT NULL,
	status            TEXT NOT NULL DEFAULT 'created',
	current_stage     TEXT,
	stages_json       TEXT NOT NULL DEFAULT '[]',
	artifacts_json    TEXT DEFAULT '{}',
	config_json       TEXT DEFAULT '{}',
	branch_name       TEXT,
	worktree_path     TEXT,
	error_message     TEXT,
	max_total_retries INTEGER DEFAULT 5,
	total_retries     INTEGER DEFAULT 0,
	run_count         INTEGER DEFAULT 0,
	last_error_type   TEXT,
	task_item_id      TEXT,
	last_heartbeat_at DATETIME,
	started_at        DATETIME,
	finished_at       DATETIME,
	created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);
`); err != nil {
		t.Fatalf("seed legacy pipelines table: %v", err)
	}

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	assertColumnExists(t, db, "pipelines", "queued_at")
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
