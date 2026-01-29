package logging

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Options struct {
	Stdout  io.Writer
	LogPath string
	Level   slog.Level
}

func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func NewLogger(opts Options) (*slog.Logger, func() error, error) {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}

	var writer io.Writer = opts.Stdout
	var file *os.File
	closeFn := func() error { return nil }

	if opts.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.LogPath), 0o755); err != nil {
			return nil, nil, err
		}
		f, err := os.OpenFile(opts.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, err
		}
		file = f
		writer = io.MultiWriter(opts.Stdout, file)
		closeFn = func() error {
			if file == nil {
				return nil
			}
			return file.Close()
		}
	}

	if writer == nil {
		return nil, nil, errors.New("logger writer is nil")
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: opts.Level,
	})
	logger := slog.New(handler)
	return logger, closeFn, nil
}
