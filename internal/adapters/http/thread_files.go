package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// threadFileRef is a unified file reference returned by the file search API.
type threadFileRef struct {
	Source      string `json:"source"` // "attachment" | "project" | "workspace"
	Name        string `json:"name"`   // display name (file name or relative path)
	Path        string `json:"path"`   // relative path from thread cwd
	Size        int64  `json:"size,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	IsDirectory bool   `json:"is_directory,omitempty"`
	Project     string `json:"project,omitempty"` // project slug (for source=project)
	Note        string `json:"note,omitempty"`    // attachment note
}

// listThreadFiles returns a unified file listing for the # trigger and file picker.
// GET /threads/{threadID}/files?q=<search>&source=<all|attachment|project|workspace>&limit=<n>
func (h *Handler) listThreadFiles(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if source == "" {
		source = "all"
	}
	limit := queryInt(r, "limit", 20)

	var results []threadFileRef

	// 1. Attachments (from DB — fast, always first).
	if source == "all" || source == "attachment" {
		attachments, err := h.store.ListThreadAttachments(r.Context(), threadID)
		if err == nil {
			for _, att := range attachments {
				if att == nil {
					continue
				}
				if query != "" && !fuzzyMatch(att.FileName, query) {
					continue
				}
				results = append(results, threadFileRef{
					Source:      "attachment",
					Name:        att.FileName,
					Path:        "attachments/" + filepath.Base(att.FilePath),
					Size:        att.FileSize,
					ContentType: att.ContentType,
					IsDirectory: att.IsDirectory,
					Note:        att.Note,
				})
			}
		}
	}

	// 2. Workspace files (top-level files in threadDir, excluding infrastructure).
	if (source == "all" || source == "workspace") && h.dataDir != "" {
		threadDir := filepath.Join(h.dataDir, "threads", strconv.FormatInt(threadID, 10))
		results = appendWorkspaceFiles(results, threadDir, query)
	}

	// 3. Project files (from projects/ symlinks — scan on demand).
	if (source == "all" || source == "project") && h.dataDir != "" {
		threadDir := filepath.Join(h.dataDir, "threads", strconv.FormatInt(threadID, 10))
		projectsDir := filepath.Join(threadDir, "projects")
		results = appendProjectFiles(results, projectsDir, query, limit-len(results))
	}

	if len(results) > limit {
		results = results[:limit]
	}

	writeJSON(w, http.StatusOK, results)
}

var infrastructureDirs = map[string]bool{
	"attachments": true,
	"projects":    true,
	".archive":    true,
}

// appendWorkspaceFiles lists non-infrastructure files in the thread root.
func appendWorkspaceFiles(results []threadFileRef, threadDir string, query string) []threadFileRef {
	entries, err := os.ReadDir(threadDir)
	if err != nil {
		return results
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".context.json" {
			continue
		}
		if infrastructureDirs[name] {
			continue
		}
		if query != "" && !fuzzyMatch(name, query) {
			continue
		}
		ref := threadFileRef{
			Source:      "workspace",
			Name:        name,
			Path:        name,
			IsDirectory: entry.IsDir(),
		}
		if info, err := entry.Info(); err == nil {
			ref.Size = info.Size()
		}
		results = append(results, ref)
	}
	return results
}

// appendProjectFiles scans project directories for matching files.
// Only walks one level deep in each project to keep it fast.
func appendProjectFiles(results []threadFileRef, projectsDir string, query string, budget int) []threadFileRef {
	if budget <= 0 {
		return results
	}
	projectEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		return results
	}
	for _, projEntry := range projectEntries {
		if !projEntry.IsDir() {
			continue
		}
		slug := projEntry.Name()
		projPath := filepath.Join(projectsDir, slug)

		// Walk project tree (max 3 levels deep to avoid huge scans).
		count := 0
		_ = filepath.WalkDir(projPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}
			if count >= budget {
				return filepath.SkipAll
			}

			rel, _ := filepath.Rel(projPath, path)
			if rel == "." {
				return nil
			}

			// Skip common large/hidden directories.
			if d.IsDir() {
				base := d.Name()
				if base == ".git" || base == "node_modules" || base == ".venv" || base == "__pycache__" || base == "vendor" {
					return filepath.SkipDir
				}
				depth := strings.Count(filepath.ToSlash(rel), "/")
				if depth >= 3 {
					return filepath.SkipDir
				}
				return nil
			}

			if query != "" && !fuzzyMatch(rel, query) {
				return nil
			}

			ref := threadFileRef{
				Source:  "project",
				Name:    rel,
				Path:    filepath.ToSlash(filepath.Join("projects", slug, rel)),
				Project: slug,
			}
			if info, err := d.Info(); err == nil {
				ref.Size = info.Size()
			}
			results = append(results, ref)
			count++
			return nil
		})
	}
	return results
}

// fuzzyMatch checks if the query terms appear in the target (case-insensitive, order-independent).
func fuzzyMatch(target string, query string) bool {
	lower := strings.ToLower(target)
	for _, term := range strings.Fields(query) {
		if !strings.Contains(lower, term) {
			return false
		}
	}
	return true
}
