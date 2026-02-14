/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
)

// initDbCmd represents the initDb command
var initDbCmd = &cobra.Command{
	Use:   "init-db",
	Short: "Initialize database schema",
	RunE: withApp(func(cmd *cobra.Command, app *bootstrap.App) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))
		logging.Info(ctx, "start init-db")

		if err := app.InitSchema(ctx); err != nil {
			logging.Error(ctx, "initialize schema failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "initialize schema")
		}

		logging.Info(ctx, "init-db finished", slog.String("database_dsn", app.Config.Database.DSN))
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "database schema initialized: %s\n", app.Config.Database.DSN); err != nil {
			return errs.Wrap(err, "write init-db output")
		}
		return nil
	}),
}

func init() {
	rootCmd.AddCommand(initDbCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initDbCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initDbCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
