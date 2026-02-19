package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
)

var outboxPipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Run codex coding/review/test pipeline",
}

var outboxPipelineRunCmd = newOutboxPipelineRunCmd(nil)

func newOutboxPipelineRunCmd(svc *outbox.Service) *cobra.Command {
	runWithService := func(cmd *cobra.Command, pipelineSvc *outbox.Service) error {
		if pipelineSvc == nil {
			return errors.New("outbox service is not configured")
		}

		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		projectDir, _ := cmd.Flags().GetString("project-dir")
		promptFile, _ := cmd.Flags().GetString("prompt-file")
		workflowFile, _ := cmd.Flags().GetString("workflow")
		codingRole, _ := cmd.Flags().GetString("coding-role")
		maxReviewRound, _ := cmd.Flags().GetInt("max-review-round")
		maxTestRound, _ := cmd.Flags().GetInt("max-test-round")

		pipelineSvc.ConfigureCodexRunnerWithWorkflow(strings.TrimSpace(workflowFile))

		out, err := pipelineSvc.RunCodexPipeline(ctx, outbox.RunCodexPipelineInput{
			IssueRef:       strings.TrimSpace(issueRef),
			ProjectDir:     strings.TrimSpace(projectDir),
			PromptFile:     strings.TrimSpace(promptFile),
			CodingRole:     strings.TrimSpace(codingRole),
			MaxReviewRound: maxReviewRound,
			MaxTestRound:   maxTestRound,
		})
		if err != nil {
			logging.Error(ctx, "run outbox pipeline failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "run outbox pipeline")
		}

		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"pipeline run finished: issue=%s ready_to_merge=%t rounds=%d last_result_code=%s\n",
			out.IssueRef,
			out.ReadyToMerge,
			out.Rounds,
			out.LastResultCode,
		); err != nil {
			return errs.Wrap(err, "write pipeline output")
		}
		return nil
	}

	runE := withApp(func(cmd *cobra.Command, _ *bootstrap.App, appSvc *outbox.Service) error {
		return runWithService(cmd, appSvc)
	})
	if svc != nil {
		runE = func(cmd *cobra.Command, _ []string) error {
			return runWithService(cmd, svc)
		}
	}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute coding -> review -> test loop and open merge gate on success",
		RunE:  runE,
	}

	cmd.Flags().String("issue", "", "IssueRef, for example local#12")
	cmd.Flags().String("project-dir", "", "Target project directory")
	cmd.Flags().String("prompt-file", "", "Prompt file path")
	cmd.Flags().String("workflow", "workflow.toml", "Path to workflow.toml")
	cmd.Flags().String("coding-role", "backend", "Coding role for pipeline run")
	cmd.Flags().Int("max-review-round", 3, "Max review rounds before pipeline fails")
	cmd.Flags().Int("max-test-round", 3, "Max test rounds before pipeline fails")
	_ = cmd.MarkFlagRequired("issue")
	_ = cmd.MarkFlagRequired("project-dir")
	_ = cmd.MarkFlagRequired("prompt-file")

	return cmd
}
