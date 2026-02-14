package outbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type workflowOutboxConfig struct {
	Backend string `toml:"backend"`
	Path    string `toml:"path"`
}

type workflowRolesConfig struct {
	Enabled []string `toml:"enabled"`
}

type workflowGroupConfig struct {
	Role          string   `toml:"role"`
	MaxConcurrent int      `toml:"max_concurrent"`
	ListenLabels  []string `toml:"listen_labels"`
}

type workflowExecutorConfig struct {
	Program        string   `toml:"program"`
	Args           []string `toml:"args"`
	TimeoutSeconds int      `toml:"timeout_seconds"`
}

type workflowProfile struct {
	Version   int                               `toml:"version"`
	Outbox    workflowOutboxConfig              `toml:"outbox"`
	Roles     workflowRolesConfig               `toml:"roles"`
	Repos     map[string]string                 `toml:"repos"`
	RoleRepo  map[string]string                 `toml:"role_repo"`
	Groups    map[string]workflowGroupConfig    `toml:"groups"`
	Executors map[string]workflowExecutorConfig `toml:"executors"`
}

func loadWorkflowProfile(workflowFile string) (workflowProfile, error) {
	path := strings.TrimSpace(workflowFile)
	if path == "" {
		return workflowProfile{}, errors.New("workflow file is required")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return workflowProfile{}, err
	}

	var profile workflowProfile
	if err := toml.Unmarshal(raw, &profile); err != nil {
		return workflowProfile{}, err
	}
	return profile, nil
}

func isRoleEnabled(profile workflowProfile, role string) bool {
	normalized := strings.TrimSpace(role)
	if normalized == "" {
		return false
	}
	for _, item := range profile.Roles.Enabled {
		if strings.TrimSpace(item) == normalized {
			return true
		}
	}
	return false
}

func resolveRoleRepoDir(profile workflowProfile, workflowFile string, role string) (string, error) {
	repoKey := strings.TrimSpace(profile.RoleRepo[role])
	if repoKey == "" {
		return "", errors.New("role_repo mapping is required for role " + role)
	}

	repoPath := strings.TrimSpace(profile.Repos[repoKey])
	if repoPath == "" {
		return "", errors.New("repos mapping is required for repo key " + repoKey)
	}

	workflowDir := filepath.Dir(workflowFile)
	if filepath.IsAbs(repoPath) {
		return filepath.Clean(repoPath), nil
	}
	return filepath.Clean(filepath.Join(workflowDir, repoPath)), nil
}

func findGroupByRole(profile workflowProfile, role string) (workflowGroupConfig, bool) {
	for _, group := range profile.Groups {
		if strings.TrimSpace(group.Role) == role {
			return group, true
		}
	}
	return workflowGroupConfig{}, false
}

func resolveExecutor(profile workflowProfile, role string) workflowExecutorConfig {
	executor, ok := profile.Executors[role]
	if !ok {
		return workflowExecutorConfig{
			Program:        "go",
			Args:           []string{"test", "./..."},
			TimeoutSeconds: 1800,
		}
	}

	executor.Program = strings.TrimSpace(executor.Program)
	if executor.Program == "" {
		executor.Program = "go"
	}
	if len(executor.Args) == 0 {
		executor.Args = []string{"test", "./..."}
	}
	if executor.TimeoutSeconds <= 0 {
		executor.TimeoutSeconds = 1800
	}
	return executor
}
