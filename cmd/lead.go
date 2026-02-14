package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
)

var leadCmd = &cobra.Command{
	Use:   "lead",
	Short: "Lead control-plane commands",
}

var leadRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run role lead loop with polling and cursor",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		role, _ := cmd.Flags().GetString("role")
		assignee, _ := cmd.Flags().GetString("assignee")
		workflowFile, _ := cmd.Flags().GetString("workflow")
		once, _ := cmd.Flags().GetBool("once")
		pollInterval, _ := cmd.Flags().GetDuration("poll-interval")
		eventBatch, _ := cmd.Flags().GetInt("event-batch")

		executablePath, err := os.Executable()
		if err != nil {
			return errs.Wrap(err, "resolve executable path")
		}

		runOnce := func() error {
			result, err := svc.LeadSyncOnce(ctx, outbox.LeadSyncInput{
				Role:           role,
				Assignee:       assignee,
				WorkflowFile:   workflowFile,
				ConfigFile:     cfgFile,
				ExecutablePath: executablePath,
				EventBatch:     eventBatch,
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"lead sync role=%s cursor=%d->%d candidates=%d processed=%d blocked=%d spawned=%d skipped=%d\n",
				role,
				result.CursorBefore,
				result.CursorAfter,
				result.Candidates,
				result.Processed,
				result.Blocked,
				result.Spawned,
				result.Skipped,
			); err != nil {
				return errs.Wrap(err, "write lead output")
			}
			return nil
		}

		if once {
			return runOnce()
		}

		if pollInterval <= 0 {
			pollInterval = 5 * time.Second
		}

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			if err := runOnce(); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return errs.Wrap(ctx.Err(), "lead run loop stopped")
			case <-ticker.C:
			}
		}
	}),
}

func init() {
	rootCmd.AddCommand(leadCmd)
	leadCmd.AddCommand(leadRunCmd)

	leadRunCmd.Flags().String("role", "backend", "Role to run as lead")
	leadRunCmd.Flags().String("assignee", "lead-backend", "Required assignee value for claimed issues")
	leadRunCmd.Flags().String("workflow", "workflow.toml", "Path to workflow.toml")
	leadRunCmd.Flags().Bool("once", false, "Run one sync tick and exit")
	leadRunCmd.Flags().Duration("poll-interval", 5*time.Second, "Polling interval for continuous lead loop")
	leadRunCmd.Flags().Int("event-batch", 200, "Max events fetched per sync tick")
}
