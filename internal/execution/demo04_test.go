package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/verify"
)

func TestDemo04_Run_GeneratesReport(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "logs"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "revs", "r1"), 0o755)

	aud, err := gateway.NewAuditor(filepath.Join(root, "logs", "tool_audit.jsonl"))
	if err != nil {
		t.Fatalf("NewAuditor: %v", err)
	}
	t.Cleanup(func() { _ = aud.Close() })

	gw, err := gateway.New(root, gateway.Actor{AgentID: "taskctl", Role: "system"}, gateway.Linkage{TaskID: "t1", RunID: "r1", Rev: "r1"}, gateway.Policy{
		AllowedWritePrefixes: []string{"revs/", "logs/"},
	}, aud)
	if err != nil {
		t.Fatalf("gateway.New: %v", err)
	}

	w := NewDemo04Workflow()
	res, err := w.Run(Context{Ctx: context.Background(), GW: gw, TaskID: "t1", RunID: "r1", Rev: "r1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.HasBlocker() {
		t.Fatalf("unexpected blockers: %+v", res.Issues)
	}

	if _, err := os.Stat(filepath.Join(root, "revs", "r1", "deliver", "report.md")); err != nil {
		t.Fatalf("missing deliver/report.md: %v", err)
	}
	for _, rel := range []string{
		filepath.Join("revs", "r1", "deliver", "ppt_ir.json"),
		filepath.Join("revs", "r1", "deliver", "ppt_renderer_input.json"),
		filepath.Join("revs", "r1", "deliver", "slides.html"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("missing %s: %v", filepath.ToSlash(rel), err)
		}
	}

	issuesPath := filepath.Join(root, "revs", "r1", "issues.json")
	b, err := os.ReadFile(issuesPath)
	if err != nil {
		t.Fatalf("missing rev/issues.json: %v", err)
	}
	var issues verify.IssuesFile
	if err := json.Unmarshal(b, &issues); err != nil {
		t.Fatalf("rev/issues.json invalid json: %v", err)
	}
	for _, it := range issues.Issues {
		if it.Severity == "blocker" {
			t.Fatalf("unexpected blocker in issues.json: %+v", it)
		}
	}

	summaries, err := filepath.Glob(filepath.Join(root, "revs", "r1", "mpus", "*", "summary.md"))
	if err != nil {
		t.Fatalf("glob mpu summaries: %v", err)
	}
	if len(summaries) == 0 {
		t.Fatalf("expected at least 1 mpu summary.md")
	}
}
