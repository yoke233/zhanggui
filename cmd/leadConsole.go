package cmd

import (
	"log/slog"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/leadconsole"
	"zhanggui/internal/usecase/outbox"
)

var consoleLeadCmd = &cobra.Command{
	Use:   "lead",
	Short: "Start lead operations console",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		role, _ := cmd.Flags().GetString("role")
		assignee, _ := cmd.Flags().GetString("assignee")
		state, _ := cmd.Flags().GetString("state")
		workflowFile, _ := cmd.Flags().GetString("workflow")
		refreshInterval, _ := cmd.Flags().GetDuration("refresh-interval")
		if refreshInterval <= 0 {
			refreshInterval = 5 * time.Second
		}
		executablePath, err := os.Executable()
		if err != nil {
			executablePath = ""
		}

		model := leadconsole.NewLeadModel(ctx, svc, leadconsole.LeadOptions{
			Role:            role,
			Assignee:        assignee,
			StateFilter:     state,
			WorkflowFile:    workflowFile,
			ConfigFile:      cfgFile,
			ExecutablePath:  executablePath,
			RefreshInterval: refreshInterval,
		})

		program := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := program.Run(); err != nil {
			return errs.Wrap(err, "run lead console")
		}
		return nil
	}),
}

func init() {
	consoleCmd.AddCommand(consoleLeadCmd)
	consoleLeadCmd.Flags().String("role", "backend", "Role queue to inspect")
	consoleLeadCmd.Flags().String("assignee", "", "Assignee filter (default: lead-<role>)")
	consoleLeadCmd.Flags().String("state", "", "Optional state filter (todo|doing|blocked|review|done)")
	consoleLeadCmd.Flags().String("workflow", "workflow.toml", "Path to workflow.toml")
	consoleLeadCmd.Flags().Duration("refresh-interval", 5*time.Second, "Auto refresh interval")
}
