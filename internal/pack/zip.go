package pack

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/zhanggui/internal/manifest"
)

func CreateZip(taskDir, zipPath string, man manifest.Manifest) error {
	if strings.TrimSpace(taskDir) == "" {
		return fmt.Errorf("taskDir 不能为空")
	}
	if strings.TrimSpace(zipPath) == "" {
		return fmt.Errorf("zipPath 不能为空")
	}

	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		return err
	}

	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}

	zw := zip.NewWriter(f)
	defer func() {
		if zw != nil {
			_ = zw.Close()
		}
		if f != nil {
			_ = f.Close()
		}
	}()

	seen := map[string]struct{}{}
	for _, entry := range man.Files {
		name := strings.TrimPrefix(entry.Path, "/")
		name = filepath.ToSlash(name)
		if name == "" || strings.HasPrefix(name, "/") || hasPathTraversal(name) {
			return fmt.Errorf("manifest 包含非法 path: %q", entry.Path)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("manifest path 重复: %q", name)
		}
		seen[name] = struct{}{}

		srcPath := filepath.Join(taskDir, filepath.FromSlash(name))
		sf, err := os.Open(srcPath)
		if err != nil {
			return err
		}

		info, err := sf.Stat()
		if err != nil {
			_ = sf.Close()
			return err
		}

		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = sf.Close()
			return err
		}
		hdr.Name = name
		hdr.Method = zip.Deflate

		w, err := zw.CreateHeader(hdr)
		if err != nil {
			_ = sf.Close()
			return err
		}
		if _, err := io.Copy(w, sf); err != nil {
			_ = sf.Close()
			return err
		}
		_ = sf.Close()
	}

	if err := zw.Close(); err != nil {
		return err
	}
	zw = nil
	if err := f.Close(); err != nil {
		return err
	}
	f = nil
	return nil
}

func hasPathTraversal(rel string) bool {
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return true
	}
	if strings.Contains(rel, "/../") || strings.HasSuffix(rel, "/..") {
		return true
	}
	return false
}
