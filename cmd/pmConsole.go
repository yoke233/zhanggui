package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
	"zhanggui/internal/usecase/pmconsole"
)

var consolePmCmd = &cobra.Command{
	Use:     "pm",
	Aliases: []string{"lead"},
	Short:   "Start PM/Operator operations console (global view)",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		if cmd.CalledAs() == "lead" {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: `console lead` is deprecated; use `console pm` instead")
		}

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

		model := pmconsole.NewPMModel(ctx, svc, pmconsole.PMOptions{
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
			return errs.Wrap(err, "run pm console")
		}
		return nil
	}),
}

func init() {
	consoleCmd.AddCommand(consolePmCmd)
	consolePmCmd.Flags().String("role", "all", "Scope role filter (all|backend|frontend|qa|reviewer|integrator)")
	consolePmCmd.Flags().String("assignee", "", "Optional assignee filter (role=all) or override (role!=all)")
	consolePmCmd.Flags().String("state", "", "Optional state filter (todo|doing|blocked|review|done)")
	consolePmCmd.Flags().String("workflow", "workflow.toml", "Path to workflow.toml")
	consolePmCmd.Flags().Duration("refresh-interval", 5*time.Second, "Auto refresh interval")
}
