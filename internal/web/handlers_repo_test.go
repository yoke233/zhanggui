package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/ai-workflow/internal/core"
)

func TestRepoHandlersProjectNotFound(t *testing.T) {
	store := newTestStore(t)
	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/proj-not-found/repo/status")
	if err != nil {
		t.Fatalf("GET /repo/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if apiErr.Code != "PROJECT_NOT_FOUND" {
		t.Fatalf("expected code PROJECT_NOT_FOUND, got %s", apiErr.Code)
	}
}

func TestRepoHandlersRepoPathRequired(t *testing.T) {
	store := newTestStore(t)
	project := &core.Project{
		ID:   "proj-repo-path-required",
		Name: "repo-path-required",
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/proj-repo-path-required/repo/status")
	if err != nil {
		t.Fatalf("GET /repo/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if apiErr.Code != "REPO_PATH_REQUIRED" {
		t.Fatalf("expected code REPO_PATH_REQUIRED, got %s", apiErr.Code)
	}
}

func TestRepoHandlersDiffPathValidation(t *testing.T) {
	repoPath := setupRepoHandlersGitRepo(t)
	store := newTestStore(t)
	project := &core.Project{
		ID:       "proj-repo-diff-validate",
		Name:     "repo-diff-validate",
		RepoPath: repoPath,
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	testCases := []struct {
		name     string
		query    string
		wantCode string
	}{
		{
			name:     "empty file path",
			query:    "file=",
			wantCode: "FILE_PATH_REQUIRED",
		},
		{
			name:     "path traversal",
			query:    "file=../outside.txt",
			wantCode: "INVALID_FILE_PATH",
		},
		{
			name:     "absolute path",
			query:    "file=" + filepath.ToSlash(filepath.Join(repoPath, "README.md")),
			wantCode: "INVALID_FILE_PATH",
		},
		{
			name:     "git metadata path",
			query:    "file=.git/config",
			wantCode: "INVALID_FILE_PATH",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(ts.URL + "/api/v1/projects/proj-repo-diff-validate/repo/diff?" + tc.query)
			if err != nil {
				t.Fatalf("GET /repo/diff: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}

			var apiErr apiError
			if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if apiErr.Code != tc.wantCode {
				t.Fatalf("expected code %s, got %s", tc.wantCode, apiErr.Code)
			}
		})
	}
}

func TestRepoHandlersTreeRejectsGitMetadataPath(t *testing.T) {
	repoPath := setupRepoHandlersGitRepoWithoutCommit(t)
	store := newTestStore(t)
	project := &core.Project{
		ID:       "proj-repo-tree-validate",
		Name:     "repo-tree-validate",
		RepoPath: repoPath,
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/proj-repo-tree-validate/repo/tree?dir=.git")
	if err != nil {
		t.Fatalf("GET /repo/tree: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var apiErr apiError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if apiErr.Code != "INVALID_FILE_PATH" {
		t.Fatalf("expected code INVALID_FILE_PATH, got %s", apiErr.Code)
	}
}

func TestRepoHandlersHappyPath(t *testing.T) {
	repoPath := setupRepoHandlersGitRepo(t)
	store := newTestStore(t)
	project := &core.Project{
		ID:       "proj-repo-happy",
		Name:     "repo-happy",
		RepoPath: repoPath,
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	treeResp, err := http.Get(ts.URL + "/api/v1/projects/proj-repo-happy/repo/tree?dir=.")
	if err != nil {
		t.Fatalf("GET /repo/tree: %v", err)
	}
	defer treeResp.Body.Close()
	if treeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected tree 200, got %d", treeResp.StatusCode)
	}

	var treeBody repoTreeResponse
	if err := json.NewDecoder(treeResp.Body).Decode(&treeBody); err != nil {
		t.Fatalf("decode tree response: %v", err)
	}
	if len(treeBody.Items) == 0 {
		t.Fatal("expected non-empty tree items")
	}

	statusResp, err := http.Get(ts.URL + "/api/v1/projects/proj-repo-happy/repo/status")
	if err != nil {
		t.Fatalf("GET /repo/status: %v", err)
	}
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusResp.StatusCode)
	}

	var statusBody repoStatusResponse
	if err := json.NewDecoder(statusResp.Body).Decode(&statusBody); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if len(statusBody.Items) == 0 {
		t.Fatal("expected non-empty status items")
	}

	diffResp, err := http.Get(ts.URL + "/api/v1/projects/proj-repo-happy/repo/diff?file=README.md")
	if err != nil {
		t.Fatalf("GET /repo/diff: %v", err)
	}
	defer diffResp.Body.Close()
	if diffResp.StatusCode != http.StatusOK {
		t.Fatalf("expected diff 200, got %d", diffResp.StatusCode)
	}

	var diffBody repoDiffResponse
	if err := json.NewDecoder(diffResp.Body).Decode(&diffBody); err != nil {
		t.Fatalf("decode diff response: %v", err)
	}
	if diffBody.FilePath != "README.md" {
		t.Fatalf("expected diff file README.md, got %s", diffBody.FilePath)
	}
	if !strings.Contains(diffBody.Diff, "README.md") {
		t.Fatalf("expected diff contains README.md, got %q", diffBody.Diff)
	}
}

func TestRepoHandlersDiffNoHeadStagedFile(t *testing.T) {
	repoPath := setupRepoHandlersGitRepoWithoutCommit(t)
	if err := os.WriteFile(filepath.Join(repoPath, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatalf("write staged.txt: %v", err)
	}
	runGitRepoHandlerTest(t, repoPath, "add", "staged.txt")

	store := newTestStore(t)
	project := &core.Project{
		ID:       "proj-repo-nohead-staged",
		Name:     "repo-nohead-staged",
		RepoPath: repoPath,
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	srv := NewServer(Config{Store: store})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	diffResp, err := http.Get(ts.URL + "/api/v1/projects/proj-repo-nohead-staged/repo/diff?file=staged.txt")
	if err != nil {
		t.Fatalf("GET /repo/diff: %v", err)
	}
	defer diffResp.Body.Close()
	if diffResp.StatusCode != http.StatusOK {
		t.Fatalf("expected diff 200, got %d", diffResp.StatusCode)
	}

	var diffBody repoDiffResponse
	if err := json.NewDecoder(diffResp.Body).Decode(&diffBody); err != nil {
		t.Fatalf("decode diff response: %v", err)
	}
	if !strings.Contains(diffBody.Diff, "staged.txt") {
		t.Fatalf("expected diff contains staged.txt, got %q", diffBody.Diff)
	}
}

func setupRepoHandlersGitRepo(t *testing.T) string {
	t.Helper()
	repo := setupRepoHandlersGitRepoWithoutCommit(t)

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGitRepoHandlerTest(t, repo, "add", ".")
	runGitRepoHandlerTest(t, repo, "commit", "-m", "init")

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("modify README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}

	return repo
}

func setupRepoHandlersGitRepoWithoutCommit(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitRepoHandlerTest(t, "", "init", repo)
	runGitRepoHandlerTest(t, repo, "config", "user.email", "test@example.com")
	runGitRepoHandlerTest(t, repo, "config", "user.name", "test-user")
	return repo
}

func runGitRepoHandlerTest(t *testing.T, repo string, args ...string) string {
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
