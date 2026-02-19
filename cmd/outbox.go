package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
)

var outboxCmd = &cobra.Command{
	Use:   "outbox",
	Short: "Manage local outbox issues and events",
}

var outboxCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a local outbox issue",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		title, _ := cmd.Flags().GetString("title")
		body, err := resolveBody(cmd, true)
		if err != nil {
			return err
		}

		labels, _ := cmd.Flags().GetStringSlice("label")
		issueRef, err := svc.CreateIssue(ctx, outbox.CreateIssueInput{
			Title:  title,
			Body:   body,
			Labels: labels,
		})
		if err != nil {
			logging.Error(ctx, "create outbox issue failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "create outbox issue")
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "created issue: %s\n", issueRef); err != nil {
			return errs.Wrap(err, "write create output")
		}
		return nil
	}),
}

var outboxClaimCmd = &cobra.Command{
	Use:   "claim",
	Short: "Claim an issue via assignee and move it to state:doing",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		assignee, _ := cmd.Flags().GetString("assignee")
		actor, _ := cmd.Flags().GetString("actor")
		comment, err := resolveBody(cmd, false)
		if err != nil {
			return err
		}

		if err := svc.ClaimIssue(ctx, outbox.ClaimIssueInput{
			IssueRef: issueRef,
			Assignee: assignee,
			Actor:    actor,
			Comment:  comment,
		}); err != nil {
			logging.Error(ctx, "claim outbox issue failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "claim outbox issue")
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "claimed issue: %s assignee=%s\n", issueRef, assignee); err != nil {
			return errs.Wrap(err, "write claim output")
		}
		return nil
	}),
}

var outboxCommentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Append a structured comment event to an issue",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		actor, _ := cmd.Flags().GetString("actor")
		state, _ := cmd.Flags().GetString("state")
		body, err := resolveBody(cmd, true)
		if err != nil {
			return err
		}

		if err := svc.CommentIssue(ctx, outbox.CommentIssueInput{
			IssueRef: issueRef,
			Actor:    actor,
			Body:     body,
			State:    state,
		}); err != nil {
			logging.Error(ctx, "append outbox comment failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "append outbox comment")
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "appended comment to issue: %s\n", issueRef); err != nil {
			return errs.Wrap(err, "write comment output")
		}
		return nil
	}),
}

var outboxCloseCmd = &cobra.Command{
	Use:   "close",
	Short: "Close an issue and set state:done",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		actor, _ := cmd.Flags().GetString("actor")
		comment, err := resolveBody(cmd, false)
		if err != nil {
			return err
		}

		if err := svc.CloseIssue(ctx, outbox.CloseIssueInput{
			IssueRef: issueRef,
			Actor:    actor,
			Comment:  comment,
		}); err != nil {
			logging.Error(ctx, "close outbox issue failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "close outbox issue")
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "closed issue: %s\n", issueRef); err != nil {
			return errs.Wrap(err, "write close output")
		}
		return nil
	}),
}

var outboxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		includeClosed, _ := cmd.Flags().GetBool("all")
		assignee, _ := cmd.Flags().GetString("assignee")

		items, err := svc.ListIssues(ctx, includeClosed, assignee)
		if err != nil {
			logging.Error(ctx, "list outbox issues failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "list outbox issues")
		}

		if len(items) == 0 {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "no issues"); err != nil {
				return errs.Wrap(err, "write list output")
			}
			return nil
		}

		for _, item := range items {
			status := "open"
			if item.IsClosed {
				status = "closed"
			}
			assigneeValue := item.Assignee
			if assigneeValue == "" {
				assigneeValue = "-"
			}
			labels := "-"
			if len(item.Labels) > 0 {
				labels = strings.Join(item.Labels, ",")
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"%s [%s] assignee=%s labels=%s title=%s\n",
				item.IssueRef,
				status,
				assigneeValue,
				labels,
				item.Title,
			); err != nil {
				return errs.Wrap(err, "write list item")
			}
		}
		return nil
	}),
}

var outboxShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show issue detail and timeline events",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		issue, err := svc.GetIssue(ctx, issueRef)
		if err != nil {
			logging.Error(ctx, "show outbox issue failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "show outbox issue")
		}

		status := "open"
		if issue.IsClosed {
			status = "closed"
		}
		assignee := issue.Assignee
		if assignee == "" {
			assignee = "-"
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "IssueRef: %s\n", issue.IssueRef); err != nil {
			return errs.Wrap(err, "write show output")
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", status); err != nil {
			return errs.Wrap(err, "write show output")
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Assignee: %s\n", assignee); err != nil {
			return errs.Wrap(err, "write show output")
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Labels: %s\n", strings.Join(issue.Labels, ",")); err != nil {
			return errs.Wrap(err, "write show output")
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", issue.Title); err != nil {
			return errs.Wrap(err, "write show output")
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "CreatedAt: %s\n", issue.CreatedAt); err != nil {
			return errs.Wrap(err, "write show output")
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "UpdatedAt: %s\n", issue.UpdatedAt); err != nil {
			return errs.Wrap(err, "write show output")
		}
		if issue.ClosedAt != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "ClosedAt: %s\n", issue.ClosedAt); err != nil {
				return errs.Wrap(err, "write show output")
			}
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "\nBody:\n%s\n", issue.Body); err != nil {
			return errs.Wrap(err, "write show body")
		}

		if len(issue.Events) == 0 {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "\nEvents: none"); err != nil {
				return errs.Wrap(err, "write show events")
			}
			return nil
		}

		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "\nEvents:"); err != nil {
			return errs.Wrap(err, "write show events")
		}
		for _, event := range issue.Events {
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"- e%d actor=%s at=%s\n%s\n\n",
				event.EventID,
				event.Actor,
				event.CreatedAt,
				event.Body,
			); err != nil {
				return errs.Wrap(err, "write show event")
			}
		}

		return nil
	}),
}

var outboxLabelCmd = &cobra.Command{
	Use:   "label",
	Short: "Add or remove issue labels",
}

var outboxLabelAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add one or more labels to an issue",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		actor, _ := cmd.Flags().GetString("actor")
		labels, _ := cmd.Flags().GetStringSlice("label")

		if err := svc.AddIssueLabels(ctx, outbox.AddIssueLabelsInput{
			IssueRef: issueRef,
			Actor:    actor,
			Labels:   labels,
		}); err != nil {
			logging.Error(ctx, "add issue labels failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "add issue labels")
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "labels added to issue: %s\n", issueRef); err != nil {
			return errs.Wrap(err, "write label add output")
		}
		return nil
	}),
}

var outboxLabelRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove one or more labels from an issue",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		actor, _ := cmd.Flags().GetString("actor")
		labels, _ := cmd.Flags().GetStringSlice("label")

		if err := svc.RemoveIssueLabels(ctx, outbox.RemoveIssueLabelsInput{
			IssueRef: issueRef,
			Actor:    actor,
			Labels:   labels,
		}); err != nil {
			logging.Error(ctx, "remove issue labels failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "remove issue labels")
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "labels removed from issue: %s\n", issueRef); err != nil {
			return errs.Wrap(err, "write label remove output")
		}
		return nil
	}),
}

func init() {
	rootCmd.AddCommand(outboxCmd)
	outboxCmd.AddCommand(outboxCreateCmd)
	outboxCmd.AddCommand(outboxClaimCmd)
	outboxCmd.AddCommand(outboxCommentCmd)
	outboxCmd.AddCommand(outboxCloseCmd)
	outboxCmd.AddCommand(outboxListCmd)
	outboxCmd.AddCommand(outboxShowCmd)
	outboxCmd.AddCommand(outboxLabelCmd)
	outboxCmd.AddCommand(outboxPipelineCmd)
	outboxCmd.AddCommand(outboxMergeCmd)
	outboxLabelCmd.AddCommand(outboxLabelAddCmd)
	outboxLabelCmd.AddCommand(outboxLabelRemoveCmd)
	outboxPipelineCmd.AddCommand(outboxPipelineRunCmd)
	outboxMergeCmd.AddCommand(outboxMergeCheckCmd)
	outboxMergeCmd.AddCommand(outboxMergeApplyCmd)

	outboxCreateCmd.Flags().String("title", "", "Issue title")
	outboxCreateCmd.Flags().String("body", "", "Issue body content")
	outboxCreateCmd.Flags().String("body-file", "", "Path to issue body markdown file")
	outboxCreateCmd.Flags().StringSlice("label", nil, "Issue labels")
	_ = outboxCreateCmd.MarkFlagRequired("title")

	outboxClaimCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxClaimCmd.Flags().String("assignee", "", "Claim owner")
	outboxClaimCmd.Flags().String("actor", "", "Event actor (default: assignee)")
	outboxClaimCmd.Flags().String("body", "", "Claim comment content")
	outboxClaimCmd.Flags().String("body-file", "", "Path to claim comment markdown file")
	_ = outboxClaimCmd.MarkFlagRequired("issue")
	_ = outboxClaimCmd.MarkFlagRequired("assignee")

	outboxCommentCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxCommentCmd.Flags().String("actor", "", "Event actor")
	outboxCommentCmd.Flags().String("state", "", "Optional state label (todo|doing|blocked|review|done)")
	outboxCommentCmd.Flags().String("body", "", "Comment content")
	outboxCommentCmd.Flags().String("body-file", "", "Path to comment markdown file")
	_ = outboxCommentCmd.MarkFlagRequired("issue")
	_ = outboxCommentCmd.MarkFlagRequired("actor")

	outboxCloseCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxCloseCmd.Flags().String("actor", "", "Event actor for optional close comment")
	outboxCloseCmd.Flags().String("body", "", "Optional close comment content")
	outboxCloseCmd.Flags().String("body-file", "", "Path to optional close comment markdown file")
	_ = outboxCloseCmd.MarkFlagRequired("issue")

	outboxListCmd.Flags().Bool("all", false, "Include closed issues")
	outboxListCmd.Flags().String("assignee", "", "Filter by assignee")

	outboxShowCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	_ = outboxShowCmd.MarkFlagRequired("issue")

	outboxLabelAddCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxLabelAddCmd.Flags().String("actor", "", "Event actor")
	outboxLabelAddCmd.Flags().StringSlice("label", nil, "Label(s) to add")
	_ = outboxLabelAddCmd.MarkFlagRequired("issue")
	_ = outboxLabelAddCmd.MarkFlagRequired("actor")
	_ = outboxLabelAddCmd.MarkFlagRequired("label")

	outboxLabelRemoveCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxLabelRemoveCmd.Flags().String("actor", "", "Event actor")
	outboxLabelRemoveCmd.Flags().StringSlice("label", nil, "Label(s) to remove")
	_ = outboxLabelRemoveCmd.MarkFlagRequired("issue")
	_ = outboxLabelRemoveCmd.MarkFlagRequired("actor")
	_ = outboxLabelRemoveCmd.MarkFlagRequired("label")
}

func resolveBody(cmd *cobra.Command, required bool) (string, error) {
	inlineBody, _ := cmd.Flags().GetString("body")
	bodyFile, _ := cmd.Flags().GetString("body-file")

	if strings.TrimSpace(inlineBody) != "" && strings.TrimSpace(bodyFile) != "" {
		return "", errors.New("body and body-file are mutually exclusive")
	}

	if strings.TrimSpace(bodyFile) != "" {
		raw, err := os.ReadFile(bodyFile)
		if err != nil {
			return "", errs.Wrapf(err, "read body file %q", bodyFile)
		}
		inlineBody = string(raw)
	}

	if required && strings.TrimSpace(inlineBody) == "" {
		return "", errors.New("body is required (set --body or --body-file)")
	}
	return inlineBody, nil
}
