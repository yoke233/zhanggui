package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalFileStore persists files under a local data directory.
type LocalFileStore struct {
	rootDir string
}

func NewLocal(rootDir string) *LocalFileStore {
	return &LocalFileStore{rootDir: rootDir}
}

func (s *LocalFileStore) Save(_ context.Context, fileName string, r io.Reader) (string, int64, error) {
	if s == nil || s.rootDir == "" {
		return "", 0, fmt.Errorf("local file store is not configured")
	}
	if r == nil {
		return "", 0, fmt.Errorf("reader is required")
	}

	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create root dir: %w", err)
	}

	tmp, err := os.CreateTemp(s.rootDir, "resource-*")
	if err != nil {
		return "", 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), r)
	if err != nil {
		return "", 0, fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, fmt.Errorf("close temp file: %w", err)
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	prefix := sum[:4]
	dir := filepath.Join(s.rootDir, prefix)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create resource dir: %w", err)
	}

	name := sanitizeFileName(fileName)
	if name == "" {
		name = "file"
	}
	destPath := filepath.Join(dir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), name))
	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", 0, fmt.Errorf("move file into store: %w", err)
	}
	return destPath, size, nil
}

func (s *LocalFileStore) Open(_ context.Context, uri string) (io.ReadCloser, error) {
	if uri == "" {
		return nil, fmt.Errorf("uri is required")
	}
	return os.Open(uri)
}

func (s *LocalFileStore) Delete(_ context.Context, uri string) error {
	if uri == "" {
		return fmt.Errorf("uri is required")
	}
	if err := os.Remove(uri); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func sanitizeFileName(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "." || name == ".." {
		return "file"
	}
	return name
}
