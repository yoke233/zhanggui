package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSecrets_StrictRejectsLegacyTopLevelPATFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.toml")
	content := `
commit_pat = "legacy-token"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}

	_, err := LoadSecrets(path)
	if err == nil {
		t.Fatal("expected legacy top-level PAT fields to be rejected by strict parsing")
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Fatalf("expected strict-mode decode error, got %v", err)
	}
}

func TestLoadSecrets_NestedPATFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.toml")
	content := `
[github]
pat = "gh-pat-token"

[codeup]
pat = "codeup-pat-token"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}

	secrets, err := LoadSecrets(path)
	if err != nil {
		t.Fatalf("LoadSecrets error: %v", err)
	}
	if got := secrets.GitHub.PAT; got != "gh-pat-token" {
		t.Fatalf("GitHub.PAT = %q, want gh-pat-token", got)
	}
	if got := secrets.Codeup.PAT; got != "codeup-pat-token" {
		t.Fatalf("Codeup.PAT = %q, want codeup-pat-token", got)
	}
}
