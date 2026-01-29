package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestRunCmd_WorkflowDemo04(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })

	baseDir := t.TempDir()

	cmd := NewRunCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	cmd.SetArgs([]string{
		"--base-dir", baseDir,
		"--sandbox-mode", "local",
		"--workflow", "demo04",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}

	taskDir := strings.TrimSpace(out.String())
	if taskDir == "" {
		t.Fatalf("expected task dir on stdout")
	}

	if _, err := os.Stat(filepath.Join(taskDir, "revs", "r1", "deliver", "report.md")); err != nil {
		t.Fatalf("missing deliver/report.md: %v", err)
	}
}
