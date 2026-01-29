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

func TestRunCmd_WorkflowDemo04_WithDeliveryPlan(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })

	baseDir := t.TempDir()
	planPath := filepath.Join(t.TempDir(), "delivery_plan.yaml")
	planBytes := []byte(`
teams:
  - team_id: team_a
roles:
  - role: writer
    count: 1
  - role: designer
    count: 1
budgets:
  max_parallel: 1
`)
	if err := os.WriteFile(planPath, planBytes, 0o644); err != nil {
		t.Fatalf("write delivery plan: %v", err)
	}

	cmd := NewRunCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	cmd.SetArgs([]string{
		"--base-dir", baseDir,
		"--sandbox-mode", "local",
		"--workflow", "demo04",
		"--delivery-plan", planPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}

	taskDir := strings.TrimSpace(out.String())
	if taskDir == "" {
		t.Fatalf("expected task dir on stdout")
	}

	snapshotPath := filepath.Join(taskDir, "revs", "r1", "delivery_plan.yaml")
	b, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("missing rev delivery_plan.yaml: %v", err)
	}
	if !strings.Contains(string(b), "max_parallel: 1") {
		t.Fatalf("delivery_plan.yaml snapshot missing max_parallel: 1")
	}
}

func TestRunCmd_DeliveryPlanWithoutWorkflow_Err(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })

	baseDir := t.TempDir()
	planPath := filepath.Join(t.TempDir(), "delivery_plan.yaml")
	if err := os.WriteFile(planPath, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write delivery plan: %v", err)
	}

	cmd := NewRunCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"--base-dir", baseDir,
		"--sandbox-mode", "local",
		"--delivery-plan", planPath,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunCmd_WorkflowAndEntrypointMutualExclusive(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })

	baseDir := t.TempDir()

	cmd := NewRunCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"--base-dir", baseDir,
		"--sandbox-mode", "local",
		"--workflow", "demo04",
		"--entrypoint", "echo",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error")
	}
}
