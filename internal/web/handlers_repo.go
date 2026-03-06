package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
	gitops "github.com/yoke233/ai-workflow/internal/git"
)

type repoHandlers struct {
	store core.Store
}

type repoTreeResponse struct {
	Dir   string             `json:"dir"`
	Items []gitops.FileEntry `json:"items"`
}

type repoStatusResponse struct {
	Items []gitops.FileEntry `json:"items"`
}

type repoDiffResponse struct {
	FilePath string `json:"file_path"`
	Diff     string `json:"diff"`
}

func registerRepoRoutes(r chi.Router, store core.Store) {
	h := &repoHandlers{store: store}
	r.With(RequireScope(ScopeProjectsRead)).Get("/projects/{projectID}/repo/tree", h.getRepoTree)
	r.With(RequireScope(ScopeProjectsRead)).Get("/projects/{projectID}/repo/status", h.getRepoStatus)
	r.With(RequireScope(ScopeProjectsRead)).Get("/projects/{projectID}/repo/diff", h.getRepoDiff)
}

func (h *repoHandlers) getRepoTree(w http.ResponseWriter, r *http.Request) {
	repoPath, ok := h.loadProjectRepoPath(w, r)
	if !ok {
		return
	}

	rawDir := strings.TrimSpace(r.URL.Query().Get("dir"))
	normalizedDir := ""
	if rawDir != "" {
		_, normalizedPath, err := validateRelativePath(repoPath, rawDir)
		if err != nil {
			writePathValidationError(w, rawDir, err)
			return
		}
		if normalizedPath != "." {
			normalizedDir = normalizedPath
		}
	}
	if containsGitMetadataPath(normalizedDir) {
		writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("invalid file path %q", strings.TrimSpace(rawDir)), "INVALID_FILE_PATH")
		return
	}

	runner := gitops.NewRunner(repoPath)
	items, err := runner.ListDirectory(normalizedDir)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list repository directory", "LIST_REPO_TREE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, repoTreeResponse{
		Dir:   normalizedDir,
		Items: items,
	})
}

func (h *repoHandlers) getRepoStatus(w http.ResponseWriter, r *http.Request) {
	repoPath, ok := h.loadProjectRepoPath(w, r)
	if !ok {
		return
	}

	runner := gitops.NewRunner(repoPath)
	items, err := runner.StatusFiles()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to get repository status", "GET_REPO_STATUS_FAILED")
		return
	}

	writeJSON(w, http.StatusOK, repoStatusResponse{Items: items})
}

func (h *repoHandlers) getRepoDiff(w http.ResponseWriter, r *http.Request) {
	repoPath, ok := h.loadProjectRepoPath(w, r)
	if !ok {
		return
	}

	rawFilePath := strings.TrimSpace(r.URL.Query().Get("file"))
	_, normalizedFilePath, err := validateRelativePath(repoPath, rawFilePath)
	if err != nil {
		writePathValidationError(w, rawFilePath, err)
		return
	}
	if normalizedFilePath == "." {
		writeAPIError(w, http.StatusBadRequest, "file path is required", "FILE_PATH_REQUIRED")
		return
	}
	if containsGitMetadataPath(normalizedFilePath) {
		writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("invalid file path %q", strings.TrimSpace(rawFilePath)), "INVALID_FILE_PATH")
		return
	}

	runner := gitops.NewRunner(repoPath)
	diffText, err := runner.DiffFile(normalizedFilePath)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to get file diff", "GET_REPO_DIFF_FAILED")
		return
	}

	writeJSON(w, http.StatusOK, repoDiffResponse{
		FilePath: normalizedFilePath,
		Diff:     diffText,
	})
}

func (h *repoHandlers) loadProjectRepoPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return "", false
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id is required", "PROJECT_ID_REQUIRED")
		return "", false
	}

	project, err := h.store.GetProject(projectID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", projectID), "PROJECT_NOT_FOUND")
			return "", false
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return "", false
	}

	repoPath := strings.TrimSpace(project.RepoPath)
	if repoPath == "" {
		writeAPIError(w, http.StatusBadRequest, "project repo_path is required", "REPO_PATH_REQUIRED")
		return "", false
	}
	return repoPath, true
}

func writePathValidationError(w http.ResponseWriter, rawPath string, err error) {
	if errors.Is(err, errRelativePathRequired) {
		writeAPIError(w, http.StatusBadRequest, "file path is required", "FILE_PATH_REQUIRED")
		return
	}
	writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("invalid file path %q", strings.TrimSpace(rawPath)), "INVALID_FILE_PATH")
}

func containsGitMetadataPath(relativePath string) bool {
	trimmed := strings.Trim(strings.TrimSpace(relativePath), "/")
	if trimmed == "" || trimmed == "." {
		return false
	}
	parts := strings.Split(trimmed, "/")
	for _, part := range parts {
		if part == ".git" {
			return true
		}
	}
	return false
}
