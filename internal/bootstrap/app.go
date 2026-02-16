package bootstrap

import (
	"context"
	"errors"
	"log/slog"

	"gorm.io/gorm"

	"zhanggui/internal/bootstrap/config"
	"zhanggui/internal/bootstrap/database"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/infrastructure/persistence/sqlite/model"
)

type App struct {
	Config config.Config
	DB     *gorm.DB
}

func New(ctx context.Context, configFile string) (*App, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(err, "check context")
	}

	logCtx := logging.WithAttrs(ctx, slog.String("component", "bootstrap.app"))
	logging.Info(logCtx, "loading application config", slog.String("config_file", configFile))

	cfg, err := config.Load(logCtx, configFile)
	if err != nil {
		return nil, errs.Wrap(err, "load config")
	}

	db, err := database.Open(logCtx, cfg.Database)
	if err != nil {
		return nil, errs.Wrap(err, "open database")
	}

	logging.Info(logCtx, "application bootstrap completed", slog.String("database_driver", cfg.Database.Driver))

	return &App{
		Config: cfg,
		DB:     db,
	}, nil
}

func (a *App) InitSchema(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "check context")
	}

	logCtx := logging.WithAttrs(ctx, slog.String("component", "bootstrap.app"))
	logging.Info(logCtx, "start schema migration")

	if err := a.DB.WithContext(ctx).AutoMigrate(
		&model.Issue{},
		&model.IssueLabel{},
		&model.Event{},
		&model.OutboxKV{},
		&model.QualityEvent{},
	); err != nil {
		return errs.Wrap(err, "auto migrate schema")
	}

	logging.Info(logCtx, "schema migration completed")
	return nil
}

func (a *App) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}

	sqlDB, err := a.DB.DB()
	if err != nil {
		return errs.Wrap(err, "get sql db")
	}

	if err := sqlDB.Close(); err != nil {
		return errs.Wrap(err, "close sql db")
	}

	logging.Info(logging.WithAttrs(ctx, slog.String("component", "bootstrap.app")), "database connection closed")
	return nil
}
