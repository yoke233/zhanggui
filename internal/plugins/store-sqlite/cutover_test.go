package storesqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCutover_V2Baseline_NoWave3CompatibilityArtifacts(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	assertTableExists(t, db, "issues")
	assertTableExists(t, db, "review_records")
	assertTableNotExists(t, db, "migration_flags")
	assertTableNotExists(t, db, "task_plans")
	assertTableNotExists(t, db, "task_items")
}

func TestCutover_V2Baseline_IdempotentApplyMigrations(t *testing.T) {
	db := openSQLite(t)
	defer db.Close()

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations first run: %v", err)
	}
	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations second run: %v", err)
	}

	assertIndexExists(t, db, "idx_runs_status_queued_at")
	assertIndexExists(t, db, "idx_issues_run")
	assertIndexExists(t, db, "idx_review_records_issue")
	assertTableNotExists(t, db, "migration_flags")
}

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "v2-baseline.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()

	ok, err := hasTable(db, table)
	if err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	if !ok {
		t.Fatalf("expected table %s to exist", table)
	}
}

func assertTableNotExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()

	ok, err := hasTable(db, table)
	if err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	if ok {
		t.Fatalf("expected table %s to be absent", table)
	}
}

func assertIndexExists(t *testing.T, db *sql.DB, index string) {
	t.Helper()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, index).Scan(&count); err != nil {
		t.Fatalf("check index %s: %v", index, err)
	}
	if count == 0 {
		t.Fatalf("expected index %s to exist", index)
	}
}
