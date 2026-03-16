package sandbox

import (
	"context"
	"errors"
	"testing"

	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func TestDefaultSupportInspectorDisabled(t *testing.T) {
	report := NewDefaultSupportInspector(false, "").Inspect(context.Background())
	if report.CurrentProvider != "noop" {
		t.Fatalf("CurrentProvider = %q, want noop", report.CurrentProvider)
	}
	if report.ConfiguredProvider != "home_dir" {
		t.Fatalf("ConfiguredProvider = %q, want home_dir", report.ConfiguredProvider)
	}
	if report.CurrentSupported {
		t.Fatal("CurrentSupported = true, want false when sandbox disabled")
	}
	homeDir := report.Providers["home_dir"]
	if !homeDir.Supported {
		t.Fatal("home_dir should always be supported")
	}
	if !homeDir.Implemented {
		t.Fatal("home_dir should be implemented")
	}
}

func TestDefaultSupportInspectorUnknownProvider(t *testing.T) {
	report := NewDefaultSupportInspector(true, "custom").Inspect(context.Background())
	if report.CurrentProvider != "custom" {
		t.Fatalf("CurrentProvider = %q, want custom", report.CurrentProvider)
	}
	if report.CurrentSupported {
		t.Fatal("CurrentSupported = true, want false for unknown provider")
	}
	custom := report.Providers["custom"]
	if got := custom.Reason; got == "" {
		t.Fatal("unknown provider reason should not be empty")
	}
	if custom.Implemented {
		t.Fatal("unknown provider must not be marked implemented")
	}
}

func TestBuildSupportReportImplementedSemantics(t *testing.T) {
	patchLookPath(t, func(name string) (string, error) {
		switch name {
		case "docker":
			return "C:\\bin\\" + name, nil
		default:
			return "", errors.New("missing")
		}
	})

	report := buildSupportReport(config.RuntimeSandboxConfig{
		Enabled:  true,
		Provider: "docker",
	}, "linux", "amd64")
	if !report.CurrentSupported {
		t.Fatal("docker should be current_supported after provider is implemented and command exists")
	}
	if !report.Providers["docker"].Implemented {
		t.Fatal("docker should be reported as implemented")
	}
}

func TestDetectLiteBoxSupport(t *testing.T) {
	if got := detectLiteBoxSupport("darwin", "arm64"); got.Supported {
		t.Fatalf("darwin should not support litebox, got %#v", got)
	}
	if !detectLiteBoxSupport("darwin", "arm64").Implemented {
		t.Fatal("litebox should be marked implemented even when host unsupported")
	}
	if got := detectLiteBoxSupport("windows", "arm64"); got.Supported {
		t.Fatalf("windows arm64 should not support litebox, got %#v", got)
	}
	if got := detectLiteBoxSupport("windows", "amd64"); !got.Supported {
		t.Fatalf("windows amd64 should support litebox, got %#v", got)
	}
}

func TestDetectDockerSupportRequiresCommand(t *testing.T) {
	patchLookPath(t, func(name string) (string, error) {
		return "", errors.New("missing")
	})

	got := detectDockerSupport()
	if got.Supported {
		t.Fatalf("docker should require command presence, got %#v", got)
	}
	if !got.Implemented {
		t.Fatalf("docker should be implemented, got %#v", got)
	}
}

func TestDetectBwrapSupportRequiresCommand(t *testing.T) {
	patchLookPath(t, func(name string) (string, error) {
		return "", errors.New("missing")
	})

	got := detectBwrapSupport("linux")
	if got.Supported {
		t.Fatalf("bwrap should require command presence, got %#v", got)
	}
	if !got.Implemented {
		t.Fatalf("bwrap should be implemented, got %#v", got)
	}
}

func patchLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	previous := lookPath
	lookPath = fn
	t.Cleanup(func() {
		lookPath = previous
	})
}

func TestDetectDockerSupportImplemented(t *testing.T) {
	if got := detectDockerSupport(); !got.Implemented {
		t.Fatalf("docker support should always report implementation presence, got %#v", got)
	}
}

func TestDetectBwrapSupportImplemented(t *testing.T) {
	if got := detectBwrapSupport("linux"); !got.Implemented {
		t.Fatalf("linux should mark bwrap implemented, got %#v", got)
	}
}
