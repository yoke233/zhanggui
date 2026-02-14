package config

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/spf13/viper"

	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
)

type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Database DatabaseConfig `mapstructure:"database"`
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
}

func Load(ctx context.Context, configFile string) (Config, error) {
	if ctx == nil {
		return Config{}, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return Config{}, errs.Wrap(err, "check context")
	}

	logCtx := logging.WithAttrs(ctx, slog.String("component", "bootstrap.config"))

	v := viper.New()
	setDefaults(logCtx, v)

	v.SetEnvPrefix("ZG")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if configFile == "" && errors.As(err, &notFound) {
			// Keep default and env-backed config when no file is provided.
			logging.Warn(logCtx, "config file not found, fallback to defaults and env")
		} else {
			return Config{}, errs.Wrap(err, "read config")
		}
	} else {
		logging.Info(logCtx, "using config file", slog.String("path", v.ConfigFileUsed()))
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, errs.Wrap(err, "unmarshal config")
	}

	if cfg.Database.DSN == "" {
		return Config{}, errors.New("database.dsn is required")
	}

	logging.Info(
		logCtx,
		"config loaded",
		slog.String("app", cfg.App.Name),
		slog.String("env", cfg.App.Env),
		slog.String("database_driver", cfg.Database.Driver),
	)

	return cfg, nil
}

func setDefaults(ctx context.Context, v *viper.Viper) {
	if ctx == nil {
		return
	}

	v.SetDefault("app.name", "zhanggui")
	v.SetDefault("app.env", "local")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", ".agents/state/outbox.sqlite")
}
