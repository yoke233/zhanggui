package appcmd

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// initAppLogger configures the default slog logger to write to both stderr and
// a log file under <dataDir>/logs.
func initAppLogger(dataDir string, command string) (func() error, error) {
	dataDir = strings.TrimSpace(dataDir)
	command = strings.TrimSpace(command)
	if dataDir == "" || command == "" {
		return func() error { return nil }, nil
	}

	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, err
	}

	logPath := filepath.Join(logsDir, command+".log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	handler := slog.NewTextHandler(io.MultiWriter(os.Stderr, f), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))

	return f.Close, nil
}
