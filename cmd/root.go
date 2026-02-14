/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"log/slog"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:          "zhanggui",
	Short:        "Clean architecture Go project scaffold",
	Long:         "Project bootstrap CLI powered by Cobra + Viper + GORM(SQLite no-cgo).",
	SilenceUsage: true,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}

	logger := slog.New(slog.NewTextHandler(rootCmd.ErrOrStderr(), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	ctx = logging.WithLogger(ctx, logger)
	ctx = logging.WithAttrs(ctx, slog.String("app", "zhanggui"))

	rootCmd.SetContext(ctx)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logging.Error(ctx, "command execution failed", slog.Any("err", errs.Loggable(err)))
		return errs.Wrap(err, "execute root command")
	}

	return nil
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "configs/config.yaml", "Config file path")
}
