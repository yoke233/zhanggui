package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

type BwrapSandbox struct {
	Base    Sandbox
	Command string
}

func (s BwrapSandbox) Prepare(ctx context.Context, in PrepareInput) (acpclient.LaunchConfig, error) {
	base := s.Base
	if base == nil {
		base = NoopSandbox{}
	}

	launch, err := base.Prepare(ctx, in)
	if err != nil {
		return launch, err
	}

	command := strings.TrimSpace(s.Command)
	if command == "" {
		command = "bwrap"
	}
	if _, err := lookPath(command); err != nil {
		return launch, fmt.Errorf("find bwrap command %q: %w", command, err)
	}
	if strings.TrimSpace(launch.Command) == "" {
		return launch, fmt.Errorf("bwrap sandbox: target program is required")
	}
	if strings.TrimSpace(launch.WorkDir) == "" {
		return launch, fmt.Errorf("bwrap sandbox: workdir is required")
	}

	env, err := buildBwrapEnv(launch)
	if err != nil {
		return launch, err
	}
	roots, err := buildBwrapWritableRoots(launch, env)
	if err != nil {
		return launch, err
	}

	args := make([]string, 0, 32+len(roots)*3+len(env)*3+len(launch.Args))
	args = append(args,
		"--die-with-parent",
		"--new-session",
		"--ro-bind", "/", "/",
		"--dev-bind", "/dev", "/dev",
		"--proc", "/proc",
		"--clearenv",
	)
	if tempDir := strings.TrimSpace(env["TMPDIR"]); tempDir != "" {
		// Keep /tmp writable for tools that ignore TMPDIR and hardcode /tmp paths.
		args = append(args, "--bind", tempDir, "/tmp")
	}
	for _, root := range roots {
		args = append(args, "--bind", root, root)
	}

	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--setenv", key, env[key])
	}

	args = append(args, "--chdir", launch.WorkDir, launch.Command)
	args = append(args, launch.Args...)

	return acpclient.LaunchConfig{
		Command: command,
		Args:    args,
	}, nil
}

func buildBwrapEnv(launch acpclient.LaunchConfig) (map[string]string, error) {
	env := cloneLaunchEnv(launch.Env)
	homeKey, homeDir := detectLaunchHome(env)
	if homeDir != "" {
		if _, ok := env["HOME"]; !ok {
			env["HOME"] = homeDir
		}
		if homeKey == "CODEX_HOME" || homeKey == "CLAUDE_CONFIG_DIR" {
			env["XDG_CONFIG_HOME"] = homeDir
		}
		if _, ok := env["GOPATH"]; !ok {
			env["GOPATH"] = filepath.Join(homeDir, "go")
		}
		if _, ok := env["GOMODCACHE"]; !ok {
			env["GOMODCACHE"] = filepath.Join(env["GOPATH"], "pkg", "mod")
		}
	}

	tempDir := detectLaunchTemp(env)
	if tempDir == "" {
		return nil, fmt.Errorf("bwrap sandbox: TMPDIR/TMP/TEMP is required")
	}
	if _, ok := env["NPM_CONFIG_CACHE"]; !ok {
		env["NPM_CONFIG_CACHE"] = filepath.Join(tempDir, "npm-cache")
	}
	if _, ok := env["XDG_CACHE_HOME"]; !ok {
		env["XDG_CACHE_HOME"] = filepath.Join(tempDir, "xdg-cache")
	}
	if _, ok := env["PIP_CACHE_DIR"]; !ok {
		env["PIP_CACHE_DIR"] = filepath.Join(tempDir, "pip-cache")
	}
	if _, ok := env["UV_CACHE_DIR"]; !ok {
		env["UV_CACHE_DIR"] = filepath.Join(tempDir, "uv-cache")
	}
	if _, ok := env["PYTHONPYCACHEPREFIX"]; !ok {
		env["PYTHONPYCACHEPREFIX"] = filepath.Join(tempDir, "pycache")
	}
	if _, ok := env["GOCACHE"]; !ok {
		env["GOCACHE"] = filepath.Join(tempDir, "go-build")
	}
	if _, ok := env["GOTMPDIR"]; !ok {
		env["GOTMPDIR"] = filepath.Join(tempDir, "go-tmp")
	}

	if _, ok := env["PATH"]; !ok {
		env["PATH"] = os.Getenv("PATH")
	}
	for _, key := range bwrapInheritedEnvKeys() {
		if _, ok := env[key]; ok {
			continue
		}
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			env[key] = value
		}
	}

	for _, key := range []string{
		"TMPDIR",
		"TMP",
		"TEMP",
		"NPM_CONFIG_CACHE",
		"XDG_CACHE_HOME",
		"PIP_CACHE_DIR",
		"UV_CACHE_DIR",
		"PYTHONPYCACHEPREFIX",
		"GOCACHE",
		"GOTMPDIR",
	} {
		if value := strings.TrimSpace(env[key]); value != "" {
			if err := os.MkdirAll(value, 0o755); err != nil {
				return nil, fmt.Errorf("create bwrap dir %s: %w", value, err)
			}
		}
	}
	if value := strings.TrimSpace(env["GOMODCACHE"]); value != "" {
		if err := os.MkdirAll(value, 0o755); err != nil {
			return nil, fmt.Errorf("create bwrap dir %s: %w", value, err)
		}
	}
	return env, nil
}

func buildBwrapWritableRoots(launch acpclient.LaunchConfig, env map[string]string) ([]string, error) {
	roots := uniqueCleanPaths([]string{
		launch.WorkDir,
		env["CODEX_HOME"],
		env["CLAUDE_CONFIG_DIR"],
		env["HOME"],
		env["TMPDIR"],
		env["TMP"],
		env["TEMP"],
		env["NPM_CONFIG_CACHE"],
		env["XDG_CACHE_HOME"],
		env["PIP_CACHE_DIR"],
		env["UV_CACHE_DIR"],
		env["PYTHONPYCACHEPREFIX"],
		env["GOCACHE"],
		env["GOTMPDIR"],
		env["GOPATH"],
		env["GOMODCACHE"],
	})
	for _, root := range roots {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, fmt.Errorf("create bwrap root %s: %w", root, err)
		}
	}
	return roots, nil
}

func uniqueCleanPaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if path == "." {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func bwrapInheritedEnvKeys() []string {
	return []string{
		"LANG",
		"LC_ALL",
		"TERM",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"ALL_PROXY",
		"NO_PROXY",
		"http_proxy",
		"https_proxy",
		"all_proxy",
		"no_proxy",
		"SSL_CERT_FILE",
		"NODE_EXTRA_CA_CERTS",
		"NPM_CONFIG_REGISTRY",
		"npm_config_registry",
	}
}
