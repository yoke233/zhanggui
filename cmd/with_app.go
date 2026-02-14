package cmd

import (
	"context"
	"log/slog"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
)

func withApp(run func(cmd *cobra.Command, app *bootstrap.App, outboxSvc *outbox.Service) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := logging.WithAttrs(
			cmd.Context(),
			slog.String("command", cmd.CommandPath()),
			slog.String("config_file", cfgFile),
		)

		var app *bootstrap.App
		var outboxSvc *outbox.Service
		fxApp := fx.New(
			bootstrap.Module,
			fx.Provide(func() context.Context { return ctx }),
			fx.Provide(
				fx.Annotate(
					func() string { return cfgFile },
					fx.ResultTags(`name:"configFile"`),
				),
			),
			fx.Populate(&app, &outboxSvc),
		)

		startCtx, cancelStart := context.WithTimeout(ctx, 10*time.Second)
		defer cancelStart()
		if err := fxApp.Start(startCtx); err != nil {
			logging.Error(ctx, "bootstrap application failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "start fx application")
		}

		defer func() {
			stopCtx, cancelStop := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelStop()
			if err := fxApp.Stop(stopCtx); err != nil {
				logging.Error(ctx, "fx application stop failed", slog.Any("err", errs.Loggable(err)))
			}
		}()

		if err := run(cmd, app, outboxSvc); err != nil {
			return errs.Wrap(err, "run command")
		}
		return nil
	}
}
