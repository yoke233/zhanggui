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
	Mode          string   `toml:"mode"`
	Writeback     string   `toml:"writeback"`
	ListenLabels  []string `toml:"listen_labels"`
}

type workflowExecutorConfig struct {
	Program        string   `toml:"program"`
	Args           []string `toml:"args"`
	TimeoutSeconds int      `toml:"timeout_seconds"`
}

type workflowWorkdirConfig struct {
	Enabled bool     `toml:"enabled"`
	Backend string   `toml:"backend"`
	Root    string   `toml:"root"`
	Cleanup string   `toml:"cleanup"`
	Roles   []string `toml:"roles"`
}

type workflowProfile struct {
	Version   int                               `toml:"version"`
	Outbox    workflowOutboxConfig              `toml:"outbox"`
	Workdir   workflowWorkdirConfig             `toml:"workdir"`
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
	if err := validateWorkflowProfile(profile); err != nil {
		return workflowProfile{}, err
	}
	profile.Workdir = resolveWorkdirConfig(profile)
	return profile, nil
}

func validateWorkflowProfile(profile workflowProfile) error {
	if profile.Version != 2 {
		return errors.New("unsupported workflow version: expected version = 2 (Phase 2.8 V2 baseline)")
	}

	for name, group := range profile.Groups {
		role := strings.TrimSpace(group.Role)
		if role == "" {
			return errors.New("groups." + strings.TrimSpace(name) + ".role is required")
		}

		mode := strings.ToLower(strings.TrimSpace(group.Mode))
		if mode == "" {
			return errors.New("groups." + strings.TrimSpace(name) + ".mode is required")
		}
		if mode != "owner" && mode != "subscriber" {
			return errors.New("groups." + strings.TrimSpace(name) + ".mode must be owner or subscriber")
		}

		writeback := strings.ToLower(strings.TrimSpace(group.Writeback))
		if writeback == "" {
			return errors.New("groups." + strings.TrimSpace(name) + ".writeback is required")
		}
		if writeback != "full" && writeback != "comment-only" {
			return errors.New("groups." + strings.TrimSpace(name) + ".writeback must be full or comment-only")
		}
		if mode == "subscriber" && writeback != "comment-only" {
			return errors.New("groups." + strings.TrimSpace(name) + ": subscriber mode requires writeback = comment-only")
		}
	}
	return nil
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

func resolveWorkdirConfig(profile workflowProfile) workflowWorkdirConfig {
	cfg := profile.Workdir
	if !cfg.Enabled {
		return cfg
	}

	cfg.Backend = strings.TrimSpace(cfg.Backend)
	if cfg.Backend == "" {
		cfg.Backend = "git-worktree"
	}
	cfg.Root = strings.TrimSpace(cfg.Root)
	if cfg.Root == "" {
		cfg.Root = filepath.Join(".worktrees", "runs")
	}
	if filepath.VolumeName(cfg.Root) == "" && !strings.HasPrefix(cfg.Root, `\\`) {
		cfg.Root = strings.TrimLeft(cfg.Root, `/\`)
	}
	cfg.Cleanup = strings.TrimSpace(cfg.Cleanup)
	if cfg.Cleanup == "" {
		cfg.Cleanup = "immediate"
	}
	cfg.Roles = normalizeLabels(cfg.Roles)
	if len(cfg.Roles) == 0 {
		cfg.Roles = []string{"backend"}
	}
	return cfg
}
