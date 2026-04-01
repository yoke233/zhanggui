package configruntime

import (
	"runtime"
	"strings"
	"testing"

	"github.com/yoke233/zhanggui/internal/platform/config"
)

func TestNormalizeDriverConfigForPlatform_CodexACP(t *testing.T) {
	t.Parallel()

	driver := config.RuntimeDriverConfig{
		ID:            "codex-acp",
		LaunchCommand: "npx",
		LaunchArgs:    []string{"-y", "@zed-industries/codex-acp"},
	}

	got := normalizeDriverConfigForPlatform(driver)

	if runtime.GOOS == "windows" {
		if len(got.LaunchArgs) < 6 || got.LaunchArgs[1] != "@zed-industries/codex-acp@0.9.5" {
			t.Fatalf("windows codex launch args = %#v, want pinned 0.9.5 plus overrides", got.LaunchArgs)
		}
	} else if len(got.LaunchArgs) < 8 || got.LaunchArgs[1] != "@zed-industries/codex-acp" {
		t.Fatalf("non-windows codex launch args = %#v, want unchanged package plus overrides", got.LaunchArgs)
	}

	assertContainsOverride(t, got.LaunchArgs, `approval_policy="never"`)
	if runtime.GOOS == "windows" {
		assertContainsOverride(t, got.LaunchArgs, `sandbox_mode="danger-full-access"`)
		assertNotContainsOverride(t, got.LaunchArgs, "sandbox_workspace_write.network_access")
	} else {
		assertContainsOverride(t, got.LaunchArgs, `sandbox_mode="workspace-write"`)
		assertContainsOverride(t, got.LaunchArgs, "sandbox_workspace_write.network_access=true")
	}
}

func TestNormalizeDriverConfigForPlatform_PreservesExistingOverrides(t *testing.T) {
	t.Parallel()

	driver := config.RuntimeDriverConfig{
		ID:            "codex-acp",
		LaunchCommand: "npx",
		LaunchArgs: []string{
			"-y",
			"@zed-industries/codex-acp@0.9.5",
			"-c", `approval_policy="on-request"`,
			"-c", `sandbox_mode="read-only"`,
		},
	}

	got := normalizeDriverConfigForPlatform(driver)
	assertContainsOverride(t, got.LaunchArgs, `approval_policy="on-request"`)
	if runtime.GOOS == "windows" {
		assertContainsOverride(t, got.LaunchArgs, `sandbox_mode="danger-full-access"`)
	} else {
		assertContainsOverride(t, got.LaunchArgs, `sandbox_mode="read-only"`)
	}
	assertNotContainsOverride(t, got.LaunchArgs, "sandbox_workspace_write.network_access")
}

func assertContainsOverride(t *testing.T, args []string, want string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-c" && args[i+1] == want {
			return
		}
	}
	t.Fatalf("launch args %#v do not contain override %q", args, want)
}

func assertNotContainsOverride(t *testing.T, args []string, key string) {
	t.Helper()
	prefix := key
	if !strings.Contains(prefix, "=") {
		prefix += "="
	}
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-c" && strings.HasPrefix(args[i+1], prefix) {
			t.Fatalf("launch args %#v unexpectedly contain override with prefix %q", args, prefix)
		}
	}
}

func TestNormalizeDriverConfigForPlatform_LeavesPinnedVersionUntouched(t *testing.T) {
	t.Parallel()

	driver := config.RuntimeDriverConfig{
		ID:            "codex-acp",
		LaunchCommand: "npx",
		LaunchArgs:    []string{"-y", "@zed-industries/codex-acp@0.9.5"},
	}

	got := normalizeDriverConfigForPlatform(driver)
	if len(got.LaunchArgs) < 2 || got.LaunchArgs[1] != "@zed-industries/codex-acp@0.9.5" {
		t.Fatalf("launch args = %#v, want existing pinned version preserved", got.LaunchArgs)
	}
	assertContainsOverride(t, got.LaunchArgs, `approval_policy="never"`)
	if runtime.GOOS == "windows" {
		assertContainsOverride(t, got.LaunchArgs, `sandbox_mode="danger-full-access"`)
		assertNotContainsOverride(t, got.LaunchArgs, "sandbox_workspace_write.network_access")
	} else {
		assertContainsOverride(t, got.LaunchArgs, `sandbox_mode="workspace-write"`)
		assertContainsOverride(t, got.LaunchArgs, "sandbox_workspace_write.network_access=true")
	}
}
