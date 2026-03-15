package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/ai-workflow/internal/adapters/resource/filestore"
	"github.com/yoke233/ai-workflow/internal/core"
)

var allowedResourceMIME = map[string]bool{
	"text/plain":               true,
	"text/markdown":            true,
	"image/png":                true,
	"image/jpeg":               true,
	"image/gif":                true,
	"image/webp":               true,
	"image/svg+xml":            true,
	"application/pdf":          true,
	"text/csv":                 true,
	"text/x-diff":              true,
	"application/json":         true,
	"application/zip":          true,
	"application/octet-stream": true,
}

var allowedResourceExt = map[string]string{
	".md":    "text/markdown",
	".txt":   "text/plain",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".svg":   "image/svg+xml",
	".pdf":   "application/pdf",
	".csv":   "text/csv",
	".diff":  "text/x-diff",
	".patch": "text/x-diff",
	".json":  "application/json",
	".zip":   "application/zip",
}

const maxResourceSize = 10 << 20 // 10 MB

func (h *Handler) uploadWorkItemResource(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}
	projectID, err := h.resolveWorkItemProjectID(r.Context(), workItemID)
	if err != nil {
		writeOwnerLookupError(w, "work item", err)
		return
	}
	resource, err := h.uploadOwnedResource(r, "input", projectID, func(res *core.Resource) {
		res.WorkItemID = &workItemID
	})
	if err != nil {
		writeResourceUploadError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resource)
}

func (h *Handler) listWorkItemResources(w http.ResponseWriter, r *http.Request) {
	workItemID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}
	resources, err := h.store.ListResourcesByWorkItem(r.Context(), workItemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if resources == nil {
		resources = []*core.Resource{}
	}
	writeJSON(w, http.StatusOK, resources)
}

func (h *Handler) uploadMessageResource(w http.ResponseWriter, r *http.Request) {
	messageID, ok := urlParamInt64(r, "messageID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid message ID", "BAD_ID")
		return
	}
	projectID, err := h.resolveMessageProjectID(r.Context(), messageID)
	if err != nil {
		writeOwnerLookupError(w, "message", err)
		return
	}
	resource, err := h.uploadOwnedResource(r, "attachment", projectID, func(res *core.Resource) {
		res.MessageID = &messageID
	})
	if err != nil {
		writeResourceUploadError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resource)
}

func (h *Handler) listMessageResources(w http.ResponseWriter, r *http.Request) {
	messageID, ok := urlParamInt64(r, "messageID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid message ID", "BAD_ID")
		return
	}
	resources, err := h.store.ListResourcesByMessage(r.Context(), messageID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if resources == nil {
		resources = []*core.Resource{}
	}
	writeJSON(w, http.StatusOK, resources)
}

func (h *Handler) listRunResources(w http.ResponseWriter, r *http.Request) {
	runID, ok := urlParamInt64(r, "runID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid run ID", "BAD_ID")
		return
	}
	resources, err := h.store.ListResourcesByRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if resources == nil {
		resources = []*core.Resource{}
	}
	writeJSON(w, http.StatusOK, resources)
}

func (h *Handler) getResource(w http.ResponseWriter, r *http.Request) {
	resourceID, ok := urlParamInt64(r, "resourceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid resource ID", "BAD_ID")
		return
	}
	resource, err := h.store.GetResource(r.Context(), resourceID)
	if err != nil {
		writeResourceLookupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

func (h *Handler) downloadResource(w http.ResponseWriter, r *http.Request) {
	resourceID, ok := urlParamInt64(r, "resourceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid resource ID", "BAD_ID")
		return
	}
	resource, err := h.store.GetResource(r.Context(), resourceID)
	if err != nil {
		writeResourceLookupError(w, err)
		return
	}
	rc, err := h.resourceFileStore().Open(r.Context(), resource.URI)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "resource file missing", "FILE_MISSING")
		return
	}
	defer rc.Close()

	if resource.MimeType != "" {
		w.Header().Set("Content-Type", resource.MimeType)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, resource.FileName))
	if _, err := io.Copy(w, rc); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stream resource", "STREAM_ERROR")
		return
	}
}

func (h *Handler) deleteResource(w http.ResponseWriter, r *http.Request) {
	resourceID, ok := urlParamInt64(r, "resourceID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid resource ID", "BAD_ID")
		return
	}
	resource, err := h.store.GetResource(r.Context(), resourceID)
	if err != nil {
		writeResourceLookupError(w, err)
		return
	}
	if err := h.store.DeleteResource(r.Context(), resourceID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if resource.StorageKind == "local" && resource.URI != "" {
		_ = h.resourceFileStore().Delete(r.Context(), resource.URI)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) uploadOwnedResource(r *http.Request, role string, projectID int64, attach func(*core.Resource)) (*core.Resource, error) {
	if h.dataDir == "" {
		return nil, fmt.Errorf("file storage not configured")
	}
	if err := r.ParseMultipartForm(maxResourceSize); err != nil {
		return nil, fmt.Errorf("file too large or invalid multipart form")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("missing file field")
	}
	defer file.Close()

	if header.Size > maxResourceSize {
		return nil, fmt.Errorf("file exceeds 10MB limit")
	}
	mimeType, err := detectUploadMimeType(header.Filename, header.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	uri, size, err := h.resourceFileStore().Save(r.Context(), header.Filename, file)
	if err != nil {
		return nil, err
	}
	resource := &core.Resource{
		ProjectID:   projectID,
		StorageKind: "local",
		URI:         uri,
		Role:        role,
		FileName:    header.Filename,
		MimeType:    mimeType,
		SizeBytes:   size,
	}
	if attach != nil {
		attach(resource)
	}
	id, err := h.store.CreateResource(r.Context(), resource)
	if err != nil {
		_ = h.resourceFileStore().Delete(r.Context(), uri)
		return nil, err
	}
	resource.ID = id
	return resource, nil
}

func (h *Handler) resourceFileStore() core.FileStore {
	return filestore.NewLocal(filepath.Join(h.dataDir, "files"))
}

func detectUploadMimeType(fileName, contentType string) (string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if mimeType, ok := allowedResourceExt[ext]; ok {
		return mimeType, nil
	}
	mimeType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if !allowedResourceMIME[mimeType] {
		return "", fmt.Errorf("file type %q not allowed", ext)
	}
	return mimeType, nil
}

func (h *Handler) resolveWorkItemProjectID(ctx context.Context, workItemID int64) (int64, error) {
	workItem, err := h.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		return 0, err
	}
	if workItem.ProjectID != nil {
		return *workItem.ProjectID, nil
	}
	return 0, nil
}

func (h *Handler) resolveMessageProjectID(ctx context.Context, messageID int64) (int64, error) {
	msg, err := h.store.GetThreadMessage(ctx, messageID)
	if err != nil {
		return 0, err
	}
	thread, err := h.store.GetThread(ctx, msg.ThreadID)
	if err != nil {
		return 0, err
	}
	if projectID, ok := core.ReadThreadFocusProjectID(thread); ok {
		return projectID, nil
	}
	return 0, nil
}

func writeResourceLookupError(w http.ResponseWriter, err error) {
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "resource not found", "NOT_FOUND")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
}

func writeOwnerLookupError(w http.ResponseWriter, owner string, err error) {
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, owner+" not found", "NOT_FOUND")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
}

func writeResourceUploadError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not configured"):
		writeError(w, http.StatusInternalServerError, msg, "STORAGE_NOT_CONFIGURED")
	case strings.Contains(msg, "too large"), strings.Contains(msg, "exceeds 10MB limit"):
		writeError(w, http.StatusRequestEntityTooLarge, msg, "FILE_TOO_LARGE")
	case strings.Contains(msg, "missing file field"), strings.Contains(msg, "multipart"), strings.Contains(msg, "not allowed"):
		writeError(w, http.StatusBadRequest, msg, "BAD_REQUEST")
	default:
		writeError(w, http.StatusInternalServerError, msg, "STORE_ERROR")
	}
}

func removeLocalResourceFile(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}
