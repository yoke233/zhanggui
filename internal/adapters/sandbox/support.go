package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
)

var (
	ErrSandboxConfigUnavailable = errors.New("sandbox config runtime unavailable")
	lookPath                    = exec.LookPath
)

type ProviderSupport struct {
	Supported   bool   `json:"supported"`
	Implemented bool   `json:"implemented"`
	Reason      string `json:"reason,omitempty"`
}

type SupportReport struct {
	OS                 string                     `json:"os"`
	Arch               string                     `json:"arch"`
	Enabled            bool                       `json:"enabled"`
	ConfiguredProvider string                     `json:"configured_provider"`
	CurrentProvider    string                     `json:"current_provider"`
	CurrentSupported   bool                       `json:"current_supported"`
	Providers          map[string]ProviderSupport `json:"providers"`
}

type UpdateRequest struct {
	Enabled  *bool   `json:"enabled,omitempty"`
	Provider *string `json:"provider,omitempty"`
}

// SupportInspector reports sandbox capability support for the current runtime.
type SupportInspector interface {
	Inspect(ctx context.Context) SupportReport
}

// ControlService reports support and optionally persists sandbox config changes.
type ControlService interface {
	SupportInspector
	Update(ctx context.Context, req UpdateRequest) (SupportReport, error)
}

type DefaultSupportInspector struct {
	cfg  config.RuntimeSandboxConfig
	os   string
	arch string
}

func NewDefaultSupportInspector(enabled bool, currentProvider string) DefaultSupportInspector {
	return DefaultSupportInspector{
		cfg: config.RuntimeSandboxConfig{
			Enabled:  enabled,
			Provider: currentProvider,
		},
		os:   runtime.GOOS,
		arch: runtime.GOARCH,
	}
}

func (i DefaultSupportInspector) Inspect(_ context.Context) SupportReport {
	return buildSupportReport(i.cfg, i.os, i.arch)
}

type ReadOnlyControlService struct {
	inspector SupportInspector
}

func NewReadOnlyControlService(inspector SupportInspector) ReadOnlyControlService {
	return ReadOnlyControlService{inspector: inspector}
}

func (s ReadOnlyControlService) Inspect(ctx context.Context) SupportReport {
	if s.inspector == nil {
		return NewDefaultSupportInspector(false, "").Inspect(ctx)
	}
	return s.inspector.Inspect(ctx)
}

func (s ReadOnlyControlService) Update(ctx context.Context, _ UpdateRequest) (SupportReport, error) {
	return s.Inspect(ctx), ErrSandboxConfigUnavailable
}

type RuntimeControlService struct {
	manager  *configruntime.Manager
	fallback config.RuntimeSandboxConfig
	os       string
	arch     string
}

func NewRuntimeControlService(manager *configruntime.Manager, fallback config.RuntimeSandboxConfig) RuntimeControlService {
	return RuntimeControlService{
		manager:  manager,
		fallback: fallback,
		os:       runtime.GOOS,
		arch:     runtime.GOARCH,
	}
}

func (s RuntimeControlService) Inspect(_ context.Context) SupportReport {
	return buildSupportReport(s.currentConfig(), s.os, s.arch)
}

func (s RuntimeControlService) Update(ctx context.Context, req UpdateRequest) (SupportReport, error) {
	if s.manager == nil {
		return s.Inspect(ctx), ErrSandboxConfigUnavailable
	}

	current := s.manager.GetRuntime()
	next := current.Sandbox
	if req.Enabled != nil {
		next.Enabled = *req.Enabled
	}
	if req.Provider != nil {
		next.Provider = strings.TrimSpace(*req.Provider)
	}
	next.Provider = normalizeProvider(next.Provider)

	if req.Provider != nil {
		if _, ok := knownProviderSupport(next.Provider, s.os, s.arch); !ok {
			return s.Inspect(ctx), fmt.Errorf("unknown sandbox provider %q", next.Provider)
		}
	}
	if next.Enabled {
		support, ok := knownProviderSupport(next.Provider, s.os, s.arch)
		if !ok {
			return s.Inspect(ctx), fmt.Errorf("unknown sandbox provider %q", next.Provider)
		}
		if !support.Supported || !support.Implemented {
			return s.Inspect(ctx), fmt.Errorf(
				"sandbox provider %q is unavailable on %s/%s: %s",
				next.Provider,
				s.os,
				s.arch,
				support.Reason,
			)
		}
	}

	current.Sandbox = next
	if _, err := s.manager.UpdateRuntime(ctx, current); err != nil {
		return s.Inspect(ctx), err
	}
	return s.Inspect(ctx), nil
}

func (s RuntimeControlService) currentConfig() config.RuntimeSandboxConfig {
	if s.manager != nil {
		if snap := s.manager.Current(); snap != nil && snap.Config != nil {
			return snap.Config.Runtime.Sandbox
		}
	}
	return s.fallback
}

func buildSupportReport(cfg config.RuntimeSandboxConfig, goos, goarch string) SupportReport {
	configuredProvider := normalizeProvider(cfg.Provider)
	currentProvider := "noop"
	if cfg.Enabled {
		currentProvider = configuredProvider
	}

	report := SupportReport{
		OS:                 goos,
		Arch:               goarch,
		Enabled:            cfg.Enabled,
		ConfiguredProvider: configuredProvider,
		CurrentProvider:    currentProvider,
		Providers:          detectProviders(goos, goarch),
	}
	if currentProvider == "noop" {
		report.CurrentSupported = false
	} else if support, ok := knownProviderSupport(currentProvider, goos, goarch); ok {
		report.CurrentSupported = support.Supported && support.Implemented
	} else {
		report.Providers[currentProvider] = ProviderSupport{
			Supported:   false,
			Implemented: false,
			Reason:      "未知 sandbox provider",
		}
		report.CurrentSupported = false
	}
	if _, ok := report.Providers[configuredProvider]; !ok {
		report.Providers[configuredProvider] = ProviderSupport{
			Supported:   false,
			Implemented: false,
			Reason:      "未知 sandbox provider",
		}
	}
	return report
}

func normalizeProvider(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" || normalized == "noop" {
		return "home_dir"
	}
	return normalized
}

func knownProviderSupport(provider string, goos string, goarch string) (ProviderSupport, bool) {
	switch provider {
	case "home_dir":
		return ProviderSupport{
			Supported:   true,
			Implemented: true,
			Reason:      "环境变量驱动的 HOME/TMP/skills 隔离",
		}, true
	case "litebox":
		return detectLiteBoxSupport(goos, goarch), true
	case "docker":
		return detectDockerSupport(), true
	case "bwrap":
		return detectBwrapSupport(goos), true
	default:
		return ProviderSupport{}, false
	}
}

func detectProviders(goos, goarch string) map[string]ProviderSupport {
	return map[string]ProviderSupport{
		"home_dir": {
			Supported:   true,
			Implemented: true,
			Reason:      "环境变量驱动的 HOME/TMP/skills 隔离",
		},
		"litebox": detectLiteBoxSupport(goos, goarch),
		"docker":  detectDockerSupport(),
		"bwrap":   detectBwrapSupport(goos),
	}
}

func detectLiteBoxSupport(goos, goarch string) ProviderSupport {
	if goos != "windows" {
		return ProviderSupport{
			Supported:   false,
			Implemented: true,
			Reason:      "litebox provider 当前仅支持 Windows 主机",
		}
	}
	if goarch != "amd64" {
		return ProviderSupport{
			Supported:   false,
			Implemented: true,
			Reason:      "litebox provider 当前仅支持 Windows amd64",
		}
	}
	return ProviderSupport{
		Supported:   true,
		Implemented: true,
		Reason:      "支持 LiteBox Windows runner；仍需额外配置 bridge_command 与 runner_path",
	}
}

func detectDockerSupport() ProviderSupport {
	if _, err := lookPath("docker"); err != nil {
		return ProviderSupport{
			Supported:   false,
			Implemented: true,
			Reason:      "未在 PATH 中发现 docker 命令",
		}
	}
	return ProviderSupport{
		Supported:   true,
		Implemented: true,
		Reason:      "docker provider 已接入，可作为 mac Intel / Windows 的通用兜底",
	}
}

func detectBwrapSupport(goos string) ProviderSupport {
	if goos != "linux" {
		return ProviderSupport{
			Supported:   false,
			Implemented: true,
			Reason:      "bwrap 仅适用于 Linux 主机",
		}
	}
	if _, err := lookPath("bwrap"); err != nil {
		return ProviderSupport{
			Supported:   false,
			Implemented: true,
			Reason:      "未在 PATH 中发现 bwrap 命令",
		}
	}
	return ProviderSupport{
		Supported:   true,
		Implemented: true,
		Reason:      "bwrap provider 已接入，适用于 Linux/WSL2 上的最小进程沙箱",
	}
}
