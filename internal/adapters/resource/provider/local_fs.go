package provider

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yoke233/ai-workflow/internal/core"
)

// LocalFSProvider handles resources on local/mounted filesystems.
type LocalFSProvider struct{}

func (p *LocalFSProvider) Kind() core.ResourceLocatorKind {
	return core.LocatorLocalFS
}

func (p *LocalFSProvider) Fetch(_ context.Context, locator *core.ResourceLocator, path string, destDir string) (string, error) {
	src := filepath.Join(locator.BaseURI, path)
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("local_fs fetch: source %s: %w", src, err)
	}

	destPath := filepath.Join(destDir, filepath.Base(path))
	if err := copyFile(src, destPath); err != nil {
		return "", fmt.Errorf("local_fs fetch: copy %s → %s: %w", src, destPath, err)
	}
	return destPath, nil
}

func (p *LocalFSProvider) Deposit(_ context.Context, locator *core.ResourceLocator, path string, localPath string) error {
	dest := filepath.Join(locator.BaseURI, path)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("local_fs deposit: mkdir %s: %w", filepath.Dir(dest), err)
	}
	if err := copyFile(localPath, dest); err != nil {
		return fmt.Errorf("local_fs deposit: copy %s → %s: %w", localPath, dest, err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
