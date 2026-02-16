package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"zhanggui/internal/bootstrap"
	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/errs"
	"zhanggui/internal/usecase/outbox"
)

type qualityBatchStats struct {
	Total     int
	Success   int
	Duplicate int
	Failed    int
}

type qualityBatchFailureRecord struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

var outboxQualityIngestBatchCmd = &cobra.Command{
	Use:   "ingest-batch",
	Short: "Batch ingest quality events from payload files",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		ctx := logging.WithAttrs(cmd.Context(), slog.String("command", cmd.CommandPath()))

		issueRef, _ := cmd.Flags().GetString("issue")
		source, _ := cmd.Flags().GetString("source")
		payloadDir, _ := cmd.Flags().GetString("dir")
		globPattern, _ := cmd.Flags().GetString("glob")
		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		failedOutPath, _ := cmd.Flags().GetString("failed-out")

		files, err := collectQualityBatchFiles(payloadDir, globPattern)
		if err != nil {
			return err
		}

		stats := qualityBatchStats{
			Total: len(files),
		}

		failedOutFile, failedOutEncoder, err := openQualityBatchFailedOut(failedOutPath)
		if err != nil {
			return err
		}
		if failedOutFile != nil {
			defer func() {
				_ = failedOutFile.Close()
			}()
		}

		for _, payloadFile := range files {
			payload, err := os.ReadFile(payloadFile)
			if err != nil {
				stats.Failed++
				logging.Error(ctx, "read quality batch payload failed", slog.String("path", payloadFile), slog.Any("err", errs.Loggable(err)))
				if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "quality ingest failed: file=%s err=%v\n", payloadFile, err); writeErr != nil {
					return errs.Wrap(writeErr, "write quality ingest-batch error output")
				}
				if writeErr := writeQualityBatchFailedOut(failedOutEncoder, payloadFile, err); writeErr != nil {
					return writeErr
				}
				if !continueOnError {
					break
				}
				continue
			}

			out, err := svc.IngestQualityEvent(ctx, outbox.IngestQualityEventInput{
				IssueRef: issueRef,
				Source:   source,
				Payload:  string(payload),
			})
			if err != nil {
				stats.Failed++
				logging.Error(ctx, "ingest quality batch payload failed", slog.String("path", payloadFile), slog.Any("err", errs.Loggable(err)))
				if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "quality ingest failed: file=%s err=%v\n", payloadFile, err); writeErr != nil {
					return errs.Wrap(writeErr, "write quality ingest-batch error output")
				}
				if writeErr := writeQualityBatchFailedOut(failedOutEncoder, payloadFile, err); writeErr != nil {
					return writeErr
				}
				if !continueOnError {
					break
				}
				continue
			}

			if out.Duplicate {
				stats.Duplicate++
				continue
			}
			stats.Success++
		}

		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"quality ingest-batch summary: issue=%s source=%s total=%d success=%d duplicate=%d failed=%d\n",
			issueRef,
			source,
			stats.Total,
			stats.Success,
			stats.Duplicate,
			stats.Failed,
		); err != nil {
			return errs.Wrap(err, "write quality ingest-batch summary")
		}

		if stats.Failed > 0 {
			return fmt.Errorf("quality ingest-batch completed with %d failed file(s)", stats.Failed)
		}

		return nil
	}),
}

func init() {
	outboxQualityCmd.AddCommand(outboxQualityIngestBatchCmd)

	outboxQualityIngestBatchCmd.Flags().String("issue", "", "IssueRef, for example local#12")
	outboxQualityIngestBatchCmd.Flags().String("source", "manual", "Quality event source (for example github/gitlab/manual)")
	outboxQualityIngestBatchCmd.Flags().String("dir", "", "Payload directory path")
	outboxQualityIngestBatchCmd.Flags().String("glob", "*.json", "Payload file glob pattern")
	outboxQualityIngestBatchCmd.Flags().Bool("continue-on-error", true, "Continue processing remaining files when a payload fails")
	outboxQualityIngestBatchCmd.Flags().String("failed-out", "", "Optional JSONL output path for failed payload files")
	_ = outboxQualityIngestBatchCmd.MarkFlagRequired("issue")
	_ = outboxQualityIngestBatchCmd.MarkFlagRequired("dir")
}

func openQualityBatchFailedOut(pathValue string) (*os.File, *json.Encoder, error) {
	normalizedPath := strings.TrimSpace(pathValue)
	if normalizedPath == "" {
		return nil, nil, nil
	}

	file, err := os.Create(normalizedPath)
	if err != nil {
		return nil, nil, errs.Wrapf(err, "create failed-out file %q", normalizedPath)
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	return file, encoder, nil
}

func writeQualityBatchFailedOut(encoder *json.Encoder, payloadFile string, ingestErr error) error {
	if encoder == nil {
		return nil
	}

	record := qualityBatchFailureRecord{
		File:  payloadFile,
		Error: ingestErr.Error(),
	}
	if err := encoder.Encode(record); err != nil {
		return errs.Wrapf(err, "write failed-out record for file %q", payloadFile)
	}
	return nil
}

func collectQualityBatchFiles(dir string, globPattern string) ([]string, error) {
	normalizedDir := strings.TrimSpace(dir)
	if normalizedDir == "" {
		return nil, fmt.Errorf("dir is required")
	}

	info, err := os.Stat(normalizedDir)
	if err != nil {
		return nil, errs.Wrapf(err, "stat dir %q", normalizedDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("dir %q is not a directory", normalizedDir)
	}

	pattern := strings.TrimSpace(globPattern)
	if pattern == "" {
		pattern = "*.json"
	}
	if _, err := filepath.Match(pattern, "probe"); err != nil {
		return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
	}

	files := make([]string, 0)
	if err := filepath.WalkDir(normalizedDir, func(pathValue string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(normalizedDir, pathValue)
		if relErr != nil {
			return relErr
		}

		matched, matchErr := matchBatchPattern(pattern, d.Name(), relPath)
		if matchErr != nil {
			return matchErr
		}
		if matched {
			files = append(files, pathValue)
		}
		return nil
	}); err != nil {
		return nil, errs.Wrapf(err, "walk payload dir %q", normalizedDir)
	}

	sort.Strings(files)
	return files, nil
}

func matchBatchPattern(pattern string, fileName string, relPath string) (bool, error) {
	matched, err := filepath.Match(pattern, fileName)
	if err != nil {
		return false, err
	}
	if matched {
		return true, nil
	}

	matched, err = filepath.Match(pattern, relPath)
	if err != nil {
		return false, err
	}
	if matched {
		return true, nil
	}

	slashPattern := filepath.ToSlash(pattern)
	slashRelPath := filepath.ToSlash(relPath)
	matched, err = path.Match(slashPattern, slashRelPath)
	if err != nil {
		return false, err
	}
	return matched, nil
}
