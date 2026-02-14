package bootstrap

import (
	"context"
	"log/slog"

	"go.uber.org/fx"
	"gorm.io/gorm"

	"zhanggui/internal/bootstrap/config"
	"zhanggui/internal/bootstrap/database"
	"zhanggui/internal/bootstrap/logging"
	cacheinfra "zhanggui/internal/infrastructure/cache"
	sqliterepo "zhanggui/internal/infrastructure/persistence/sqlite/repository"
	sqliteuow "zhanggui/internal/infrastructure/persistence/sqlite/uow"
	"zhanggui/internal/ports"
	"zhanggui/internal/usecase/outbox"
)

var Module = fx.Options(
	fx.Provide(provideConfig),
	fx.Provide(provideDatabase),
	fx.Provide(provideApp),
	fx.Provide(
		fx.Annotate(
			sqliterepo.NewOutboxRepository,
			fx.As(new(ports.OutboxRepository)),
		),
	),
	fx.Provide(
		fx.Annotate(
			sqliteuow.NewUnitOfWork,
			fx.As(new(ports.UnitOfWork)),
		),
	),
	fx.Provide(
		fx.Annotate(
			cacheinfra.NewSQLiteCache,
			fx.As(new(ports.Cache)),
		),
	),
	fx.Provide(outbox.NewService),
)

type configParams struct {
	fx.In

	Ctx        context.Context
	ConfigFile string `name:"configFile"`
}

func provideConfig(p configParams) (config.Config, error) {
	ctx := logging.WithAttrs(p.Ctx, slog.String("component", "bootstrap.fx"))
	return config.Load(ctx, p.ConfigFile)
}

func provideDatabase(lc fx.Lifecycle, ctx context.Context, cfg config.Config) (*gorm.DB, error) {
	logCtx := logging.WithAttrs(ctx, slog.String("component", "bootstrap.fx"))

	db, err := database.Open(logCtx, cfg.Database)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			return sqlDB.Close()
		},
	})

	return db, nil
}

func provideApp(cfg config.Config, db *gorm.DB) *App {
	return &App{
		Config: cfg,
		DB:     db,
	}
}
