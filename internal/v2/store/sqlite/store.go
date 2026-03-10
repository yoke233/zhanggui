package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store implements core.Store backed by SQLite.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at path and runs migrations.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// SQLite: serialize writes through one connection to avoid SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	// Enable WAL mode and foreign keys.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("exec %s: %w", pragma, err)
		}
	}
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
