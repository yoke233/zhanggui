package configruntime

import (
	"context"
	"errors"
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

func TestManager_UpdateRuntimeWritesSandboxConfig(t *testing.T) {
	manager, _ := newManagerForTest(t)

	current := manager.GetRuntime()
	current.Sandbox = config.RuntimeSandboxConfig{
		Enabled:  true,
		Provider: "docker",
		LiteBox: config.RuntimeLiteBoxConfig{
			BridgeCommand: "litebox-acp",
		},
		Docker: config.RuntimeDockerConfig{
			Command:        "docker",
			Image:          "node:20-bookworm",
			RunArgs:        []string{"--pull=missing"},
			CPUs:           "1.5",
			Memory:         "3g",
			MemorySwap:     "3g",
			PidsLimit:      "256",
			Network:        "host",
			ReadOnlyRootFS: true,
			Tmpfs:          []string{"/tmp:size=512m"},
		},
	}

	if _, err := manager.UpdateRuntime(context.Background(), current); err != nil {
		t.Fatalf("UpdateRuntime() error = %v", err)
	}

	got := manager.GetRuntime().Sandbox
	if got.Provider != "docker" || !got.Enabled {
		t.Fatalf("unexpected sandbox config: %+v", got)
	}
	if got.Docker.Image != "node:20-bookworm" || !got.Docker.ReadOnlyRootFS {
		t.Fatalf("docker config not persisted: %+v", got.Docker)
	}

	raw, err := manager.ReadRawString()
	if err != nil {
		t.Fatalf("ReadRawString() error = %v", err)
	}
	layer, err := config.LoadLayerBytes([]byte(raw))
	if err != nil {
		t.Fatalf("LoadLayerBytes() error = %v", err)
	}
	if layer.Runtime == nil || layer.Runtime.Sandbox == nil || layer.Runtime.Sandbox.Docker == nil {
		t.Fatalf("expected runtime sandbox sections written back")
	}
	if layer.Runtime.Sandbox.Docker.Image == nil || *layer.Runtime.Sandbox.Docker.Image != "node:20-bookworm" {
		t.Fatalf("docker image missing in raw layer: %+v", layer.Runtime.Sandbox.Docker)
	}
}

func TestManager_UpdateRuntimeWritesLLMConfig(t *testing.T) {
	manager, _ := newManagerForTest(t)

	current := manager.GetRuntime()
	current.LLM = config.RuntimeLLMConfig{
		DefaultConfigID: "anthropic-main",
		Configs: []config.RuntimeLLMEntryConfig{
			{
				ID:              "openai-chat-main",
				Type:            "openai_chat_completion",
				BaseURL:         "https://openai.example.com/v1",
				APIKey:          "chat-key",
				Model:           "gpt-4.1",
				Temperature:     0.1,
				ReasoningEffort: "medium",
			},
			{
				ID:              "openai-response-main",
				Type:            "openai_response",
				BaseURL:         "https://responses.example.com/v1",
				APIKey:          "responses-key",
				Model:           "gpt-4.1-mini",
				MaxOutputTokens: 4096,
			},
			{
				ID:                   "anthropic-main",
				Type:                 "anthropic",
				BaseURL:              "https://api.anthropic.com",
				APIKey:               "anthropic-key",
				Model:                "claude-3-7-sonnet-latest",
				ThinkingBudgetTokens: 2048,
			},
		},
	}

	if _, err := manager.UpdateRuntime(context.Background(), current); err != nil {
		t.Fatalf("UpdateRuntime() error = %v", err)
	}

	got := manager.GetRuntime().LLM
	if got.DefaultConfigID != "anthropic-main" {
		t.Fatalf("default_config_id = %q, want anthropic-main", got.DefaultConfigID)
	}
	if len(got.Configs) != 3 {
		t.Fatalf("configs len = %d, want 3", len(got.Configs))
	}
	if got.Configs[1].APIKey != "responses-key" {
		t.Fatalf("openai response api key not persisted: %+v", got.Configs[1])
	}
	if got.Configs[0].ReasoningEffort != "medium" || got.Configs[1].MaxOutputTokens != 4096 || got.Configs[2].ThinkingBudgetTokens != 2048 {
		t.Fatalf("llm tuning fields not persisted: %+v", got.Configs)
	}

	raw, err := manager.ReadRawString()
	if err != nil {
		t.Fatalf("ReadRawString() error = %v", err)
	}
	layer, err := config.LoadLayerBytes([]byte(raw))
	if err != nil {
		t.Fatalf("LoadLayerBytes() error = %v", err)
	}
	if layer.Runtime == nil || layer.Runtime.LLM == nil {
		t.Fatalf("expected runtime llm section written back")
	}
	if layer.Runtime.LLM.DefaultConfigID == nil || *layer.Runtime.LLM.DefaultConfigID != "anthropic-main" {
		t.Fatalf("llm default_config_id missing in raw layer: %+v", layer.Runtime.LLM)
	}
	if layer.Runtime.LLM.Configs == nil || len(*layer.Runtime.LLM.Configs) != 3 {
		t.Fatalf("llm configs missing in raw layer: %+v", layer.Runtime.LLM)
	}
	if (*layer.Runtime.LLM.Configs)[1].Model != "gpt-4.1-mini" {
		t.Fatalf("openai response model missing in raw layer: %+v", (*layer.Runtime.LLM.Configs)[1])
	}
	if (*layer.Runtime.LLM.Configs)[2].ThinkingBudgetTokens != 2048 {
		t.Fatalf("anthropic thinking budget missing in raw layer: %+v", (*layer.Runtime.LLM.Configs)[2])
	}
}

func TestManager_DriverCRUD(t *testing.T) {
	manager, _ := newManagerForTest(t)
	initialCount := len(manager.ListDriverConfigs())

	if _, err := manager.CreateDriverConfig(context.Background(), config.RuntimeDriverConfig{
		ID:            "codex-cli",
		LaunchCommand: "codex",
		LaunchArgs:    []string{"run"},
		CapabilitiesMax: config.CapabilitiesConfig{
			FSRead:   true,
			FSWrite:  true,
			Terminal: true,
		},
	}); err != nil {
		t.Fatalf("CreateDriverConfig() error = %v", err)
	}

	items := manager.ListDriverConfigs()
	if len(items) != initialCount+1 {
		t.Fatalf("unexpected drivers after create: %+v", items)
	}
	found := false
	for _, item := range items {
		if item.ID == "codex-cli" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created driver codex-cli not found: %+v", items)
	}

	if _, err := manager.UpdateDriverConfig(context.Background(), "codex-cli", config.RuntimeDriverConfig{
		LaunchCommand: "codex-updated",
		LaunchArgs:    []string{"chat"},
		CapabilitiesMax: config.CapabilitiesConfig{
			FSRead:   true,
			FSWrite:  false,
			Terminal: true,
		},
	}); err != nil {
		t.Fatalf("UpdateDriverConfig() error = %v", err)
	}

	items = manager.ListDriverConfigs()
	found = false
	for _, item := range items {
		if item.ID == "codex-cli" && item.LaunchCommand == "codex-updated" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unexpected drivers after update: %+v", items)
	}

	if _, err := manager.DeleteDriverConfig(context.Background(), "codex-cli"); err != nil {
		t.Fatalf("DeleteDriverConfig() error = %v", err)
	}

	if got := manager.ListDriverConfigs(); len(got) != initialCount {
		t.Fatalf("unexpected drivers after delete, got %+v", got)
	} else {
		for _, item := range got {
			if item.ID == "codex-cli" {
				t.Fatalf("deleted driver codex-cli still present: %+v", got)
			}
		}
	}
}

func TestManager_DeleteDriverConfigRejectsInUseDriver(t *testing.T) {
	manager, _ := newManagerForTest(t)

	current := manager.GetRuntime()
	current.Agents.Drivers = []config.RuntimeDriverConfig{{
		ID:            "codex-cli",
		LaunchCommand: "codex",
		CapabilitiesMax: config.CapabilitiesConfig{
			FSRead:   true,
			FSWrite:  true,
			Terminal: true,
		},
	}}
	current.Agents.Profiles = []config.RuntimeProfileConfig{{
		ID:             "worker-a",
		Name:           "Worker A",
		Driver:         "codex-cli",
		Role:           "worker",
		PromptTemplate: "worker",
	}}
	if _, err := manager.UpdateRuntime(context.Background(), current); err != nil {
		t.Fatalf("UpdateRuntime() error = %v", err)
	}

	if _, err := manager.DeleteDriverConfig(context.Background(), "codex-cli"); err == nil || !errors.Is(err, ErrDriverInUse) {
		t.Fatalf("expected ErrDriverInUse, got %v", err)
	}
}

func newManagerForTest(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	secretsPath := filepath.Join(dir, "secrets.toml")
	// Keep the runtime layer file minimal for tests. The manager already loads
	// process defaults via config.Defaults(), and the full defaults.toml now
	// contains sections that are not part of ConfigLayer.
	raw := []byte("")
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
