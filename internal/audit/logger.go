package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

type Config struct {
	Enabled        bool
	RootDir        string
	RedactionLevel string
}

type Scope struct {
	WorkItemID int64
	ActionID   int64
	RunID      int64
}

type Logger struct {
	store    core.ToolCallAuditStore
	cfg      Config
	redactor *Redactor
	exporter Exporter
}

type RunSink struct {
	logger *Logger
	scope  Scope
}

func ResolveRootDir(dataDir string, fallbackDir string) string {
	trimmed := strings.TrimSpace(fallbackDir)
	if trimmed == "" {
		trimmed = filepath.Join("audit", "tool-calls")
	}
	if filepath.IsAbs(trimmed) || strings.TrimSpace(dataDir) == "" {
		return filepath.Clean(trimmed)
	}
	return filepath.Join(dataDir, trimmed)
}

func NewLogger(store core.ToolCallAuditStore, cfg Config) *Logger {
	return NewLoggerWithExporter(store, cfg, nil)
}

func NewLoggerWithExporter(store core.ToolCallAuditStore, cfg Config, exporter Exporter) *Logger {
	cfg.RootDir = filepath.Clean(strings.TrimSpace(cfg.RootDir))
	if exporter == nil {
		exporter = NewFileExporter(cfg.RootDir)
	}
	return &Logger{
		store:    store,
		cfg:      cfg,
		redactor: NewRedactor(cfg.RedactionLevel),
		exporter: exporter,
	}
}

func (l *Logger) NewRunSink(scope Scope) *RunSink {
	return &RunSink{logger: l, scope: scope}
}

func (s *RunSink) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	if s == nil || s.logger == nil {
		return nil
	}
	if err := s.logger.handleSessionUpdate(ctx, s.scope, update); err != nil {
		slog.Warn("audit: failed to persist tool call audit", "run_id", s.scope.RunID, "type", update.Type, "error", err)
	}
	return nil
}

func (l *Logger) LogExecutionAudit(ctx context.Context, scope Scope, kind string, status string, data map[string]any) string {
	if l == nil {
		return ""
	}
	logRef, err := l.logExecutionAudit(ctx, scope, kind, status, data)
	if err != nil {
		slog.Warn("audit: failed to persist execution audit", "run_id", scope.RunID, "kind", kind, "status", status, "error", err)
		return ""
	}
	return logRef
}

func (l *Logger) handleSessionUpdate(ctx context.Context, scope Scope, update acpclient.SessionUpdate) error {
	if l == nil || !l.cfg.Enabled || l.store == nil {
		return nil
	}
	switch update.Type {
	case "tool_call":
		return l.handleToolCallStarted(ctx, scope, update)
	case "tool_call_update":
		if !isTerminalToolStatus(update.Status) {
			return nil
		}
		return l.handleToolCallFinished(ctx, scope, update)
	default:
		return nil
	}
}

func (l *Logger) logExecutionAudit(ctx context.Context, scope Scope, kind string, status string, data map[string]any) (string, error) {
	if l == nil || !l.cfg.Enabled || l.exporter == nil || l.cfg.RootDir == "." || strings.TrimSpace(l.cfg.RootDir) == "" {
		return "", nil
	}
	now := time.Now().UTC()
	logRef := buildExecutionAuditLogRef(scope.RunID, now)
	record := ExecutionAuditRecord{
		EventName:      "execution.audit",
		WorkItemID:     scope.WorkItemID,
		ActionID:       scope.ActionID,
		RunID:          scope.RunID,
		Kind:           strings.TrimSpace(kind),
		Status:         strings.TrimSpace(status),
		RedactionLevel: l.redactor.Level(),
		Data:           redactAuditData(l.redactor, data),
		CreatedAt:      now,
	}
	if err := l.exporter.ExportExecutionAudit(ctx, logRef, []ExecutionAuditRecord{record}); err != nil {
		return "", err
	}
	return logRef, nil
}

type toolCallStartedPayload struct {
	Title      string          `json:"title"`
	ToolCallID string          `json:"toolCallId"`
	Status     string          `json:"status"`
	RawInput   json.RawMessage `json:"rawInput"`
}

type toolCallFinishedPayload struct {
	Title      string          `json:"title"`
	ToolCallID string          `json:"toolCallId"`
	Status     string          `json:"status"`
	RawOutput  json.RawMessage `json:"rawOutput"`
}

type rawOutputSummary struct {
	ExitCode  *int   `json:"exit_code,omitempty"`
	ExitCode2 *int   `json:"exitCode,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
}

func (l *Logger) handleToolCallStarted(ctx context.Context, scope Scope, update acpclient.SessionUpdate) error {
	var parsed toolCallStartedPayload
	if err := json.Unmarshal(update.RawJSON, &parsed); err != nil {
		return fmt.Errorf("decode tool_call start: %w", err)
	}
	toolCallID := strings.TrimSpace(parsed.ToolCallID)
	if toolCallID == "" {
		return nil
	}

	if existing, err := l.store.GetToolCallAuditByToolCallID(ctx, scope.RunID, toolCallID); err == nil && existing != nil {
		return nil
	} else if err != nil && err != core.ErrNotFound {
		return err
	}

	now := time.Now().UTC()
	rawInput := compactJSON(parsed.RawInput)
	redactedInput := l.redactor.Redact(rawInput)

	audit := &core.ToolCallAudit{
		WorkItemID:     scope.WorkItemID,
		ActionID:       scope.ActionID,
		RunID:          scope.RunID,
		SessionID:      strings.TrimSpace(update.SessionID),
		ToolCallID:     toolCallID,
		ToolName:       strings.TrimSpace(parsed.Title),
		Status:         normalizeToolStatus(update.Status, "started"),
		StartedAt:      &now,
		InputDigest:    digestString(rawInput),
		InputPreview:   preview(redactedInput),
		RedactionLevel: l.redactor.Level(),
		CreatedAt:      now,
	}
	_, err := l.store.CreateToolCallAudit(ctx, audit)
	return err
}

func (l *Logger) handleToolCallFinished(ctx context.Context, scope Scope, update acpclient.SessionUpdate) error {
	var parsed toolCallFinishedPayload
	if err := json.Unmarshal(update.RawJSON, &parsed); err != nil {
		return fmt.Errorf("decode tool_call finish: %w", err)
	}
	toolCallID := strings.TrimSpace(parsed.ToolCallID)
	if toolCallID == "" {
		return nil
	}

	existing, err := l.store.GetToolCallAuditByToolCallID(ctx, scope.RunID, toolCallID)
	if err != nil && err != core.ErrNotFound {
		return err
	}

	now := time.Now().UTC()
	audit := &core.ToolCallAudit{
		WorkItemID:     scope.WorkItemID,
		ActionID:       scope.ActionID,
		RunID:          scope.RunID,
		SessionID:      strings.TrimSpace(update.SessionID),
		ToolCallID:     toolCallID,
		ToolName:       strings.TrimSpace(parsed.Title),
		Status:         normalizeToolStatus(update.Status, "completed"),
		FinishedAt:     &now,
		RedactionLevel: l.redactor.Level(),
		CreatedAt:      now,
	}
	if existing != nil {
		audit = existing
		audit.Status = normalizeToolStatus(update.Status, audit.Status)
		audit.FinishedAt = &now
		if strings.TrimSpace(audit.SessionID) == "" {
			audit.SessionID = strings.TrimSpace(update.SessionID)
		}
		if strings.TrimSpace(audit.ToolName) == "" {
			audit.ToolName = strings.TrimSpace(parsed.Title)
		}
	}

	rawOutput := compactJSON(parsed.RawOutput)
	redactedOutput := l.redactor.Redact(rawOutput)
	audit.OutputDigest = digestString(rawOutput)
	audit.OutputPreview = preview(redactedOutput)

	var summary rawOutputSummary
	_ = json.Unmarshal(parsed.RawOutput, &summary)
	exitCode := summary.ExitCode
	if exitCode == nil {
		exitCode = summary.ExitCode2
	}
	audit.ExitCode = exitCode

	redactedStdout := l.redactor.Redact(summary.Stdout)
	redactedStderr := l.redactor.Redact(summary.Stderr)
	audit.StdoutDigest = digestString(summary.Stdout)
	audit.StderrDigest = digestString(summary.Stderr)
	audit.StdoutPreview = preview(redactedStdout)
	audit.StderrPreview = preview(redactedStderr)
	if audit.StartedAt != nil {
		audit.DurationMs = now.Sub(*audit.StartedAt).Milliseconds()
	}

	if existing == nil {
		if _, err := l.store.CreateToolCallAudit(ctx, audit); err != nil {
			return err
		}
		return nil
	}
	return l.store.UpdateToolCallAudit(ctx, audit)
}

func resolveLogPath(rootDir, logRef string) (string, error) {
	trimmedRoot := filepath.Clean(strings.TrimSpace(rootDir))
	if trimmedRoot == "." || trimmedRoot == "" {
		return "", fmt.Errorf("audit root dir is not configured")
	}
	trimmedRef := strings.TrimSpace(logRef)
	if trimmedRef == "" {
		return "", fmt.Errorf("empty audit log_ref")
	}
	if filepath.IsAbs(trimmedRef) {
		return "", fmt.Errorf("audit log_ref must be relative")
	}
	cleanRef := filepath.Clean(filepath.FromSlash(trimmedRef))
	if strings.HasPrefix(cleanRef, "..") {
		return "", fmt.Errorf("invalid audit log_ref %q", logRef)
	}
	return filepath.Join(trimmedRoot, cleanRef), nil
}

func normalizeToolStatus(raw string, fallback string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func isTerminalToolStatus(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "completed", "failed", "cancelled", "canceled", "errored", "error":
		return true
	default:
		return false
	}
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err == nil {
		return buf.String()
	}
	return strings.TrimSpace(string(raw))
}

func digestString(raw string) string {
	if raw == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func preview(raw string) string {
	const maxPreview = 512
	if len(raw) <= maxPreview {
		return raw
	}
	return raw[:maxPreview] + "...(truncated)"
}

func redactAuditData(redactor *Redactor, data map[string]any) map[string]any {
	if len(data) == 0 {
		return nil
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = redactAuditValue(redactor, k, v)
	}
	return out
}

func redactAuditValue(redactor *Redactor, key string, value any) any {
	switch typed := value.(type) {
	case string:
		if isSensitiveAuditKey(key) {
			return "[REDACTED]"
		}
		return redactor.Redact(typed)
	case map[string]any:
		return redactAuditData(redactor, typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactAuditValue(redactor, key, item))
		}
		return out
	default:
		return value
	}
}

func isSensitiveAuditKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "authorization", "api_key", "apikey", "token", "password", "secret", "cookie":
		return true
	default:
		return false
	}
}

type Redactor struct {
	level    string
	patterns []*regexp.Regexp
}

func NewRedactor(level string) *Redactor {
	return &Redactor{
		level: strings.TrimSpace(level),
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)("?authorization"?\s*[:=]\s*"?)([^",;]+)("?)`),
			regexp.MustCompile(`(?i)("?(?:api[_-]?key|token|password|secret|cookie)"?\s*[:=]\s*"?)([^"\s,;]+)("?)`),
			regexp.MustCompile(`(?i)(bearer\s+)([a-z0-9._\-]+)`),
		},
	}
}

func (r *Redactor) Level() string {
	if r == nil || strings.TrimSpace(r.level) == "" {
		return "basic"
	}
	return r.level
}

func (r *Redactor) Redact(raw string) string {
	if r == nil || raw == "" {
		return raw
	}
	redacted := raw
	for _, pattern := range r.patterns {
		redacted = pattern.ReplaceAllString(redacted, `${1}[REDACTED]${3}`)
	}
	return redacted
}
