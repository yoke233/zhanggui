package flow

import (
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestManifestCheckEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   bool
	}{
		{"nil config", nil, false},
		{"empty config", map[string]any{}, false},
		{"false", map[string]any{"manifest_check": false}, false},
		{"true", map[string]any{"manifest_check": true}, true},
		{"string value", map[string]any{"manifest_check": "true"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &core.Action{Config: tt.config}
			if got := manifestCheckEnabled(action); got != tt.want {
				t.Errorf("manifestCheckEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
