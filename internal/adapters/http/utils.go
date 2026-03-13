package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	gitutil "github.com/yoke233/ai-workflow/internal/adapters/workspace/git"
)

type detectGitRequest struct {
	Path string `json:"path"`
}

type detectGitResponse struct {
	IsGit         bool   `json:"is_git"`
	RemoteURL     string `json:"remote_url,omitempty"`
	CurrentBranch string `json:"current_branch,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

func (h *Handler) detectGitInfo(w http.ResponseWriter, r *http.Request) {
	var req detectGitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	dirPath := strings.TrimSpace(req.Path)
	if dirPath == "" {
		writeError(w, http.StatusBadRequest, "path is required", "MISSING_PATH")
		return
	}

	// Normalize path separators.
	dirPath = filepath.Clean(dirPath)

	// Check if the directory exists.
	info, err := os.Stat(dirPath)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusOK, detectGitResponse{IsGit: false})
		return
	}

	// Check if .git directory or file exists.
	gitDir := filepath.Join(dirPath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		writeJSON(w, http.StatusOK, detectGitResponse{IsGit: false})
		return
	}

	// It's a git repo — gather info.
	runner := gitutil.NewRunner(dirPath)

	resp := detectGitResponse{IsGit: true}

	// Get remote URL (origin).
	if remoteURL, err := runner.RemoteURL("origin"); err == nil {
		resp.RemoteURL = remoteURL
	}

	// Get current branch.
	if branch, err := runner.CurrentBranch(); err == nil {
		resp.CurrentBranch = branch
	}

	// Detect default branch.
	resp.DefaultBranch = gitutil.DetectDefaultBranch(dirPath)

	writeJSON(w, http.StatusOK, resp)
}
