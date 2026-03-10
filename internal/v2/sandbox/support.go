package sandbox

import (
	"context"
	"runtime"
	"strings"
)

type ProviderSupport struct {
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

type SupportReport struct {
	OS               string                     `json:"os"`
	Arch             string                     `json:"arch"`
	Enabled          bool                       `json:"enabled"`
	CurrentProvider  string                     `json:"current_provider"`
	CurrentSupported bool                       `json:"current_supported"`
	Providers        map[string]ProviderSupport `json:"providers"`
}

// SupportInspector reports sandbox capability support for the current runtime.
type SupportInspector interface {
	Inspect(ctx context.Context) SupportReport
}

type DefaultSupportInspector struct {
	enabled         bool
	currentProvider string
	os              string
	arch            string
}

func NewDefaultSupportInspector(enabled bool, currentProvider string) DefaultSupportInspector {
	provider := strings.ToLower(strings.TrimSpace(currentProvider))
	if provider == "" {
		if enabled {
			provider = "home_dir"
		} else {
			provider = "noop"
		}
	}
	return DefaultSupportInspector{
		enabled:         enabled,
		currentProvider: provider,
		os:              runtime.GOOS,
		arch:            runtime.GOARCH,
	}
}

func (i DefaultSupportInspector) Inspect(_ context.Context) SupportReport {
	report := SupportReport{
		OS:              i.os,
		Arch:            i.arch,
		Enabled:         i.enabled,
		CurrentProvider: i.currentProvider,
		Providers: map[string]ProviderSupport{
			"home_dir": {Supported: true, Reason: "环境变量驱动的 HOME/TMP/skills 隔离"},
			"litebox":  detectLiteBoxSupport(i.os, i.arch),
		},
	}

	switch i.currentProvider {
	case "", "noop":
		report.CurrentSupported = false
	case "home_dir", "litebox":
		report.CurrentSupported = report.Providers[i.currentProvider].Supported
	default:
		report.CurrentSupported = false
		report.Providers[i.currentProvider] = ProviderSupport{
			Supported: false,
			Reason:    "未知 sandbox provider",
		}
	}
	return report
}

func detectLiteBoxSupport(goos, goarch string) ProviderSupport {
	if goos != "windows" {
		return ProviderSupport{
			Supported: false,
			Reason:    "litebox provider 当前仅支持 Windows 主机",
		}
	}
	if goarch != "amd64" {
		return ProviderSupport{
			Supported: false,
			Reason:    "litebox provider 当前仅支持 Windows amd64",
		}
	}
	return ProviderSupport{
		Supported: true,
		Reason:    "支持 LiteBox Windows runner；仍需额外配置 bridge_command 与 runner_path",
	}
}
