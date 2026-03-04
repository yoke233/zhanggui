package views

import (
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestRenderRunListShowsCurrentStage(t *testing.T) {
	out := RenderRunList([]core.Run{
		{
			ID:           "p-1",
			Name:         "demo-pipe",
			Status:       core.StatusRunning,
			CurrentStage: core.StageImplement,
		},
	}, 0, map[string]func(string) string{
		"running": func(s string) string { return s },
	})

	if !strings.Contains(out, "implement") {
		t.Fatalf("expected current stage in list output, got: %s", out)
	}
}
