package web

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRelativePathValid(t *testing.T) {
	repoRoot := t.TempDir()
	absPath, normalizedPath, err := validateRelativePath(repoRoot, "docs\\plan.md")
	if err != nil {
		t.Fatalf("validateRelativePath returned error: %v", err)
	}

	if normalizedPath != "docs/plan.md" {
		t.Fatalf("expected normalized path docs/plan.md, got %s", normalizedPath)
	}

	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}
	wantAbsPath := filepath.Join(absRepoRoot, filepath.FromSlash("docs/plan.md"))
	if absPath != wantAbsPath {
		t.Fatalf("expected abs path %s, got %s", wantAbsPath, absPath)
	}
}

func TestValidateRelativePathRejectsEmptyPath(t *testing.T) {
	repoRoot := t.TempDir()
	_, _, err := validateRelativePath(repoRoot, "   ")
	if !errors.Is(err, errRelativePathRequired) {
		t.Fatalf("expected errRelativePathRequired, got %v", err)
	}
}

func TestValidateRelativePathRejectsAbsolutePath(t *testing.T) {
	repoRoot := t.TempDir()
	absolutePath := filepath.Join(repoRoot, "README.md")
	_, _, err := validateRelativePath(repoRoot, absolutePath)
	if !errors.Is(err, errInvalidRelativePath) {
		t.Fatalf("expected errInvalidRelativePath, got %v", err)
	}
}

func TestValidateRelativePathRejectsTraversal(t *testing.T) {
	repoRoot := t.TempDir()
	_, _, err := validateRelativePath(repoRoot, "../outside.md")
	if !errors.Is(err, errInvalidRelativePath) {
		t.Fatalf("expected errInvalidRelativePath, got %v", err)
	}
}

func TestValidateRelativePathRejectsWindowsVolumePath(t *testing.T) {
	repoRoot := t.TempDir()
	_, _, err := validateRelativePath(repoRoot, "C:temp\\x.txt")
	if !errors.Is(err, errInvalidRelativePath) {
		t.Fatalf("expected errInvalidRelativePath, got %v", err)
	}
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid relative path") {
		t.Fatalf("expected invalid relative path error message, got %v", err)
	}
}
