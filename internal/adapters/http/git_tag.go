package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- request / response types ---

type gitCommitEntry struct {
	SHA       string `json:"sha"`
	Short     string `json:"short"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	Timestamp string `json:"timestamp"`
}

type gitTagEntry struct {
	Name      string `json:"name"`
	SHA       string `json:"sha"`
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type createTagRequest struct {
	Name    string `json:"name"`
	Ref     string `json:"ref,omitempty"`
	Message string `json:"message,omitempty"`
	Push    bool   `json:"push,omitempty"`
}

type createTagResponse struct {
	Name   string `json:"name"`
	SHA    string `json:"sha"`
	Pushed bool   `json:"pushed"`
}

type pushTagRequest struct {
	Name string `json:"name"`
}

type pushTagResponse struct {
	Name   string `json:"name"`
	Pushed bool   `json:"pushed"`
}

// --- route registration ---

func (h *Handler) registerGitTagRoutes(r routeRegistrar) {
	r.Get("/projects/{projectID}/git/commits", h.listGitCommits)
	r.Get("/projects/{projectID}/git/tags", h.listGitTags)
	r.Post("/projects/{projectID}/git/tags", h.createGitTag)
	r.Post("/projects/{projectID}/git/tags/push", h.pushGitTag)
}

// routeRegistrar is satisfied by chi.Router (avoids importing chi here).
type routeRegistrar interface {
	Get(pattern string, handlerFn http.HandlerFunc)
	Post(pattern string, handlerFn http.HandlerFunc)
}

// --- helpers ---

// resolveGitWorkDir finds a local git work directory for the project.
// It checks resource bindings for kind=git with a config.work_dir or config.local_path,
// falling back to the URI if it looks like a local path.
func (h *Handler) resolveGitWorkDir(ctx context.Context, projectID int64) (string, error) {
	bindings, err := h.store.ListResourceBindings(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("list resource bindings: %w", err)
	}
	for _, b := range bindings {
		if !strings.EqualFold(strings.TrimSpace(b.Kind), "git") {
			continue
		}
		if b.Config != nil {
			if wd, ok := b.Config["work_dir"].(string); ok && strings.TrimSpace(wd) != "" {
				return strings.TrimSpace(wd), nil
			}
			if lp, ok := b.Config["local_path"].(string); ok && strings.TrimSpace(lp) != "" {
				return strings.TrimSpace(lp), nil
			}
		}
		// If URI is a local absolute path, use it directly.
		uri := strings.TrimSpace(b.URI)
		if filepath.IsAbs(uri) {
			if info, statErr := os.Stat(uri); statErr == nil && info.IsDir() {
				return uri, nil
			}
		}
	}
	return "", fmt.Errorf("no git workspace found for project %d — add a git resource binding with config.work_dir", projectID)
}

func gitCmd(ctx context.Context, dir string, extraEnv []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// --- handlers ---

func (h *Handler) listGitCommits(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}
	if _, err := h.store.GetProject(r.Context(), projectID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	workDir, err := h.resolveGitWorkDir(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "NO_GIT_WORKSPACE")
		return
	}

	limit := queryInt(r, "limit", 30)
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}

	// %H full sha, %h short sha, %s subject, %an author, %aI iso timestamp
	format := "%H\x1f%h\x1f%s\x1f%an\x1f%aI"
	out, err := gitCmd(r.Context(), workDir, nil,
		"log", fmt.Sprintf("--max-count=%d", limit), fmt.Sprintf("--format=%s", format),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "GIT_ERROR")
		return
	}

	var commits []gitCommitEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 5)
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, gitCommitEntry{
			SHA:       parts[0],
			Short:     parts[1],
			Message:   parts[2],
			Author:    parts[3],
			Timestamp: parts[4],
		})
	}
	if commits == nil {
		commits = []gitCommitEntry{}
	}
	writeJSON(w, http.StatusOK, commits)
}

func (h *Handler) listGitTags(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}
	if _, err := h.store.GetProject(r.Context(), projectID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	workDir, err := h.resolveGitWorkDir(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "NO_GIT_WORKSPACE")
		return
	}

	// Use for-each-ref to get tags sorted by creation date (newest first).
	// %(creatordate:iso-strict) works for both lightweight and annotated tags.
	format := "%(refname:short)\x1f%(*objectname)%(objectname)\x1f%(contents:subject)\x1f%(creatordate:iso-strict)"
	out, err := gitCmd(r.Context(), workDir, nil,
		"for-each-ref", "--sort=-creatordate", fmt.Sprintf("--format=%s", format), "refs/tags/",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "GIT_ERROR")
		return
	}

	var tags []gitTagEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 4)
		if len(parts) < 4 {
			continue
		}
		sha := strings.TrimSpace(parts[1])
		// for-each-ref with %(*objectname)%(objectname) concatenates both;
		// the dereferenced SHA (40 chars) comes first for annotated tags.
		if len(sha) > 40 {
			sha = sha[:40]
		}
		tags = append(tags, gitTagEntry{
			Name:      parts[0],
			SHA:       sha,
			Message:   strings.TrimSpace(parts[2]),
			Timestamp: strings.TrimSpace(parts[3]),
		})
	}
	if tags == nil {
		tags = []gitTagEntry{}
	}
	writeJSON(w, http.StatusOK, tags)
}

func (h *Handler) createGitTag(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}
	if _, err := h.store.GetProject(r.Context(), projectID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req createTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	tagName := strings.TrimSpace(req.Name)
	if tagName == "" {
		writeError(w, http.StatusBadRequest, "tag name is required", "MISSING_NAME")
		return
	}
	ref := strings.TrimSpace(req.Ref)
	if ref == "" {
		ref = "HEAD"
	}

	workDir, err := h.resolveGitWorkDir(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "NO_GIT_WORKSPACE")
		return
	}

	// Create tag (annotated if message provided, lightweight otherwise).
	msg := strings.TrimSpace(req.Message)
	if msg != "" {
		_, err = gitCmd(r.Context(), workDir, nil,
			"-c", "user.name=ai-flow",
			"-c", "user.email=ai-flow@local",
			"tag", "-a", tagName, "-m", msg, ref,
		)
	} else {
		_, err = gitCmd(r.Context(), workDir, nil, "tag", tagName, ref)
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error(), "GIT_TAG_ERROR")
		return
	}

	sha, _ := gitCmd(r.Context(), workDir, nil, "rev-parse", tagName)
	sha = strings.TrimSpace(sha)

	// Optionally push right away.
	pushed := false
	if req.Push {
		if pushErr := h.pushTagToRemote(r.Context(), workDir, tagName); pushErr != nil {
			// Tag created but push failed — report partial success.
			writeJSON(w, http.StatusCreated, map[string]any{
				"name":       tagName,
				"sha":        sha,
				"pushed":     false,
				"push_error": pushErr.Error(),
			})
			return
		}
		pushed = true
	}

	writeJSON(w, http.StatusCreated, createTagResponse{
		Name:   tagName,
		SHA:    sha,
		Pushed: pushed,
	})
}

func (h *Handler) pushGitTag(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project ID", "BAD_ID")
		return
	}
	if _, err := h.store.GetProject(r.Context(), projectID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req pushTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	tagName := strings.TrimSpace(req.Name)
	if tagName == "" {
		writeError(w, http.StatusBadRequest, "tag name is required", "MISSING_NAME")
		return
	}

	workDir, err := h.resolveGitWorkDir(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "NO_GIT_WORKSPACE")
		return
	}

	if err := h.pushTagToRemote(r.Context(), workDir, tagName); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "GIT_PUSH_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, pushTagResponse{Name: tagName, Pushed: true})
}

// pushTagToRemote pushes a single tag to origin using PAT auth if available.
func (h *Handler) pushTagToRemote(ctx context.Context, workDir, tagName string) error {
	pat := strings.TrimSpace(h.gitPAT)
	if pat == "" {
		// Try plain push (works if SSH or credential helper is configured).
		_, err := gitCmd(ctx, workDir, nil, "push", "origin", "refs/tags/"+tagName)
		return err
	}

	// Write a temporary askpass script for PAT auth over HTTPS.
	askpassPath, cleanup, err := writeGitTagAskPass(pat)
	if err != nil {
		return err
	}
	defer cleanup()

	env := []string{
		"GIT_ASKPASS=" + askpassPath,
		"GIT_TERMINAL_PROMPT=0",
	}
	_, pushErr := gitCmd(ctx, workDir, env, "push", "origin", "refs/tags/"+tagName)
	return pushErr
}

// writeGitTagAskPass creates a temporary script that returns the PAT for git auth.
func writeGitTagAskPass(token string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "ai-workflow-tag-askpass-*")
	if err != nil {
		return "", nil, fmt.Errorf("create askpass dir: %w", err)
	}

	// Create a shell script (works on Linux/macOS).
	scriptPath := filepath.Join(dir, "askpass.sh")
	content := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n*sername*) echo \"x-access-token\" ;;\n*) echo \"%s\" ;;\nesac\n",
		strings.ReplaceAll(token, "\"", "\\\""))

	if err := os.WriteFile(scriptPath, []byte(content), 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("write askpass script: %w", err)
	}

	// Also create a .cmd for Windows compatibility.
	cmdPath := filepath.Join(dir, "askpass.cmd")
	cmdContent := strings.Join([]string{
		"@echo off",
		"set prompt=%~1",
		"echo %prompt% | findstr /i \"username\" >nul",
		"if %errorlevel%==0 (",
		"  echo x-access-token",
		"  exit /b 0",
		")",
		"echo " + token,
		"",
	}, "\r\n")
	_ = os.WriteFile(cmdPath, []byte(cmdContent), 0o600)

	return scriptPath, func() { _ = os.RemoveAll(dir) }, nil
}
