package appdata

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveDataDir returns the runtime data directory.
// Priority: $AI_WORKFLOW_DATA_DIR > $CWD/.ai-workflow
func ResolveDataDir() (string, error) {
	if env := strings.TrimSpace(os.Getenv("AI_WORKFLOW_DATA_DIR")); env != "" {
		return filepath.Abs(env)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, ".ai-workflow"), nil
}
