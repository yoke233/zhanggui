package logging

import (
	"context"
	"log/slog"
	"os"
	"sync"
)

type ctxLoggerKey struct{}
type ctxAttrsKey struct{}

var (
	defaultLogger     *slog.Logger
	defaultLoggerOnce sync.Once
)

func baseLogger(ctx context.Context) *slog.Logger {
	_ = ctx

	defaultLoggerOnce.Do(func() {
		defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	})

	return defaultLogger
}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		return ctx
	}

	return context.WithValue(ctx, ctxLoggerKey{}, logger)
}

func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(attrs) == 0 {
		return ctx
	}

	current := Attrs(ctx)
	next := mergeAttrs(current, attrs)

	return context.WithValue(ctx, ctxAttrsKey{}, next)
}

func WithTelemetry(ctx context.Context, traceID string, spanID string) context.Context {
	attrs := make([]slog.Attr, 0, 2)
	if traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	if spanID != "" {
		attrs = append(attrs, slog.String("span_id", spanID))
	}

	return WithAttrs(ctx, attrs...)
}

func Logger(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(ctxLoggerKey{}).(*slog.Logger); ok && logger != nil {
			return logger
		}
	}

	return baseLogger(ctx)
}

func Attrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}

	attrs, ok := ctx.Value(ctxAttrsKey{}).([]slog.Attr)
	if !ok || len(attrs) == 0 {
		return nil
	}

	cloned := make([]slog.Attr, len(attrs))
	copy(cloned, attrs)
	return cloned
}

func Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelInfo, msg, attrs...)
}

func Warn(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelWarn, msg, attrs...)
}

func Error(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelError, msg, attrs...)
}

func log(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	all := mergeAttrs(Attrs(ctx), attrs)
	Logger(ctx).LogAttrs(ctx, level, msg, all...)
}

func mergeAttrs(base []slog.Attr, extra []slog.Attr) []slog.Attr {
	if len(base) == 0 {
		cloned := make([]slog.Attr, len(extra))
		copy(cloned, extra)
		return cloned
	}
	if len(extra) == 0 {
		cloned := make([]slog.Attr, len(base))
		copy(cloned, base)
		return cloned
	}

	merged := make([]slog.Attr, 0, len(base)+len(extra))
	indexByKey := make(map[string]int, len(base)+len(extra))

	for _, attr := range base {
		merged = append(merged, attr)
		if attr.Key != "" {
			indexByKey[attr.Key] = len(merged) - 1
		}
	}

	for _, attr := range extra {
		if attr.Key != "" {
			if idx, ok := indexByKey[attr.Key]; ok {
				merged[idx] = attr
				continue
			}
		}

		merged = append(merged, attr)
		if attr.Key != "" {
			indexByKey[attr.Key] = len(merged) - 1
		}
	}

	return merged
}
