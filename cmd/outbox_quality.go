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

var outboxQualityCmd = &cobra.Command{
	Use:   "quality",
	Short: "Quality event ingest and audit commands",
}

var outboxQualityIngestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Ingest a quality event and write back structured evidence",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		source, _ := cmd.Flags().GetString("source")
		externalEventID, _ := cmd.Flags().GetString("event-id")
		category, _ := cmd.Flags().GetString("category")
		resultValue, _ := cmd.Flags().GetString("result")
		actor, _ := cmd.Flags().GetString("actor")
		summary, _ := cmd.Flags().GetString("summary")
		evidence, _ := cmd.Flags().GetStringSlice("evidence")
		eventKey, _ := cmd.Flags().GetString("event-key")
		payload, err := resolvePayload(cmd)
		if err != nil {
			return err
		}

		out, err := svc.IngestQualityEvent(ctx, outbox.IngestQualityEventInput{
			IssueRef:         issueRef,
			Source:           source,
			ExternalEventID:  externalEventID,
			Category:         category,
			Result:           resultValue,
			Actor:            actor,
			Summary:          summary,
			Evidence:         evidence,
			Payload:          payload,
			ProvidedEventKey: eventKey,
		})
		if err != nil {
			logging.Error(ctx, "ingest quality event failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "ingest quality event")
		}

		if out.Duplicate {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "quality event duplicate: issue=%s key=%s\n", out.IssueRef, out.IdempotencyKey); err != nil {
				return errs.Wrap(err, "write quality ingest output")
			}
			return nil
		}

		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"quality event ingested: issue=%s key=%s marker=%s routed=%s\n",
			out.IssueRef,
			out.IdempotencyKey,
			out.Marker,
			out.RoutedRole,
		); err != nil {
			return errs.Wrap(err, "write quality ingest output")
		}
		return nil
	}),
}

var outboxQualityListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ingested quality audit events for an issue",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		limit, _ := cmd.Flags().GetInt("limit")

		items, err := svc.ListQualityEvents(ctx, issueRef, limit)
		if err != nil {
			logging.Error(ctx, "list quality events failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "list quality events")
		}

		if len(items) == 0 {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "no quality events"); err != nil {
				return errs.Wrap(err, "write quality list output")
			}
			return nil
		}

		for _, item := range items {
			evidence := "none"
			if len(item.Evidence) > 0 {
				evidence = strings.Join(item.Evidence, ",")
			}
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"qe%d key=%s source=%s event=%s category=%s result=%s actor=%s at=%s evidence=%s\n",
				item.QualityEventID,
				item.IdempotencyKey,
				item.Source,
				item.ExternalEventID,
				item.Category,
				item.Result,
				item.Actor,
				item.IngestedAt,
				evidence,
			); err != nil {
				return errs.Wrap(err, "write quality list item")
			}
		}

		return nil
	}),
}

func init() {
	outboxCmd.AddCommand(outboxQualityCmd)
	outboxQualityCmd.AddCommand(outboxQualityIngestCmd)
	outboxQualityCmd.AddCommand(outboxQualityListCmd)

	outboxQualityIngestCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxQualityIngestCmd.Flags().String("source", "manual", "Quality event source (for example github/gitlab/manual)")
	outboxQualityIngestCmd.Flags().String("event-id", "", "External event id")
	outboxQualityIngestCmd.Flags().String("category", "", "Quality event category: review|ci (optional when payload can be inferred)")
	outboxQualityIngestCmd.Flags().String("result", "", "Quality event result: approved|changes_requested|pass|fail (optional when payload can be inferred)")
	outboxQualityIngestCmd.Flags().String("actor", "quality-bot", "Event actor")
	outboxQualityIngestCmd.Flags().String("summary", "", "Optional summary override")
	outboxQualityIngestCmd.Flags().StringSlice("evidence", nil, "Evidence link(s)")
	outboxQualityIngestCmd.Flags().String("event-key", "", "Optional idempotency key override")
	outboxQualityIngestCmd.Flags().String("payload", "", "Optional raw source payload")
	outboxQualityIngestCmd.Flags().String("payload-file", "", "Path to payload file")
	_ = outboxQualityIngestCmd.MarkFlagRequired("issue")

	outboxQualityListCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxQualityListCmd.Flags().Int("limit", 20, "Max records to show")
	_ = outboxQualityListCmd.MarkFlagRequired("issue")
}

func resolvePayload(cmd *cobra.Command) (string, error) {
	inlinePayload, _ := cmd.Flags().GetString("payload")
	payloadFile, _ := cmd.Flags().GetString("payload-file")

	if strings.TrimSpace(inlinePayload) != "" && strings.TrimSpace(payloadFile) != "" {
		return "", errors.New("payload and payload-file are mutually exclusive")
	}

	if strings.TrimSpace(payloadFile) != "" {
		raw, err := os.ReadFile(payloadFile)
		if err != nil {
			return "", errs.Wrapf(err, "read payload file %q", payloadFile)
		}
		inlinePayload = string(raw)
	}

	return inlinePayload, nil
}
