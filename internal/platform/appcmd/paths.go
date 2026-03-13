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

func resolveGlobalConfigFilePath(dataDir string) string {
	tomlPath := filepath.Join(dataDir, "config.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return tomlPath
	}
	yamlPath := filepath.Join(dataDir, "config.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}
	ymlPath := filepath.Join(dataDir, "config.yml")
	if _, err := os.Stat(ymlPath); err == nil {
		return ymlPath
	}
	return tomlPath
}

func ExpandStorePath(path string, baseDir string) string {
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
		if base := strings.TrimSpace(baseDir); base != "" {
			if normalized, ok := normalizeDataDirRelativePath(trimmed); ok {
				trimmed = normalized
			}
			return filepath.Clean(filepath.Join(base, trimmed))
		}
		if abs, err := filepath.Abs(trimmed); err == nil {
			return abs
		}
	}
	return trimmed
}

func normalizeDataDirRelativePath(path string) (string, bool) {
	cleaned := filepath.Clean(path)
	legacyRoot := filepath.Clean(".ai-workflow")
	prefix := legacyRoot + string(filepath.Separator)
	if cleaned == legacyRoot {
		return ".", true
	}
	if strings.HasPrefix(cleaned, prefix) {
		return strings.TrimPrefix(cleaned, prefix), true
	}
	return path, false
}
