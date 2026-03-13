package appcmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAppLoggerWritesFile(t *testing.T) {
	dataDir := t.TempDir()

	closeFn, err := initAppLogger(dataDir, "server")
	if err != nil {
		t.Fatalf("initAppLogger error: %v", err)
	}
	defer closeFn()

	slog.Info("test log entry", "component", "unit-test")

	logPath := filepath.Join(dataDir, "logs", "server.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(content), "test log entry") {
		t.Fatalf("expected log file to contain message, got %q", string(content))
	}
}
