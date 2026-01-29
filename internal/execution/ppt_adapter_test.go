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

func TestPPTAdapter_BadJSON_WritesBlockerIssue(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "logs"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "revs", "r1", "deliver"), 0o755)

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

	// Bad JSON input for adapter.
	if err := gw.ReplaceFile(filepath.ToSlash(filepath.Join("revs", "r1", "deliver", "ppt_ir.json")), []byte("{"), 0o644, "test: write bad ppt_ir.json"); err != nil {
		t.Fatalf("ReplaceFile(ppt_ir.json): %v", err)
	}

	issues, err := AdaptPPTIRToRendererInput(Context{Ctx: context.Background(), GW: gw, TaskID: "t1", RunID: "r1", Rev: "r1"})
	if err != nil {
		t.Fatalf("AdaptPPTIRToRendererInput: %v", err)
	}

	has := false
	for _, it := range issues {
		if it.Severity == "blocker" && it.Where == "adapter" {
			has = true
			break
		}
	}
	if !has {
		t.Fatalf("expected blocker issue where=adapter, got: %+v", issues)
	}

	out := verify.IssuesFile{SchemaVersion: 1, TaskID: "t1", Rev: "r1", Issues: issues}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	b = append(b, '\n')
	if err := gw.ReplaceFile(filepath.ToSlash(filepath.Join("revs", "r1", "issues.json")), b, 0o644, "test: write issues.json"); err != nil {
		t.Fatalf("ReplaceFile(issues.json): %v", err)
	}

	var got verify.IssuesFile
	raw, err := os.ReadFile(filepath.Join(root, "revs", "r1", "issues.json"))
	if err != nil {
		t.Fatalf("ReadFile(issues.json): %v", err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("issues.json invalid json: %v", err)
	}
	found := false
	for _, it := range got.Issues {
		if it.Severity == "blocker" && it.Where == "adapter" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues.json missing blocker where=adapter: %+v", got.Issues)
	}
}
