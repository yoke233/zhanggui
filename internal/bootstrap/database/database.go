package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	gormsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"zhanggui/internal/bootstrap/config"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
)

func Open(ctx context.Context, cfg config.DatabaseConfig) (*gorm.DB, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(err, "check context")
	}

	logCtx := logging.WithAttrs(ctx, slog.String("component", "bootstrap.database"))

	switch strings.ToLower(cfg.Driver) {
	case "sqlite", "sqlite3":
		if err := ensureSQLiteDirectory(logCtx, cfg.DSN); err != nil {
			return nil, errs.Wrap(err, "ensure sqlite directory")
		}

		db, err := gorm.Open(gormsqlite.Open(cfg.DSN), &gorm.Config{})
		if err != nil {
			return nil, errs.Wrap(err, "open sqlite db")
		}
		logging.Info(logCtx, "database opened", slog.String("driver", "sqlite"), slog.String("dsn", cfg.DSN))
		return db, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func ensureSQLiteDirectory(ctx context.Context, dsn string) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "check context")
	}

	candidate := strings.TrimSpace(dsn)
	if candidate == "" || candidate == ":memory:" {
		return nil
	}

	if strings.HasPrefix(strings.ToLower(candidate), "file:") {
		candidate = strings.TrimPrefix(candidate, "file:")
	}
	if idx := strings.Index(candidate, "?"); idx >= 0 {
		candidate = candidate[:idx]
	}

	dir := filepath.Dir(candidate)
	if dir == "" || dir == "." {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.Wrapf(err, "create sqlite directory %q", dir)
	}

	logging.Info(logging.WithAttrs(ctx, slog.String("component", "bootstrap.database")), "sqlite directory ensured", slog.String("dir", dir))
	return nil
}
