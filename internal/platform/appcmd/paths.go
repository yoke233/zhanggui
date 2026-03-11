package appcmd

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveSecretsFilePath(dataDir string) string {
	tomlPath := filepath.Join(dataDir, "secrets.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return tomlPath
	}
	yamlPath := filepath.Join(dataDir, "secrets.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}
	return tomlPath
}

func ExpandStorePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			trimmed = filepath.Join(home, trimmed[2:])
		}
	}
	if !filepath.IsAbs(trimmed) {
		if abs, err := filepath.Abs(trimmed); err == nil {
			return abs
		}
	}
	return trimmed
}
