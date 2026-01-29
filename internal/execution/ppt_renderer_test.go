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

func TestPPTRenderer_MissingInput_ReturnsBlocker(t *testing.T) {
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

	issues, err := RenderSlidesHTML(Context{Ctx: context.Background(), GW: gw, TaskID: "t1", RunID: "r1", Rev: "r1"})
	if err != nil {
		t.Fatalf("RenderSlidesHTML: %v", err)
	}
	found := false
	for _, it := range issues {
		if it.Severity == "blocker" && it.Where == "renderer" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected blocker where=renderer, got: %+v", issues)
	}
	if _, err := os.Stat(filepath.Join(root, "revs", "r1", "deliver", "slides.html")); err == nil {
		t.Fatalf("expected slides.html not to be created on blocker")
	}
}

func TestPPTRenderer_EscapesUserContent(t *testing.T) {
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

	in := PPTRendererInput{
		SchemaVersion: 1,
		Title:         "<script>alert(1)</script>",
		Slides: []PPTSlide{
			{Title: "<b>hi</b>", Bullets: []string{"<img src=x onerror=alert(1)>"}},
		},
	}
	b, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	b = append(b, '\n')
	if err := gw.ReplaceFile(filepath.ToSlash(filepath.Join("revs", "r1", "deliver", "ppt_renderer_input.json")), b, 0o644, "test: write ppt_renderer_input.json"); err != nil {
		t.Fatalf("ReplaceFile(ppt_renderer_input.json): %v", err)
	}

	issues, err := RenderSlidesHTML(Context{Ctx: context.Background(), GW: gw, TaskID: "t1", RunID: "r1", Rev: "r1"})
	if err != nil {
		t.Fatalf("RenderSlidesHTML: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got: %+v", issues)
	}

	htmlPath := filepath.Join(root, "revs", "r1", "deliver", "slides.html")
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read slides.html: %v", err)
	}
	s := string(htmlBytes)
	if strings.Contains(s, "<script>") {
		t.Fatalf("expected script tag to be escaped")
	}
	if !strings.Contains(s, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("expected escaped title")
	}
	if strings.Contains(s, "<b>hi</b>") {
		t.Fatalf("expected slide title to be escaped")
	}
	if !strings.Contains(s, "&lt;b&gt;hi&lt;/b&gt;") {
		t.Fatalf("expected escaped slide title")
	}
}
