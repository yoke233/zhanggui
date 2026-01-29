package pack

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/yoke233/zhanggui/internal/manifest"
)

func ValidateZipMatchesManifest(zipPath string, man manifest.Manifest) error {
	if strings.TrimSpace(zipPath) == "" {
		return fmt.Errorf("zipPath 不能为空")
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()

	zipEntries := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "/")
		name = strings.TrimSpace(name)
		name = strings.ReplaceAll(name, "\\", "/")
		if name == "" || strings.HasPrefix(name, "/") || hasPathTraversal(name) {
			return fmt.Errorf("zip 包含非法 entry: %q", f.Name)
		}
		if _, ok := zipEntries[name]; ok {
			return fmt.Errorf("zip entry 重复: %q", name)
		}
		zipEntries[name] = f
	}

	seen := make(map[string]struct{}, len(man.Files))
	for _, entry := range man.Files {
		name := strings.TrimPrefix(entry.Path, "/")
		name = strings.ReplaceAll(filepathToSlash(name), "\\", "/")
		if name == "" || strings.HasPrefix(name, "/") || hasPathTraversal(name) {
			return fmt.Errorf("manifest 包含非法 path: %q", entry.Path)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("manifest path 重复: %q", name)
		}
		seen[name] = struct{}{}

		zf, ok := zipEntries[name]
		if !ok {
			return fmt.Errorf("zip 缺少 manifest 文件: %q", name)
		}
		if entry.Size > 0 && zf.UncompressedSize64 != uint64(entry.Size) {
			return fmt.Errorf("zip 文件大小不匹配: %q zip=%d manifest=%d", name, zf.UncompressedSize64, entry.Size)
		}

		rc, err := zf.Open()
		if err != nil {
			return err
		}
		h := sha256.New()
		if _, err := io.Copy(h, rc); err != nil {
			_ = rc.Close()
			return err
		}
		_ = rc.Close()
		got := hex.EncodeToString(h.Sum(nil))
		if got != entry.Sha256 {
			return fmt.Errorf("zip 文件 sha256 不匹配: %q got=%s want=%s", name, got, entry.Sha256)
		}
	}

	for name := range zipEntries {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("zip 包含 manifest 外文件: %q", name)
		}
	}
	return nil
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
