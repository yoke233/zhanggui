package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ExecutionAuditRecord struct {
	EventName      string         `json:"event_name"`
	WorkItemID     int64          `json:"work_item_id,omitempty"`
	ActionID       int64          `json:"action_id,omitempty"`
	RunID          int64          `json:"run_id,omitempty"`
	Kind           string         `json:"kind"`
	Status         string         `json:"status"`
	RedactionLevel string         `json:"redaction_level,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

type Exporter interface {
	ExportExecutionAudit(ctx context.Context, logRef string, records []ExecutionAuditRecord) error
}

type FileExporter struct {
	rootDir string
}

func NewFileExporter(rootDir string) *FileExporter {
	return &FileExporter{rootDir: filepath.Clean(strings.TrimSpace(rootDir))}
}

func (e *FileExporter) ExportExecutionAudit(_ context.Context, logRef string, records []ExecutionAuditRecord) error {
	return writeJSONLRecords(e.rootDir, logRef, records)
}

func buildExecutionAuditLogRef(runID int64, now time.Time) string {
	return filepath.ToSlash(filepath.Join(
		now.Format("2006"),
		now.Format("01"),
		now.Format("02"),
		fmt.Sprintf("exec-%d-execution.jsonl", runID),
	))
}

func ReadExecutionAuditRecords(rootDir, logRef string) ([]ExecutionAuditRecord, error) {
	path, err := resolveLogPath(rootDir, logRef)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records := make([]ExecutionAuditRecord, 0)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var record ExecutionAuditRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("decode execution audit record: %w", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan execution audit file: %w", err)
	}
	return records, nil
}

func writeJSONLRecords[T any](rootDir, logRef string, records []T) error {
	if len(records) == 0 {
		return nil
	}
	path, err := resolveLogPath(rootDir, logRef)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir audit payload dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open audit payload file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, record := range records {
		if err := enc.Encode(record); err != nil {
			return fmt.Errorf("write audit payload record: %w", err)
		}
	}
	return nil
}
