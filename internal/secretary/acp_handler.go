package secretary

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/user/ai-workflow/internal/acpclient"
	"github.com/user/ai-workflow/internal/core"
)

type acpEventPublisher interface {
	Publish(evt core.Event)
}

type ACPHandlerSessionContext struct {
	SessionID    string
	ChangedFiles []string
}

type ACPHandler struct {
	acpclient.NopHandler

	cwd       string
	sessionID string
	publisher acpEventPublisher

	mu          sync.Mutex
	changedSet  map[string]struct{}
	changedList []string
}

var _ acpclient.Handler = (*ACPHandler)(nil)

func NewACPHandler(cwd string, sessionID string, publisher acpEventPublisher) *ACPHandler {
	return &ACPHandler{
		cwd:        strings.TrimSpace(cwd),
		sessionID:  strings.TrimSpace(sessionID),
		publisher:  publisher,
		changedSet: make(map[string]struct{}),
	}
}

func (h *ACPHandler) SetSessionID(sessionID string) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessionID = strings.TrimSpace(sessionID)
}

func (h *ACPHandler) HandleWriteFile(_ context.Context, req acpclient.WriteFileRequest) (acpclient.WriteFileResult, error) {
	if h == nil {
		return acpclient.WriteFileResult{}, errors.New("acp handler is nil")
	}

	targetPath, relPath, err := h.normalizePathInScope(req.Path)
	if err != nil {
		return acpclient.WriteFileResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return acpclient.WriteFileResult{}, fmt.Errorf("ensure parent dir: %w", err)
	}

	content := []byte(req.Content)
	if err := os.WriteFile(targetPath, content, 0o644); err != nil {
		return acpclient.WriteFileResult{}, fmt.Errorf("write file %q: %w", relPath, err)
	}

	filePaths := h.recordChangedFile(relPath)
	h.publishFilesChanged(filePaths)

	return acpclient.WriteFileResult{BytesWritten: len(content)}, nil
}

func (h *ACPHandler) SessionContext() ACPHandlerSessionContext {
	if h == nil {
		return ACPHandlerSessionContext{}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	changed := make([]string, len(h.changedList))
	copy(changed, h.changedList)
	return ACPHandlerSessionContext{
		SessionID:    h.sessionID,
		ChangedFiles: changed,
	}
}

func (h *ACPHandler) normalizePathInScope(rawPath string) (string, string, error) {
	cwd := strings.TrimSpace(h.cwd)
	if cwd == "" {
		return "", "", errors.New("handler cwd is required")
	}
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", fmt.Errorf("resolve cwd: %w", err)
	}

	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", "", errors.New("write file path is required")
	}

	target := trimmed
	if !filepath.IsAbs(target) {
		target = filepath.Join(cwdAbs, target)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}

	rel, err := filepath.Rel(cwdAbs, targetAbs)
	if err != nil {
		return "", "", fmt.Errorf("check path scope: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path %q is outside cwd scope", trimmed)
	}

	rel = filepath.ToSlash(filepath.Clean(rel))
	return targetAbs, rel, nil
}

func (h *ACPHandler) recordChangedFile(path string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.changedSet[path]; !ok {
		h.changedSet[path] = struct{}{}
		h.changedList = append(h.changedList, path)
	}

	out := make([]string, len(h.changedList))
	copy(out, h.changedList)
	return out
}

func (h *ACPHandler) publishFilesChanged(filePaths []string) {
	if h.publisher == nil {
		return
	}

	h.publisher.Publish(core.Event{
		Type: core.EventSecretaryFilesChanged,
		Data: map[string]string{
			"session_id": h.sessionID,
			"file_paths": strings.Join(filePaths, ","),
		},
		Timestamp: time.Now(),
	})
}
