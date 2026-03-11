package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSecrets_BackfillsPATsFromCodeupSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.toml")
	content := `
[codeup]
merge_pat = "codeup-merge-token"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}

	secrets, err := LoadSecrets(path)
	if err != nil {
		t.Fatalf("LoadSecrets error: %v", err)
	}
	if got := secrets.MergePAT; got != "codeup-merge-token" {
		t.Fatalf("MergePAT = %q, want codeup-merge-token", got)
	}
}
