package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FileEntry struct {
	Path   string `json:"path"`
	Sha256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	SchemaVersion int         `json:"schema_version"`
	TaskID        string      `json:"task_id"`
	Rev           string      `json:"rev"`
	GeneratedAt   string      `json:"generated_at"`
	Files         []FileEntry `json:"files"`
}

func Generate(taskDir, taskID, rev string) (Manifest, error) {
	if strings.TrimSpace(taskDir) == "" {
		return Manifest{}, fmt.Errorf("taskDir 不能为空")
	}
	if strings.TrimSpace(taskID) == "" {
		return Manifest{}, fmt.Errorf("taskID 不能为空")
	}
	if strings.TrimSpace(rev) == "" {
		return Manifest{}, fmt.Errorf("rev 不能为空")
	}

	revDir := filepath.Join(taskDir, "revs", rev)
	if _, err := os.Stat(revDir); err != nil {
		return Manifest{}, err
	}

	var files []FileEntry
	err := filepath.WalkDir(revDir, func(path string, d os.DirEntry, err error) error {
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

		rel, err := filepath.Rel(taskDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "" || strings.HasPrefix(rel, "/") || hasPathTraversal(rel) {
			return fmt.Errorf("非法相对路径: %q", rel)
		}
		if !strings.HasPrefix(rel, "revs/"+rev+"/") {
			return fmt.Errorf("非法产物路径（不在 rev 前缀内）: %q", rel)
		}

		sha, err := sha256File(path)
		if err != nil {
			return err
		}

		files = append(files, FileEntry{
			Path:   rel,
			Sha256: sha,
			Size:   info.Size(),
		})
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	return Manifest{
		SchemaVersion: 1,
		TaskID:        taskID,
		Rev:           rev,
		GeneratedAt:   time.Now().Format(time.RFC3339),
		Files:         files,
	}, nil
}

func WriteJSON(path string, v Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeFileAtomic(path, b, 0o644)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp := filepath.Join(dir, "."+base+".tmp."+time.Now().Format("150405.000000000"))
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic rename failed: %w", err)
	}
	return nil
}
