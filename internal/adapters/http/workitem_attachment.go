package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Allowed MIME types for issue attachments.
var allowedAttachmentMIME = map[string]bool{
	"text/plain":    true,
	"text/markdown": true,
	"image/png":     true,
	"image/jpeg":    true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
}

// allowedAttachmentExt maps file extensions to MIME types as a fallback.
var allowedAttachmentExt = map[string]string{
	".md":   "text/markdown",
	".txt":  "text/plain",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

const maxAttachmentSize = 10 << 20 // 10 MB

func (h *Handler) uploadWorkItemAttachment(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	// Verify issue exists.
	if _, err := h.store.GetWorkItem(r.Context(), issueID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	if h.dataDir == "" {
		writeError(w, http.StatusInternalServerError, "file storage not configured", "STORAGE_NOT_CONFIGURED")
		return
	}

	if err := r.ParseMultipartForm(maxAttachmentSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid multipart form", "BAD_REQUEST")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field", "MISSING_FILE")
		return
	}
	defer file.Close()

	if header.Size > maxAttachmentSize {
		writeError(w, http.StatusRequestEntityTooLarge, "file exceeds 10MB limit", "FILE_TOO_LARGE")
		return
	}

	// Determine MIME type: prefer extension-based detection for reliability.
	ext := strings.ToLower(filepath.Ext(header.Filename))
	mimeType, extAllowed := allowedAttachmentExt[ext]
	if !extAllowed {
		// Fallback: check Content-Type from multipart header.
		ct := header.Header.Get("Content-Type")
		if ct != "" {
			mimeType = strings.Split(ct, ";")[0]
		}
		if !allowedAttachmentMIME[mimeType] {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("file type %q not allowed; accepted: .md, .txt, .png, .jpg, .jpeg, .gif, .webp, .svg", ext),
				"UNSUPPORTED_FILE_TYPE")
			return
		}
	}

	// Create storage directory.
	uploadDir := filepath.Join(h.dataDir, "uploads", "issues", fmt.Sprintf("%d", issueID))
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create upload directory", "STORAGE_ERROR")
		return
	}

	// Generate unique filename to avoid conflicts.
	safeFileName := sanitizeFileName(header.Filename)
	storedName := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), safeFileName)
	diskPath := filepath.Join(uploadDir, storedName)

	dst, err := os.Create(diskPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create file", "STORAGE_ERROR")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		os.Remove(diskPath)
		writeError(w, http.StatusInternalServerError, "failed to write file", "STORAGE_ERROR")
		return
	}

	att := &core.WorkItemAttachment{
		WorkItemID: issueID,
		FileName: header.Filename,
		FilePath: diskPath,
		MimeType: mimeType,
		Size:     written,
	}

	id, err := h.store.CreateWorkItemAttachment(r.Context(), att)
	if err != nil {
		os.Remove(diskPath)
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	att.ID = id

	writeJSON(w, http.StatusCreated, att)
}

func (h *Handler) listWorkItemAttachments(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	attachments, err := h.store.ListWorkItemAttachments(r.Context(), issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if attachments == nil {
		attachments = []*core.WorkItemAttachment{}
	}
	writeJSON(w, http.StatusOK, attachments)
}

func (h *Handler) getWorkItemAttachment(w http.ResponseWriter, r *http.Request) {
	attID, ok := urlParamInt64(r, "attachmentID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid attachment ID", "BAD_ID")
		return
	}

	att, err := h.store.GetWorkItemAttachment(r.Context(), attID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "attachment not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, att)
}

func (h *Handler) downloadWorkItemAttachment(w http.ResponseWriter, r *http.Request) {
	attID, ok := urlParamInt64(r, "attachmentID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid attachment ID", "BAD_ID")
		return
	}

	att, err := h.store.GetWorkItemAttachment(r.Context(), attID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "attachment not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	f, err := os.Open(att.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "file not found on disk", "FILE_MISSING")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", att.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, att.FileName))
	io.Copy(w, f)
}

func (h *Handler) deleteWorkItemAttachment(w http.ResponseWriter, r *http.Request) {
	attID, ok := urlParamInt64(r, "attachmentID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid attachment ID", "BAD_ID")
		return
	}

	att, err := h.store.GetWorkItemAttachment(r.Context(), attID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "attachment not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	if err := h.store.DeleteWorkItemAttachment(r.Context(), attID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	// Best-effort remove file from disk.
	os.Remove(att.FilePath)

	w.WriteHeader(http.StatusNoContent)
}

// sanitizeFileName removes path separators and other dangerous characters.
func sanitizeFileName(name string) string {
	// Use only the base name.
	name = filepath.Base(name)
	// Replace any remaining path separators.
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "" || name == "." || name == ".." {
		name = "file"
	}
	return name
}
