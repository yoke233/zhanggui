package configruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func TestManager_WriteRawRejectsInvalidAndKeepsCurrent(t *testing.T) {
	manager, initialRaw := newManagerForTest(t)

	if _, err := manager.WriteRaw(context.Background(), "runtime = ["); err == nil {
		t.Fatalf("expected invalid toml error")
	}

	raw, err := manager.ReadRawString()
	if err != nil {
		t.Fatalf("ReadRawString() error = %v", err)
	}
	if raw != initialRaw {
		t.Fatalf("config should remain unchanged")
	}
	if manager.Status().ActiveVersion != 1 {
		t.Fatalf("unexpected active version: %d", manager.Status().ActiveVersion)
	}
}

func TestManager_UpdateConfigWritesBackAndReloads(t *testing.T) {
	manager, _ := newManagerForTest(t)

	_, err := manager.UpdateConfig(context.Background(),
		config.RuntimeAgentsConfig{
			Drivers: []config.RuntimeDriverConfig{{
				ID:            "codex",
				LaunchCommand: "npx",
				LaunchArgs:    []string{"-y", "@zed-industries/codex-acp"},
				CapabilitiesMax: config.CapabilitiesConfig{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			}},
			Profiles: []config.RuntimeProfileConfig{{
				ID:             "worker-default",
				Name:           "Worker",
				Driver:         "codex",
				Role:           "worker",
				ActionsAllowed: []string{"read_context"},
				PromptTemplate: "worker",
				Session:        config.RuntimeSessionConfig{Reuse: true, MaxTurns: 8},
				MCP:            config.MCPConfig{Enabled: true},
			}},
		},
		config.RuntimeMCPConfig{
			Servers: []config.RuntimeMCPServerConfig{{
				ID:        "query",
				Name:      "query",
				Kind:      "internal",
				Transport: "sse",
				Enabled:   true,
			}},
			ProfileBindings: []config.RuntimeMCPProfileBindingConfig{{
				Profile:  "worker-default",
				Server:   "query",
				Enabled:  true,
				ToolMode: "all",
			}},
		},
	)
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}

	agents, mcp, ok := manager.CurrentConfig()
	if !ok {
		t.Fatalf("CurrentConfig() ok = false, want true")
	}
	if len(agents.Profiles) != 1 || agents.Profiles[0].ID != "worker-default" {
		t.Fatalf("unexpected profiles: %+v", agents.Profiles)
	}
	if len(mcp.Servers) != 1 || mcp.Servers[0].ID != "query" {
		t.Fatalf("unexpected servers: %+v", mcp.Servers)
	}

	raw, err := manager.ReadRawString()
	if err != nil {
		t.Fatalf("ReadRawString() error = %v", err)
	}
	layer, err := config.LoadLayerBytes([]byte(raw))
	if err != nil {
		t.Fatalf("LoadLayerBytes() error = %v", err)
	}
	if layer.Runtime == nil || layer.Runtime.MCP == nil || layer.Runtime.Agents == nil {
		t.Fatalf("expected runtime sections written back")
	}
	if manager.Status().ActiveVersion < 2 {
		t.Fatalf("expected version to advance, got %d", manager.Status().ActiveVersion)
	}
}

func newManagerForTest(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	secretsPath := filepath.Join(dir, "secrets.toml")
	raw, err := os.ReadFile(filepath.Join("..", "config", "defaults.toml"))
	if err != nil {
		t.Fatalf("read defaults.toml: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(secretsPath, []byte(""), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	manager, err := NewManager(cfgPath, secretsPath, DisabledMCPEnv(), nil, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() {
		_ = manager.Close()
	})
	return manager, string(raw)
}
