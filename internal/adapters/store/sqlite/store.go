package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	gormsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store implements core.Store backed by SQLite.
type Store struct {
	db  *sql.DB
	orm *gorm.DB
}

const startupDBTimeout = 6 * time.Second

// New opens (or creates) a SQLite database at path and runs migrations.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), startupDBTimeout)
	defer cancel()

	// SQLite: serialize writes through one connection to avoid SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, startupDBError(path, "ping sqlite", err)
	}
	// Enable WAL mode and foreign keys.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, startupDBError(path, fmt.Sprintf("exec %s", pragma), err)
		}
	}
	if err := runMigrations(ctx, db); err != nil {
		db.Close()
		return nil, startupDBError(path, "run migrations", err)
	}

	orm, err := gorm.Open(gormsqlite.Dialector{
		DriverName: "sqlite",
		Conn:       db,
	}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open gorm sqlite %s: %w", path, err)
	}

	return &Store{db: db, orm: orm}, nil
}

func startupDBError(path string, op string, err error) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf("%s %s: %v", op, path, err)
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "database is locked") {
		return fmt.Errorf("%s; database may be locked by another ai-flow process, stop old processes and remove %s-shm/%s-wal if needed", msg, path, path)
	}
	return fmt.Errorf("%s", msg)
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
