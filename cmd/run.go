package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run one worker execution from context pack",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		contextPackDir, _ := cmd.Flags().GetString("context-pack")
		workflowFile, _ := cmd.Flags().GetString("workflow")
		if err := svc.WorkerRun(ctx, outbox.WorkerRunInput{
			ContextPackDir: contextPackDir,
			WorkflowFile:   workflowFile,
		}); err != nil {
			return errs.Wrap(err, "run worker")
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "worker completed context-pack=%s\n", contextPackDir); err != nil {
			return errs.Wrap(err, "write worker output")
		}
		return nil
	}),
}

func init() {
	workerCmd.AddCommand(runCmd)
	runCmd.Flags().String("context-pack", "", "Context pack directory")
	runCmd.Flags().String("workflow", "workflow.toml", "Path to workflow.toml")
	_ = runCmd.MarkFlagRequired("context-pack")
}
