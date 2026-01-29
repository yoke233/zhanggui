package execution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/zhanggui/internal/gateway"
)

func TestPPTRenderer_GeneratesSlidesHTML(t *testing.T) {
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

	htmlPath := filepath.Join(root, "revs", "r1", "deliver", "slides.html")
	html, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("missing deliver/slides.html: %v", err)
	}
	s := string(html)
	if !strings.Contains(s, "<h1>") {
		t.Fatalf("slides.html missing <h1>")
	}
	if !strings.Contains(s, "Slide: Title") {
		t.Fatalf("slides.html missing slide title")
	}
}
