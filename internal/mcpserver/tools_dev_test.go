//go:build dev

package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDevToolSourceRoot_UsesDefaultRoot(t *testing.T) {
	root, err := resolveDevToolSourceRoot("/tmp/repo", "")
	if err != nil {
		t.Fatalf("resolveDevToolSourceRoot() error = %v, want nil", err)
	}
	if root != filepath.Clean("/tmp/repo") {
		t.Fatalf("resolveDevToolSourceRoot() = %q, want %q", root, filepath.Clean("/tmp/repo"))
	}
}

func TestResolveDevToolSourceRoot_OverrideRelativeToDefault(t *testing.T) {
	root, err := resolveDevToolSourceRoot("/workspace/repo", "../another")
	if err != nil {
		t.Fatalf("resolveDevToolSourceRoot() error = %v, want nil", err)
	}
	want := filepath.Clean("/workspace/another")
	if root != want {
		t.Fatalf("resolveDevToolSourceRoot() = %q, want %q", root, want)
	}
}

func TestResolveDevToolSourceRoot_AutoDetectFromCWD(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "cmd", "ai-flow", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/ai-flow: %v", err)
	}

	t.Chdir(filepath.Join(repoRoot, "cmd", "ai-flow", "nested"))
	root, err := resolveDevToolSourceRoot("", "")
	if err != nil {
		t.Fatalf("resolveDevToolSourceRoot() auto-detect error = %v, want nil", err)
	}
	if root != filepath.Clean(repoRoot) {
		t.Fatalf("resolveDevToolSourceRoot() auto-detect = %q, want %q", root, filepath.Clean(repoRoot))
	}
}

func TestResolveDevToolFrontendDir_DefaultWeb(t *testing.T) {
	if got := resolveDevToolFrontendDir(""); got != "web" {
		t.Fatalf("resolveDevToolFrontendDir(\"\") = %q, want %q", got, "web")
	}
}
