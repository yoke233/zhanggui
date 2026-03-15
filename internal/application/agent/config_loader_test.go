package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func TestNewConfigRegistryFromConfig_LoadsProfiles(t *testing.T) {
	cfg := config.RuntimeAgentsConfig{
		Drivers: []config.RuntimeDriverConfig{
			{
				ID:            "codex",
				LaunchCommand: "codex",
				LaunchArgs:    []string{"exec"},
				Env:           map[string]string{"MODE": "test"},
				CapabilitiesMax: config.CapabilitiesConfig{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
		Profiles: []config.RuntimeProfileConfig{
			{
				ID:             "worker-1",
				Name:           "Worker One",
				Driver:         "codex",
				Role:           "worker",
				Capabilities:   []string{"backend"},
				ActionsAllowed: []string{"fs_write", "terminal"},
				PromptTemplate: "implement",
				Skills:         []string{"skill-a"},
				Session: config.RuntimeSessionConfig{
					Reuse:    true,
					MaxTurns: 7,
					IdleTTL:  config.Duration{Duration: 3 * time.Minute},
				},
				MCP: config.MCPConfig{
					Enabled: true,
					Tools:   []string{"shell_command"},
				},
			},
			{
				ID:   "worker-2",
				Name: "Worker Two",
				Role: "support",
			},
		},
	}

	reg := NewConfigRegistryFromConfig(cfg)

	got, err := reg.GetProfile(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("GetProfile(worker-1): %v", err)
	}
	if got.Driver.LaunchCommand != "codex" {
		t.Fatalf("LaunchCommand = %q, want codex", got.Driver.LaunchCommand)
	}
	if len(got.Driver.LaunchArgs) != 1 || got.Driver.LaunchArgs[0] != "exec" {
		t.Fatalf("LaunchArgs = %#v", got.Driver.LaunchArgs)
	}
	if got.Driver.Env["MODE"] != "test" {
		t.Fatalf("Env = %#v", got.Driver.Env)
	}
	if !got.Driver.CapabilitiesMax.Terminal {
		t.Fatalf("CapabilitiesMax = %#v", got.Driver.CapabilitiesMax)
	}
	if got.Role != core.RoleWorker {
		t.Fatalf("Role = %q, want worker", got.Role)
	}
	if len(got.ActionsAllowed) != 2 || got.ActionsAllowed[0] != core.AgentActionFSWrite || got.ActionsAllowed[1] != core.AgentActionTerminal {
		t.Fatalf("ActionsAllowed = %#v", got.ActionsAllowed)
	}
	if got.Session.IdleTTL != 3*time.Minute || !got.Session.Reuse || got.Session.MaxTurns != 7 {
		t.Fatalf("Session = %#v", got.Session)
	}
	if !got.MCP.Enabled || len(got.MCP.Tools) != 1 || got.MCP.Tools[0] != "shell_command" {
		t.Fatalf("MCP = %#v", got.MCP)
	}

	missingDriver, err := reg.GetProfile(context.Background(), "worker-2")
	if err != nil {
		t.Fatalf("GetProfile(worker-2): %v", err)
	}
	if missingDriver.Driver.LaunchCommand != "" || len(missingDriver.Driver.LaunchArgs) != 0 || len(missingDriver.Driver.Env) != 0 {
		t.Fatalf("missing driver should keep zero config, got %#v", missingDriver.Driver)
	}
}

func TestDriverCapabilitiesConfigured(t *testing.T) {
	tests := []struct {
		name string
		in   *core.AgentProfile
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "empty", in: &core.AgentProfile{}, want: false},
		{name: "launch command", in: &core.AgentProfile{Driver: core.DriverConfig{LaunchCommand: "codex"}}, want: true},
		{name: "launch args", in: &core.AgentProfile{Driver: core.DriverConfig{LaunchArgs: []string{"exec"}}}, want: true},
		{name: "env", in: &core.AgentProfile{Driver: core.DriverConfig{Env: map[string]string{"A": "1"}}}, want: true},
		{name: "caps", in: &core.AgentProfile{Driver: core.DriverConfig{CapabilitiesMax: core.DriverCapabilities{FSRead: true}}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := driverCapabilitiesConfigured(tt.in); got != tt.want {
				t.Fatalf("driverCapabilitiesConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCloneProfileDeepCopy(t *testing.T) {
	original := &core.AgentProfile{
		ID:             "worker",
		Capabilities:   []string{"backend"},
		ActionsAllowed: []core.AgentAction{core.AgentActionFSWrite},
		Skills:         []string{"skill-a"},
		MCP: core.ProfileMCP{
			Tools: []string{"shell_command"},
		},
		Driver: core.DriverConfig{
			LaunchArgs: []string{"exec"},
			Env:        map[string]string{"MODE": "test"},
		},
	}

	cloned := cloneProfile(original)
	cloned.Capabilities[0] = "frontend"
	cloned.ActionsAllowed[0] = core.AgentActionTerminal
	cloned.Skills[0] = "skill-b"
	cloned.MCP.Tools[0] = "other"
	cloned.Driver.LaunchArgs[0] = "run"
	cloned.Driver.Env["MODE"] = "prod"

	if original.Capabilities[0] != "backend" {
		t.Fatalf("Capabilities mutated: %#v", original.Capabilities)
	}
	if original.ActionsAllowed[0] != core.AgentActionFSWrite {
		t.Fatalf("ActionsAllowed mutated: %#v", original.ActionsAllowed)
	}
	if original.Skills[0] != "skill-a" {
		t.Fatalf("Skills mutated: %#v", original.Skills)
	}
	if original.MCP.Tools[0] != "shell_command" {
		t.Fatalf("MCP tools mutated: %#v", original.MCP.Tools)
	}
	if original.Driver.LaunchArgs[0] != "exec" {
		t.Fatalf("LaunchArgs mutated: %#v", original.Driver.LaunchArgs)
	}
	if original.Driver.Env["MODE"] != "test" {
		t.Fatalf("Env mutated: %#v", original.Driver.Env)
	}
}

func TestConfigRegistry_GetProfileNotFoundAndResolveError(t *testing.T) {
	reg := NewConfigRegistry()

	_, err := reg.GetProfile(context.Background(), "missing")
	if !errors.Is(err, core.ErrProfileNotFound) {
		t.Fatalf("GetProfile() error = %v, want ErrProfileNotFound", err)
	}

	_, err = reg.Resolve(context.Background(), &core.Action{AgentRole: "worker"})
	if !errors.Is(err, core.ErrNoMatchingAgent) {
		t.Fatalf("Resolve() error = %v, want ErrNoMatchingAgent", err)
	}
}
