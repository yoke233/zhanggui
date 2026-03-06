package config

import (
	"os"
	"path/filepath"
)

const (
	projectConfigTOML = ".ai-workflow/config.toml"
	projectConfigYAML = ".ai-workflow/config.yaml"
)

// ProjectConfigPath returns the project config file path.
// Prefers .toml; falls back to .yaml if .toml doesn't exist.
func ProjectConfigPath(repoPath string) string {
	tomlPath := filepath.Join(repoPath, projectConfigTOML)
	if _, err := os.Stat(tomlPath); err == nil {
		return tomlPath
	}
	yamlPath := filepath.Join(repoPath, projectConfigYAML)
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}
	// Default to TOML for new configs.
	return tomlPath
}
