package sandbox

import (
	"context"
	"testing"
)

func TestDefaultSupportInspectorDisabled(t *testing.T) {
	t.Parallel()

	report := NewDefaultSupportInspector(false, "").Inspect(context.Background())
	if report.CurrentProvider != "noop" {
		t.Fatalf("CurrentProvider = %q, want noop", report.CurrentProvider)
	}
	if report.CurrentSupported {
		t.Fatal("CurrentSupported = true, want false when sandbox disabled")
	}
	if !report.Providers["home_dir"].Supported {
		t.Fatal("home_dir should always be supported")
	}
}

func TestDefaultSupportInspectorUnknownProvider(t *testing.T) {
	t.Parallel()

	report := NewDefaultSupportInspector(true, "custom").Inspect(context.Background())
	if report.CurrentProvider != "custom" {
		t.Fatalf("CurrentProvider = %q, want custom", report.CurrentProvider)
	}
	if report.CurrentSupported {
		t.Fatal("CurrentSupported = true, want false for unknown provider")
	}
	if got := report.Providers["custom"].Reason; got == "" {
		t.Fatal("unknown provider reason should not be empty")
	}
}

func TestDetectLiteBoxSupport(t *testing.T) {
	t.Parallel()

	if got := detectLiteBoxSupport("darwin", "arm64"); got.Supported {
		t.Fatalf("darwin should not support litebox, got %#v", got)
	}
	if got := detectLiteBoxSupport("windows", "arm64"); got.Supported {
		t.Fatalf("windows arm64 should not support litebox, got %#v", got)
	}
	if got := detectLiteBoxSupport("windows", "amd64"); !got.Supported {
		t.Fatalf("windows amd64 should support litebox, got %#v", got)
	}
}
