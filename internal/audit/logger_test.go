package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestLogger_HandleToolCallLifecycle_PersistsAuditSummary(t *testing.T) {
	store, scope := newAuditLoggerTestStore(t)
	logger := NewLogger(store, Config{
		Enabled:        true,
		RootDir:        t.TempDir(),
		RedactionLevel: "basic",
	})
	sink := logger.NewRunSink(scope)

	ctx := context.Background()
	start := acpclient.SessionUpdate{
		SessionID: "session-1",
		Type:      "tool_call",
		Status:    "started",
		RawJSON: mustMarshalRawJSON(t, map[string]any{
			"title":      "functions.shell_command",
			"toolCallId": "call-1",
			"status":     "started",
			"rawInput": map[string]any{
				"token": "sk-test-123",
				"query": "hello",
			},
		}),
	}
	if err := sink.HandleSessionUpdate(ctx, start); err != nil {
		t.Fatalf("handle start update: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	finish := acpclient.SessionUpdate{
		SessionID: "session-1",
		Type:      "tool_call_update",
		Status:    "completed",
		RawJSON: mustMarshalRawJSON(t, map[string]any{
			"title":      "functions.shell_command",
			"toolCallId": "call-1",
			"status":     "completed",
			"rawOutput": map[string]any{
				"exit_code": 7,
				"stdout":    "Authorization: Bearer abc.def.ghi",
				"stderr":    "password=secret-pass",
				"result":    "ok",
			},
		}),
	}
	if err := sink.HandleSessionUpdate(ctx, finish); err != nil {
		t.Fatalf("handle finish update: %v", err)
	}

	auditItem, err := store.GetToolCallAuditByToolCallID(ctx, scope.RunID, "call-1")
	if err != nil {
		t.Fatalf("get audit by tool_call_id: %v", err)
	}
	if auditItem.Status != "completed" {
		t.Fatalf("status = %q, want completed", auditItem.Status)
	}
	if auditItem.StartedAt == nil || auditItem.FinishedAt == nil {
		t.Fatalf("expected started_at and finished_at to be set: %+v", auditItem)
	}
	if auditItem.DurationMs <= 0 {
		t.Fatalf("duration_ms = %d, want > 0", auditItem.DurationMs)
	}
	if auditItem.ExitCode == nil || *auditItem.ExitCode != 7 {
		t.Fatalf("exit_code = %v, want 7", auditItem.ExitCode)
	}
	if auditItem.InputDigest == "" || auditItem.OutputDigest == "" {
		t.Fatalf("expected input/output digests: %+v", auditItem)
	}
	if auditItem.StdoutDigest == "" || auditItem.StderrDigest == "" {
		t.Fatalf("expected stdout/stderr digests: %+v", auditItem)
	}
	if auditItem.RedactionLevel != "basic" {
		t.Fatalf("redaction_level = %q, want basic", auditItem.RedactionLevel)
	}
	if strings.Contains(auditItem.InputPreview, "sk-test-123") {
		t.Fatalf("input preview leaked secret: %q", auditItem.InputPreview)
	}
	if strings.Contains(auditItem.StdoutPreview, "abc.def.ghi") {
		t.Fatalf("stdout preview leaked token: %q", auditItem.StdoutPreview)
	}
	if strings.Contains(auditItem.StderrPreview, "secret-pass") {
		t.Fatalf("stderr preview leaked password: %q", auditItem.StderrPreview)
	}
}

func TestLogger_LogExecutionAudit_PersistsLocalFile(t *testing.T) {
	rootDir := filepath.Join(t.TempDir(), "execution-audit")
	logger := NewLogger(nil, Config{
		Enabled:        true,
		RootDir:        rootDir,
		RedactionLevel: "basic",
	})

	logRef := logger.LogExecutionAudit(context.Background(), Scope{
		WorkItemID: 101,
		ActionID:   202,
		RunID:      303,
	}, "execution.watch", "failed", map[string]any{
		"error": "Authorization: Bearer abc.def.ghi",
		"nested": map[string]any{
			"token": "sk-live-secret",
		},
	})
	if logRef == "" {
		t.Fatal("expected non-empty execution audit log_ref")
	}

	logPath := filepath.Join(rootDir, filepath.FromSlash(logRef))
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("stat execution audit log %s: %v", logPath, err)
	}

	records, err := ReadExecutionAuditRecords(rootDir, logRef)
	if err != nil {
		t.Fatalf("read execution audit records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.EventName != "execution.audit" {
		t.Fatalf("event_name = %q, want execution.audit", record.EventName)
	}
	if record.WorkItemID != 101 || record.ActionID != 202 || record.RunID != 303 {
		t.Fatalf("unexpected scope in record: %+v", record)
	}
	if record.Kind != "execution.watch" || record.Status != "failed" {
		t.Fatalf("unexpected execution audit record: %+v", record)
	}
	if record.RedactionLevel != "basic" {
		t.Fatalf("redaction_level = %q, want basic", record.RedactionLevel)
	}
	errorText, _ := record.Data["error"].(string)
	if strings.Contains(errorText, "abc.def.ghi") {
		t.Fatalf("error text leaked bearer token: %q", errorText)
	}
	nested, _ := record.Data["nested"].(map[string]any)
	token, _ := nested["token"].(string)
	if strings.Contains(token, "sk-live-secret") {
		t.Fatalf("nested token leaked secret: %q", token)
	}
}

func newAuditLoggerTestStore(t *testing.T) (*sqlite.Store, Scope) {
	t.Helper()

	store, err := sqlite.New(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "audit work item",
		Status:   core.WorkItemOpen,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "audit action",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	runID, err := store.CreateRun(ctx, &core.Run{
		ActionID:   actionID,
		WorkItemID: workItemID,
		Status:     core.RunCreated,
		Attempt:    1,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	return store, Scope{
		WorkItemID: workItemID,
		ActionID:   actionID,
		RunID:      runID,
	}
}

func mustMarshalRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}
	return data
}
