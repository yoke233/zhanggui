package sandbox

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
	"github.com/yoke233/zhanggui/internal/platform/config"
	"github.com/yoke233/zhanggui/internal/platform/configruntime"
)

func FromRuntimeConfig(cfg config.RuntimeSandboxConfig, dataDir string) Sandbox {
	requireAuth := false
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv("AI_WORKFLOW_CODEX_REQUIRE_AUTH"))); raw != "" {
		switch raw {
		case "1", "true", "yes", "on":
			requireAuth = true
		}
	}

	homeSandbox := HomeDirSandbox{
		DataDir:          dataDir,
		SkillsRoot:       filepath.Join(dataDir, "skills"),
		RequireCodexAuth: requireAuth,
	}

	if !cfg.Enabled {
		return homeSandbox
	}

	switch normalizeProvider(cfg.Provider) {
	case "home_dir":
		return homeSandbox
	case "litebox":
		return LiteBoxSandbox{
			Base:          homeSandbox,
			BridgeCommand: strings.TrimSpace(cfg.LiteBox.BridgeCommand),
			BridgeArgs:    append([]string(nil), cfg.LiteBox.BridgeArgs...),
			RunnerPath:    strings.TrimSpace(cfg.LiteBox.RunnerPath),
			RunnerArgs:    append([]string(nil), cfg.LiteBox.RunnerArgs...),
		}
	case "docker":
		return DockerSandbox{
			Base:           homeSandbox,
			Command:        strings.TrimSpace(cfg.Docker.Command),
			Image:          strings.TrimSpace(cfg.Docker.Image),
			RunArgs:        append([]string(nil), cfg.Docker.RunArgs...),
			CPUs:           strings.TrimSpace(cfg.Docker.CPUs),
			Memory:         strings.TrimSpace(cfg.Docker.Memory),
			MemorySwap:     strings.TrimSpace(cfg.Docker.MemorySwap),
			PidsLimit:      strings.TrimSpace(cfg.Docker.PidsLimit),
			Network:        strings.TrimSpace(cfg.Docker.Network),
			ReadOnlyRootFS: cfg.Docker.ReadOnlyRootFS,
			Tmpfs:          append([]string(nil), cfg.Docker.Tmpfs...),
		}
	case "bwrap":
		return BwrapSandbox{
			Base:    homeSandbox,
			Command: "bwrap",
		}
	default:
		slog.Warn("sandbox: unknown provider, fallback to home_dir", "provider", cfg.Provider)
		return homeSandbox
	}
}

type RuntimeSandbox struct {
	manager  *configruntime.Manager
	fallback config.RuntimeSandboxConfig
	dataDir  string
}

func NewRuntimeSandbox(manager *configruntime.Manager, fallback config.RuntimeSandboxConfig, dataDir string) RuntimeSandbox {
	return RuntimeSandbox{
		manager:  manager,
		fallback: fallback,
		dataDir:  dataDir,
	}
}

func (s RuntimeSandbox) Prepare(ctx context.Context, in PrepareInput) (acpclient.LaunchConfig, error) {
	cfg := s.fallback
	if s.manager != nil {
		if snap := s.manager.Current(); snap != nil && snap.Config != nil {
			cfg = snap.Config.Runtime.Sandbox
		}
	}
	return FromRuntimeConfig(cfg, s.dataDir).Prepare(ctx, in)
}
