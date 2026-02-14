package errs

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// Wrap adds context and preserves the error chain (errors.Is/As works).
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// Wrapf adds formatted context and preserves the error chain.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}

	// Append the original err as the last arg for %w.
	args = append(args, err)
	return fmt.Errorf(format+": %w", args...)
}

// WithStack captures a stack trace once (recommended: only at the root cause boundary).
// You can still wrap it later with Wrap/Wrapf.
func WithStack(err error) error {
	if err == nil {
		return nil
	}

	// If it already has stack, don't double-capture.
	var se *StackError
	if errors.As(err, &se) {
		return err
	}

	return &StackError{
		err:   err,
		stack: debug.Stack(),
	}
}

// StackError wraps an error and stores a stack trace.
type StackError struct {
	err   error
	stack []byte
}

func (e *StackError) Error() string { return e.err.Error() }
func (e *StackError) Unwrap() error { return e.err }
func (e *StackError) Stack() []byte { return e.stack }

// LogValue makes slog encode the error as structured fields.
// Usage: slog.Any("err", errs.Loggable(err))
type loggable struct{ err error }

func Loggable(err error) slog.LogValuer { return loggable{err: err} }

func (l loggable) LogValue() slog.Value {
	if l.err == nil {
		return slog.GroupValue()
	}

	chain := ErrorChainStrings(l.err)

	// Try to find stack (if present anywhere in the chain).
	var se *StackError
	hasStack := errors.As(l.err, &se)

	attrs := []slog.Attr{
		slog.String("message", l.err.Error()),
		slog.Any("chain", chain),
	}

	if hasStack {
		// Keep it as string for JSON logs.
		attrs = append(attrs, slog.String("stack", string(se.Stack())))
	}

	return slog.GroupValue(attrs...)
}

// ErrorChainStrings returns the unwrap chain as strings (outer -> inner).
func ErrorChainStrings(err error) []string {
	if err == nil {
		return nil
	}

	out := make([]string, 0, 8)
	for e := err; e != nil; e = errors.Unwrap(e) {
		out = append(out, e.Error())
	}
	return out
}
