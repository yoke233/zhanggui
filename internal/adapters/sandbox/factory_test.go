package sandbox

import (
	"testing"

	"github.com/yoke233/zhanggui/internal/platform/config"
)

func TestFromRuntimeConfig_DisabledStillUsesHomeDirSandbox(t *testing.T) {
	sb := FromRuntimeConfig(config.RuntimeSandboxConfig{
		Enabled:  false,
		Provider: "home_dir",
	}, t.TempDir())

	if _, ok := sb.(HomeDirSandbox); !ok {
		t.Fatalf("disabled runtime sandbox should still return HomeDirSandbox, got %T", sb)
	}
}
