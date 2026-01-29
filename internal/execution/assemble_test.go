package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/zhanggui/internal/gateway"
)

func TestAssemble_Demo04_GeneratesReportAndPPTIR(t *testing.T) {
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

	reportPath := filepath.Join(root, "revs", "r1", "deliver", "report.md")
	report, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read deliver/report.md: %v", err)
	}
	if !strings.Contains(string(report), `<a id="block-deliver-report-2"></a>`) {
		t.Fatalf("deliver/report.md missing anchor")
	}

	pptIRPath := filepath.Join(root, "revs", "r1", "deliver", "ppt_ir.json")
	b, err := os.ReadFile(pptIRPath)
	if err != nil {
		t.Fatalf("read deliver/ppt_ir.json: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatalf("ppt_ir.json invalid json: %v", err)
	}
	if _, ok := obj["schema_version"]; !ok {
		t.Fatalf("ppt_ir.json missing schema_version")
	}
}
