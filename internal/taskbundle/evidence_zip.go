package taskbundle

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func CreateEvidenceZip(bundleRootAbs string, zipPathAbs string) error {
	if strings.TrimSpace(bundleRootAbs) == "" {
		return fmt.Errorf("bundleRootAbs 不能为空")
	}
	if strings.TrimSpace(zipPathAbs) == "" {
		return fmt.Errorf("zipPathAbs 不能为空")
	}

	entries := make(map[string]struct{}, 16)
	add := func(rel string) error {
		rel = filepath.ToSlash(rel)
		rel = strings.TrimPrefix(rel, "/")
		rel = strings.TrimSpace(rel)
		if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\x00") {
			return fmt.Errorf("非法 zip entry: %q", rel)
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || strings.HasSuffix(rel, "/..") {
			return fmt.Errorf("zip entry 包含路径逃逸: %q", rel)
		}
		entries[rel] = struct{}{}
		return nil
	}

	required := []string{
		filepath.ToSlash(filepath.Join("ledger", "events.jsonl")),
		filepath.ToSlash(filepath.Join("logs", "tool_audit.jsonl")),
		filepath.ToSlash(filepath.Join("verify", "report.json")),
		filepath.ToSlash(filepath.Join("artifacts", "manifest.json")),
		filepath.ToSlash(filepath.Join("pack", "artifacts.zip")),
	}
	for _, r := range required {
		if err := add(r); err != nil {
			return err
		}
	}

	// Include all CAS evidence files to keep the pack self-contained.
	evidenceFilesAbs := filepath.Join(bundleRootAbs, "evidence", "files")
	if err := filepath.WalkDir(evidenceFilesAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(bundleRootAbs, path)
		if err != nil {
			return err
		}
		return add(rel)
	}); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Optional snapshot file.
	if _, err := os.Stat(filepath.Join(bundleRootAbs, "state.json")); err == nil {
		if err := add("state.json"); err != nil {
			return err
		}
	}

	// Deterministic-ish entry order.
	var list []string
	for p := range entries {
		list = append(list, p)
	}
	sort.Strings(list)

	if err := os.MkdirAll(filepath.Dir(zipPathAbs), 0o755); err != nil {
		return err
	}
	out, err := os.Create(zipPathAbs)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	defer func() {
		_ = zw.Close()
		_ = out.Close()
	}()

	for _, rel := range list {
		abs := filepath.Join(bundleRootAbs, filepath.FromSlash(rel))
		f, err := os.Open(abs)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return err
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = f.Close()
			return err
		}
		hdr.Name = rel
		hdr.Method = zip.Deflate
		hdr.Modified = time.Now()
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, err := io.Copy(w, f); err != nil {
			_ = f.Close()
			return err
		}
		_ = f.Close()
	}

	if err := zw.Close(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	// Structural validation (fast): ensure required entries exist.
	zr, err := zip.OpenReader(zipPathAbs)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()

	got := make(map[string]struct{}, len(zr.File))
	for _, f := range zr.File {
		name := filepath.ToSlash(strings.TrimPrefix(f.Name, "/"))
		got[name] = struct{}{}
	}
	for _, r := range required {
		if _, ok := got[r]; !ok {
			return fmt.Errorf("evidence.zip 缺少必需条目: %s", r)
		}
	}
	return nil
}
