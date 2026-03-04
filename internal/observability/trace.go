package observability

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type traceContextKey struct{}

var traceCounter uint64

// WithTraceID attaches trace_id to context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	trimmed := strings.TrimSpace(traceID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, traceContextKey{}, trimmed)
}

// TraceID extracts trace_id from context.
func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	raw := ctx.Value(traceContextKey{})
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}

// TraceIDFromWebhook resolves trace id from preferred input and delivery id.
func TraceIDFromWebhook(preferredTraceID string, deliveryID string) string {
	if traceID := strings.TrimSpace(preferredTraceID); traceID != "" {
		return traceID
	}
	if delivery := strings.TrimSpace(deliveryID); delivery != "" {
		return delivery
	}
	return NewTraceID()
}

// EnsureTraceID ensures context has trace id and returns normalized value.
func EnsureTraceID(ctx context.Context, fallback string) (context.Context, string) {
	if existing := TraceID(ctx); existing != "" {
		return ctx, existing
	}
	traceID := strings.TrimSpace(fallback)
	if traceID == "" {
		traceID = NewTraceID()
	}
	return WithTraceID(ctx, traceID), traceID
}

// EventDataWithTrace clones event payload and appends trace_id.
func EventDataWithTrace(data map[string]string, traceID string) map[string]string {
	trimmed := strings.TrimSpace(traceID)
	if len(data) == 0 && trimmed == "" {
		return nil
	}

	out := make(map[string]string, len(data)+1)
	for k, v := range data {
		out[k] = v
	}
	if trimmed != "" {
		out["trace_id"] = trimmed
	}
	return out
}

type StructuredLogInput struct {
	TraceID     string
	ProjectID   string
	RunID       string
	IssueNumber int
	Operation   string
	Latency     time.Duration
}

// StructuredLogFields returns normalized structured log field map.
func StructuredLogFields(input StructuredLogInput) map[string]any {
	fields := map[string]any{
		"trace_id":   strings.TrimSpace(input.TraceID),
		"project_id": strings.TrimSpace(input.ProjectID),
		"Run_id":     strings.TrimSpace(input.RunID),
		"op":         strings.TrimSpace(input.Operation),
		"latency_ms": input.Latency.Milliseconds(),
	}
	if input.IssueNumber > 0 {
		fields["issue_number"] = input.IssueNumber
	}
	return fields
}

// StructuredLogArgs converts structured map to slog key/value args.
func StructuredLogArgs(input StructuredLogInput) []any {
	fields := StructuredLogFields(input)
	issueRaw, hasIssue := fields["issue_number"]
	args := make([]any, 0, 12)
	args = append(args,
		"trace_id", fields["trace_id"],
		"project_id", fields["project_id"],
		"Run_id", fields["Run_id"],
	)
	if hasIssue {
		args = append(args, "issue_number", issueRaw)
	}
	args = append(args,
		"op", fields["op"],
		"latency_ms", fields["latency_ms"],
	)
	return args
}

// NewTraceID generates lightweight unique trace identifier.
func NewTraceID() string {
	now := time.Now().UTC().UnixNano()
	seq := atomic.AddUint64(&traceCounter, 1)
	return "trace-" + strconv.FormatInt(now, 36) + "-" + strconv.FormatUint(seq, 36)
}
