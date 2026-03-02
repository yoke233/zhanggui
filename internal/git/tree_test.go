package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerStatusFiles(t *testing.T) {
	repo := setupTreeTestRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("tracked-changed\n"), 0o644); err != nil {
		t.Fatalf("write tracked.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}

	runner := NewRunner(repo)
	items, err := runner.StatusFiles()
	if err != nil {
		t.Fatalf("StatusFiles failed: %v", err)
	}
	statuses := make(map[string]string, len(items))
	for _, item := range items {
		statuses[item.Path] = item.GitStatus
	}

	if statuses["tracked.txt"] != "M" {
		t.Fatalf("expected tracked.txt status contains M, got %q", statuses["tracked.txt"])
	}
	if statuses["new.txt"] != "?" {
		t.Fatalf("expected new.txt status ?, got %q", statuses["new.txt"])
	}
}

func TestRunnerListDirectoryIncludesTrackedAndUntracked(t *testing.T) {
	repo := setupTreeTestRepoWithCommit(t)
	if err := os.MkdirAll(filepath.Join(repo, "src", "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir src/pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "app.txt"), []byte("app\n"), 0o644); err != nil {
		t.Fatalf("write src/app.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "pkg", "lib.txt"), []byte("lib\n"), 0o644); err != nil {
		t.Fatalf("write src/pkg/lib.txt: %v", err)
	}
	runGitTreeTest(t, repo, "add", ".")
	runGitTreeTest(t, repo, "commit", "-m", "add src files")

	if err := os.WriteFile(filepath.Join(repo, "src", "app.txt"), []byte("app-modified\n"), 0o644); err != nil {
		t.Fatalf("modify src/app.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("write src/new.txt: %v", err)
	}

	runner := NewRunner(repo)
	items, err := runner.ListDirectory("src")
	if err != nil {
		t.Fatalf("ListDirectory failed: %v", err)
	}

	byPath := make(map[string]FileEntry, len(items))
	for _, item := range items {
		byPath[item.Path] = item
	}

	appFile, ok := byPath["src/app.txt"]
	if !ok {
		t.Fatalf("expected src/app.txt in items, got %#v", items)
	}
	if appFile.Type != "file" {
		t.Fatalf("expected src/app.txt type file, got %s", appFile.Type)
	}
	if !strings.Contains(appFile.GitStatus, "M") {
		t.Fatalf("expected src/app.txt status contains M, got %q", appFile.GitStatus)
	}

	newFile, ok := byPath["src/new.txt"]
	if !ok {
		t.Fatalf("expected src/new.txt in items, got %#v", items)
	}
	if newFile.GitStatus != "?" {
		t.Fatalf("expected src/new.txt status ?, got %q", newFile.GitStatus)
	}

	pkgDir, ok := byPath["src/pkg"]
	if !ok {
		t.Fatalf("expected src/pkg in items, got %#v", items)
	}
	if pkgDir.Type != "dir" {
		t.Fatalf("expected src/pkg type dir, got %s", pkgDir.Type)
	}
}

func TestRunnerListDirectoryFallbackWithoutCommit(t *testing.T) {
	repo := setupTreeTestRepoWithoutCommit(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "draft.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("write draft.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "note.md"), []byte("note\n"), 0o644); err != nil {
		t.Fatalf("write docs/note.md: %v", err)
	}

	runner := NewRunner(repo)
	items, err := runner.ListDirectory(".")
	if err != nil {
		t.Fatalf("ListDirectory failed: %v", err)
	}

	byPath := make(map[string]FileEntry, len(items))
	for _, item := range items {
		byPath[item.Path] = item
	}
	if _, ok := byPath["draft.txt"]; !ok {
		t.Fatalf("expected draft.txt in items, got %#v", items)
	}
	if docs, ok := byPath["docs"]; !ok || docs.Type != "dir" {
		t.Fatalf("expected docs directory in items, got %#v", items)
	}
	if _, ok := byPath[".git"]; ok {
		t.Fatalf("did not expect .git in items, got %#v", items)
	}
}

func TestRunnerDiffFileTrackedAndUntracked(t *testing.T) {
	repo := setupTreeTestRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("modify README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}

	runner := NewRunner(repo)

	trackedDiff, err := runner.DiffFile("README.md")
	if err != nil {
		t.Fatalf("DiffFile tracked failed: %v", err)
	}
	if !strings.Contains(trackedDiff, "README.md") {
		t.Fatalf("expected tracked diff contains README.md, got %q", trackedDiff)
	}

	untrackedDiff, err := runner.DiffFile("new.txt")
	if err != nil {
		t.Fatalf("DiffFile untracked failed: %v", err)
	}
	if !strings.Contains(untrackedDiff, "new.txt") {
		t.Fatalf("expected untracked diff contains new.txt, got %q", untrackedDiff)
	}
}

func TestRunnerDiffSummaryIncludesUntracked(t *testing.T) {
	repo := setupTreeTestRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("modify README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "extra.txt"), []byte("extra\n"), 0o644); err != nil {
		t.Fatalf("write extra.txt: %v", err)
	}

	runner := NewRunner(repo)
	summary, err := runner.DiffSummary()
	if err != nil {
		t.Fatalf("DiffSummary failed: %v", err)
	}
	if !strings.Contains(summary, "README.md") {
		t.Fatalf("expected summary contains README.md, got %q", summary)
	}
	if !strings.Contains(summary, "?? extra.txt") {
		t.Fatalf("expected summary contains untracked extra.txt, got %q", summary)
	}
}

func TestRunnerDiffFileStagedWithoutCommit(t *testing.T) {
	repo := setupTreeTestRepoWithoutCommit(t)
	if err := os.WriteFile(filepath.Join(repo, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatalf("write staged.txt: %v", err)
	}
	runGitTreeTest(t, repo, "add", "staged.txt")

	runner := NewRunner(repo)
	diffText, err := runner.DiffFile("staged.txt")
	if err != nil {
		t.Fatalf("DiffFile staged without commit failed: %v", err)
	}
	if !strings.Contains(diffText, "staged.txt") {
		t.Fatalf("expected diff contains staged.txt, got %q", diffText)
	}
}

func setupTreeTestRepoWithCommit(t *testing.T) string {
	t.Helper()
	repo := setupTreeTestRepoWithoutCommit(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatalf("write tracked.txt: %v", err)
	}
	runGitTreeTest(t, repo, "add", ".")
	runGitTreeTest(t, repo, "commit", "-m", "init")
	return repo
}

func setupTreeTestRepoWithoutCommit(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitTreeTestInDir(t, "", "init", repo)
	runGitTreeTest(t, repo, "config", "user.email", "test@example.com")
	runGitTreeTest(t, repo, "config", "user.name", "test-user")
	return repo
}

func runGitTreeTest(t *testing.T, repo string, args ...string) string {
	t.Helper()
	return runGitTreeTestInDir(t, repo, args...)
}

func runGitTreeTestInDir(t *testing.T, repo string, args ...string) string {
	t.Helper()
	gitArgs := args
	if strings.TrimSpace(repo) != "" {
		gitArgs = append([]string{"-C", repo}, args...)
	}
	cmd := exec.Command("git", gitArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s (%v)", gitArgs, string(output), err)
	}
	return string(output)
}
