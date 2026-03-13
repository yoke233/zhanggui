package sandbox

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

// Sandbox prepares a launch environment for an ACP agent process.
//
// Design goals:
// - Make per-process isolation (HOME/config dir, tmp, skills) pluggable by environment.
// - Keep engine code unaware of platform-specific details (Windows junction, etc.).
// - Allow future sandboxes to also override command/args for agent-native sandbox flags.
type Sandbox interface {
	Prepare(ctx context.Context, in PrepareInput) (acpclient.LaunchConfig, error)
}

type PrepareInput struct {
	Profile *core.AgentProfile
	Driver  *core.AgentDriver

	Launch acpclient.LaunchConfig

	// Scope identifies the sandbox instance directory.
	// For pooled sessions it should be stable (e.g. flow-<id>), while for one-off
	// processes it can be unique (e.g. flow-<id>-exec-<id>).
	Scope string

	// ExtraSkills are dynamically injected skill names (e.g. "step-signal")
	// that should be linked alongside Profile.Skills.
	ExtraSkills []string

	// EphemeralSkills maps skill names to pre-built directories on disk.
	// These directories are linked directly into the agent's skills dir,
	// bypassing the global skillsRoot. Used for per-execution materials.
	EphemeralSkills map[string]string
}

// NoopSandbox leaves launch config unchanged.
type NoopSandbox struct{}

func (NoopSandbox) Prepare(_ context.Context, in PrepareInput) (acpclient.LaunchConfig, error) {
	return in.Launch, nil
}

