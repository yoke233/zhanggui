package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
)

var outboxQualityExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export ingested quality events for an issue",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		limit, _ := cmd.Flags().GetInt("limit")
		format, _ := cmd.Flags().GetString("format")
		outPath, _ := cmd.Flags().GetString("out")
		sinceRaw, _ := cmd.Flags().GetString("since")
		untilRaw, _ := cmd.Flags().GetString("until")

		if limit <= 0 {
			limit = 200
		}

		format = strings.ToLower(strings.TrimSpace(format))
		if format == "" {
			format = "json"
		}
		if format != "json" && format != "jsonl" {
			return fmt.Errorf("unsupported format %q (expected: json or jsonl)", format)
		}

		window, err := parseQualityExportWindow(sinceRaw, untilRaw)
		if err != nil {
			return err
		}

		events, err := svc.ListQualityEvents(ctx, issueRef, limit)
		if err != nil {
			logging.Error(ctx, "list quality events for export failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "list quality events")
		}

		filteredEvents, err := filterQualityEventsByIngestedAt(events, window)
		if err != nil {
			return err
		}

		payload, err := marshalQualityExportPayload(filteredEvents, format)
		if err != nil {
			return err
		}

		writer, closeFn, err := resolveQualityExportWriter(cmd, outPath)
		if err != nil {
			return err
		}

		if _, err := writer.Write(payload); err != nil {
			_ = closeFn()
			return errs.Wrap(err, "write quality export output")
		}
		if err := closeFn(); err != nil {
			return errs.Wrap(err, "close quality export output")
		}
		return nil
	}),
}

type qualityExportItem struct {
	QualityEventID  uint64   `json:"quality_event_id"`
	IdempotencyKey  string   `json:"idempotency_key"`
	Source          string   `json:"source"`
	ExternalEventID string   `json:"external_event_id"`
	Category        string   `json:"category"`
	Result          string   `json:"result"`
	Actor           string   `json:"actor"`
	Summary         string   `json:"summary"`
	Evidence        []string `json:"evidence"`
	PayloadJSON     string   `json:"payload_json"`
	IngestedAt      string   `json:"ingested_at"`
}

func init() {
	outboxQualityCmd.AddCommand(outboxQualityExportCmd)

	outboxQualityExportCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxQualityExportCmd.Flags().Int("limit", 200, "Max records to export")
	outboxQualityExportCmd.Flags().String("format", "json", "Output format: json|jsonl")
	outboxQualityExportCmd.Flags().String("out", "", "Output file path (default: stdout)")
	outboxQualityExportCmd.Flags().String("since", "", "Filter events with ingested_at >= this time (RFC3339 or RFC3339Nano)")
	outboxQualityExportCmd.Flags().String("until", "", "Filter events with ingested_at <= this time (RFC3339 or RFC3339Nano)")
	_ = outboxQualityExportCmd.MarkFlagRequired("issue")
}

type qualityExportWindow struct {
	since *time.Time
	until *time.Time
}

func parseQualityExportWindow(sinceRaw string, untilRaw string) (qualityExportWindow, error) {
	var out qualityExportWindow

	since, err := parseQualityExportFlagTime("since", sinceRaw)
	if err != nil {
		return qualityExportWindow{}, err
	}
	until, err := parseQualityExportFlagTime("until", untilRaw)
	if err != nil {
		return qualityExportWindow{}, err
	}

	if since != nil && until != nil && since.After(*until) {
		return qualityExportWindow{}, fmt.Errorf("invalid time window: --since %q is after --until %q", since.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano))
	}

	out.since = since
	out.until = until
	return out, nil
}

func parseQualityExportFlagTime(flagName string, value string) (*time.Time, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil, nil
	}

	parsed, ok := parseQualityExportTime(normalized)
	if !ok {
		return nil, fmt.Errorf("invalid --%s value %q: expected RFC3339 or RFC3339Nano timestamp", flagName, normalized)
	}

	return &parsed, nil
}

func parseQualityExportTime(value string) (time.Time, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return time.Time{}, false
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, normalized)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func filterQualityEventsByIngestedAt(events []outbox.QualityEventItem, window qualityExportWindow) ([]outbox.QualityEventItem, error) {
	if window.since == nil && window.until == nil {
		return events, nil
	}

	filtered := make([]outbox.QualityEventItem, 0, len(events))
	for _, item := range events {
		ingestedAt, ok := parseQualityExportTime(item.IngestedAt)
		if !ok {
			return nil, fmt.Errorf("invalid ingested_at %q for quality_event_id=%d", strings.TrimSpace(item.IngestedAt), item.QualityEventID)
		}

		if window.since != nil && ingestedAt.Before(*window.since) {
			continue
		}
		if window.until != nil && ingestedAt.After(*window.until) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func marshalQualityExportPayload(events []outbox.QualityEventItem, format string) ([]byte, error) {
	normalized := make([]qualityExportItem, 0, len(events))
	for _, item := range events {
		normalized = append(normalized, qualityExportItem{
			QualityEventID:  item.QualityEventID,
			IdempotencyKey:  item.IdempotencyKey,
			Source:          item.Source,
			ExternalEventID: item.ExternalEventID,
			Category:        item.Category,
			Result:          item.Result,
			Actor:           item.Actor,
			Summary:         item.Summary,
			Evidence:        item.Evidence,
			PayloadJSON:     item.PayloadJSON,
			IngestedAt:      item.IngestedAt,
		})
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	switch format {
	case "json":
		if err := encoder.Encode(normalized); err != nil {
			return nil, errs.Wrap(err, "encode quality events as json")
		}
	case "jsonl":
		for _, item := range normalized {
			if err := encoder.Encode(item); err != nil {
				return nil, errs.Wrap(err, "encode quality events as jsonl")
			}
		}
	default:
		return nil, errors.New("unsupported format")
	}

	return buf.Bytes(), nil
}

func resolveQualityExportWriter(cmd *cobra.Command, outPath string) (io.Writer, func() error, error) {
	trimmed := strings.TrimSpace(outPath)
	if trimmed == "" {
		return cmd.OutOrStdout(), func() error { return nil }, nil
	}

	f, err := os.Create(trimmed)
	if err != nil {
		return nil, nil, errs.Wrapf(err, "open output file %q", trimmed)
	}
	return f, f.Close, nil
}
