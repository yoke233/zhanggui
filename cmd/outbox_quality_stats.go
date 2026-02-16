package cmd

import (
	"fmt"
	"log/slog"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
)

var outboxQualityStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show quality event stats for an issue",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		limit, _ := cmd.Flags().GetInt("limit")

		items, err := svc.ListQualityEvents(ctx, issueRef, limit)
		if err != nil {
			logging.Error(ctx, "list quality events for stats failed", slog.Any("err", errs.Loggable(err)))
			return errs.Wrap(err, "list quality events for stats")
		}

		categoryCounts := map[string]int{
			"review": 0,
			"ci":     0,
		}
		resultCounts := map[string]int{
			"approved":          0,
			"changes_requested": 0,
			"pass":              0,
			"fail":              0,
		}

		latestEventAt := "-"
		if len(items) > 0 {
			fallback := strings.TrimSpace(items[0].IngestedAt)
			if fallback != "" {
				latestEventAt = fallback
			}
		}

		var latestTime time.Time
		hasLatestTime := false
		for _, item := range items {
			category := strings.ToLower(strings.TrimSpace(item.Category))
			if _, ok := categoryCounts[category]; ok {
				categoryCounts[category]++
			}

			result := strings.ToLower(strings.TrimSpace(item.Result))
			if _, ok := resultCounts[result]; ok {
				resultCounts[result]++
			}

			parsedAt, ok := parseQualityStatsTime(item.IngestedAt)
			if !ok {
				continue
			}
			if !hasLatestTime || parsedAt.After(latestTime) {
				latestTime = parsedAt
				hasLatestTime = true
			}
		}
		if hasLatestTime {
			latestEventAt = latestTime.UTC().Format(time.RFC3339Nano)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(w, "metric\tvalue"); err != nil {
			return errs.Wrap(err, "write quality stats header")
		}
		if _, err := fmt.Fprintf(w, "total\t%d\n", len(items)); err != nil {
			return errs.Wrap(err, "write quality stats total")
		}
		if _, err := fmt.Fprintf(w, "latest_event_time\t%s\n", latestEventAt); err != nil {
			return errs.Wrap(err, "write quality stats latest time")
		}

		if _, err := fmt.Fprintln(w, ""); err != nil {
			return errs.Wrap(err, "write quality stats separator")
		}
		if _, err := fmt.Fprintln(w, "category\tcount"); err != nil {
			return errs.Wrap(err, "write quality category header")
		}
		if _, err := fmt.Fprintf(w, "review\t%d\n", categoryCounts["review"]); err != nil {
			return errs.Wrap(err, "write quality category review")
		}
		if _, err := fmt.Fprintf(w, "ci\t%d\n", categoryCounts["ci"]); err != nil {
			return errs.Wrap(err, "write quality category ci")
		}

		if _, err := fmt.Fprintln(w, ""); err != nil {
			return errs.Wrap(err, "write quality result separator")
		}
		if _, err := fmt.Fprintln(w, "result\tcount"); err != nil {
			return errs.Wrap(err, "write quality result header")
		}
		if _, err := fmt.Fprintf(w, "approved\t%d\n", resultCounts["approved"]); err != nil {
			return errs.Wrap(err, "write quality result approved")
		}
		if _, err := fmt.Fprintf(w, "changes_requested\t%d\n", resultCounts["changes_requested"]); err != nil {
			return errs.Wrap(err, "write quality result changes requested")
		}
		if _, err := fmt.Fprintf(w, "pass\t%d\n", resultCounts["pass"]); err != nil {
			return errs.Wrap(err, "write quality result pass")
		}
		if _, err := fmt.Fprintf(w, "fail\t%d\n", resultCounts["fail"]); err != nil {
			return errs.Wrap(err, "write quality result fail")
		}

		if err := w.Flush(); err != nil {
			return errs.Wrap(err, "flush quality stats output")
		}
		return nil
	}),
}

func init() {
	outboxQualityCmd.AddCommand(outboxQualityStatsCmd)

	outboxQualityStatsCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxQualityStatsCmd.Flags().Int("limit", 200, "Max records to aggregate")
	_ = outboxQualityStatsCmd.MarkFlagRequired("issue")
}

func parseQualityStatsTime(value string) (time.Time, bool) {
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
