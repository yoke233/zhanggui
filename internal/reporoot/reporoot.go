package reporoot

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindByGoMod walks up from start (or the current working directory if start is empty)
// until it finds a directory containing go.mod.
func FindByGoMod(start string) (string, error) {
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	dir := abs
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("未找到仓库根目录（go.mod）: start=%s", abs)
}
