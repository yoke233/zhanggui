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

var outboxMergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge gate checks and apply actions",
}

var outboxMergeCheckCmd = newOutboxMergeCheckCmd(nil)
var outboxMergeApplyCmd = newOutboxMergeApplyCmd(nil)

func newOutboxMergeCheckCmd(svc *outbox.Service) *cobra.Command {
	runWithService := func(cmd *cobra.Command, mergeSvc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		issueRef = strings.TrimSpace(issueRef)

		ok, reason, err := mergeSvc.CanMergeIssue(ctx, issueRef)
		if err != nil {
			logging.Error(ctx, "merge check failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "check merge gate")
		}

		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"merge check: issue=%s ready=%t reason=%s\n",
			issueRef,
			ok,
			reason,
		); err != nil {
			return errs.Wrap(err, "write merge check output")
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
		Use:   "check",
		Short: "Check whether an issue passes merge gate",
		RunE:  runE,
	}

	cmd.Flags().String("issue", "", "IssueRef, for example local#12")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}

func newOutboxMergeApplyCmd(svc *outbox.Service) *cobra.Command {
	runWithService := func(cmd *cobra.Command, mergeSvc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		actor, _ := cmd.Flags().GetString("actor")
		comment, _ := cmd.Flags().GetString("body")

		issueRef = strings.TrimSpace(issueRef)
		actor = strings.TrimSpace(actor)
		comment = strings.TrimSpace(comment)
		if actor == "" {
			return errors.New("actor is required")
		}

		ok, reason, err := mergeSvc.CanMergeIssue(ctx, issueRef)
		if err != nil {
			logging.Error(ctx, "merge gate check before apply failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "check merge gate before apply")
		}

		if !ok {
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"merge apply blocked: issue=%s reason=%s\n",
				issueRef,
				reason,
			); err != nil {
				return errs.Wrap(err, "write merge apply blocked output")
			}
			return fmt.Errorf("merge gate blocked: %s", reason)
		}

		if comment == "" {
			comment = buildMergeApplyCloseComment(issueRef, reason)
		}

		if err := mergeSvc.CloseIssue(ctx, outbox.CloseIssueInput{
			IssueRef: issueRef,
			Actor:    actor,
			Comment:  comment,
		}); err != nil {
			logging.Error(ctx, "merge apply close issue failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "close issue on merge apply")
		}

		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"merge apply: issue=%s closed=true reason=%s\n",
			issueRef,
			reason,
		); err != nil {
			return errs.Wrap(err, "write merge apply output")
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
		Use:   "apply",
		Short: "Apply merge gate action and close issue when gate passes",
		RunE:  runE,
	}

	cmd.Flags().String("issue", "", "IssueRef, for example local#12")
	cmd.Flags().String("actor", "lead-integrator", "Actor used to close issue after merge gate passes")
	cmd.Flags().String("body", "", "Optional close comment body (auto-generated when empty)")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}

func buildMergeApplyCloseComment(issueRef string, reason string) string {
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		trimmedReason = "ready"
	}

	return fmt.Sprintf(
		"Summary:\n- merge gate passed (%s)\n\nChanges:\n- PR: none\n- Commit: git:merge-gate\n\nTests:\n- Command: outbox merge check --issue %s\n- Result: pass\n- Evidence: merge-gate\n",
		trimmedReason,
		issueRef,
	)
}
