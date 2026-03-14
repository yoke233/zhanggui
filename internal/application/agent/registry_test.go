package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
	skillset "github.com/yoke233/ai-workflow/internal/skills"
)

func testDriverConfig() core.DriverConfig {
	return core.DriverConfig{
		LaunchCommand: "npx",
		LaunchArgs:    []string{"-y", "@test/claude-acp"},
		CapabilitiesMax: core.DriverCapabilities{
			FSRead: true, FSWrite: true, Terminal: true,
		},
	}
}

func testProfile(id string, role core.AgentRole, caps ...string) *core.AgentProfile {
	return &core.AgentProfile{
		ID:           id,
		Name:         id,
		Driver:       testDriverConfig(),
		Role:         role,
		Capabilities: caps,
	}
}

func TestConfigRegistry_ProfileCRUD(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()

	p := testProfile("worker-1", core.RoleWorker, "backend")

	// Create
	if err := reg.CreateProfile(ctx, p); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	// Duplicate
	if err := reg.CreateProfile(ctx, p); !errors.Is(err, core.ErrDuplicateProfile) {
		t.Fatalf("expected ErrDuplicateProfile, got %v", err)
	}

	// Get
	got, err := reg.GetProfile(ctx, "worker-1")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Role != core.RoleWorker {
		t.Fatalf("expected worker, got %s", got.Role)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0] != "backend" {
		t.Fatalf("unexpected capabilities: %v", got.Capabilities)
	}

	// List
	list, _ := reg.ListProfiles(ctx)
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	// Update
	p2 := testProfile("worker-1", core.RoleWorker, "backend", "frontend")
	if err := reg.UpdateProfile(ctx, p2); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got, _ = reg.GetProfile(ctx, "worker-1")
	if len(got.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(got.Capabilities))
	}

	// Update not found
	if err := reg.UpdateProfile(ctx, testProfile("nope", core.RoleWorker)); !errors.Is(err, core.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}

	// Delete
	if err := reg.DeleteProfile(ctx, "worker-1"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	list, _ = reg.ListProfiles(ctx)
	if len(list) != 0 {
		t.Fatalf("expected 0, got %d", len(list))
	}

	// Delete not found
	if err := reg.DeleteProfile(ctx, "nope"); !errors.Is(err, core.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestConfigRegistry_CapabilityOverflow(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()

	// Profile with read-only driver — requesting write should fail.
	p := &core.AgentProfile{
		ID:   "writer",
		Role: core.RoleWorker,
		Driver: core.DriverConfig{
			LaunchCommand: "cat",
			CapabilitiesMax: core.DriverCapabilities{
				FSRead: true, FSWrite: false, Terminal: false,
			},
		},
		ActionsAllowed: []core.AgentAction{core.AgentActionFSWrite},
	}
	if err := reg.CreateProfile(ctx, p); !errors.Is(err, core.ErrCapabilityOverflow) {
		t.Fatalf("expected ErrCapabilityOverflow, got %v", err)
	}

	// Profile that only reads — should succeed.
	p2 := &core.AgentProfile{
		ID:   "reader",
		Role: core.RoleSupport,
		Driver: core.DriverConfig{
			LaunchCommand: "cat",
			CapabilitiesMax: core.DriverCapabilities{
				FSRead: true, FSWrite: false, Terminal: false,
			},
		},
		ActionsAllowed: []core.AgentAction{core.AgentActionReadContext},
	}
	if err := reg.CreateProfile(ctx, p2); err != nil {
		t.Fatalf("CreateProfile reader: %v", err)
	}
}

func TestConfigRegistry_ResolveForAction(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()
	reg.LoadProfiles([]*core.AgentProfile{
		testProfile("lead", core.RoleLead, "planning"),
		testProfile("worker-be", core.RoleWorker, "backend"),
		testProfile("worker-fe", core.RoleWorker, "frontend"),
		testProfile("gate", core.RoleGate, "review"),
	})

	tests := []struct {
		name    string
		action  *core.Action
		wantID  string
		wantErr error
	}{
		{
			name:   "match by role",
			action: &core.Action{AgentRole: "lead"},
			wantID: "lead",
		},
		{
			name:   "match by role + capability",
			action: &core.Action{AgentRole: "worker", RequiredCapabilities: []string{"backend"}},
			wantID: "worker-be",
		},
		{
			name:   "match by capability only",
			action: &core.Action{RequiredCapabilities: []string{"frontend"}},
			wantID: "worker-fe",
		},
		{
			name:    "no match",
			action:  &core.Action{AgentRole: "worker", RequiredCapabilities: []string{"mobile"}},
			wantErr: core.ErrNoMatchingAgent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := reg.ResolveForAction(ctx, tt.action)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.ID != tt.wantID {
				t.Fatalf("expected profile %q, got %q", tt.wantID, p.ID)
			}
		})
	}
}

func TestConfigRegistry_ResolveByID(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()
	reg.LoadProfiles([]*core.AgentProfile{
		testProfile("lead", core.RoleLead),
	})

	p, err := reg.ResolveByID(ctx, "lead")
	if err != nil {
		t.Fatalf("ResolveByID: %v", err)
	}
	if p.ID != "lead" {
		t.Fatalf("expected lead, got %s", p.ID)
	}

	_, err = reg.ResolveByID(ctx, "nope")
	if !errors.Is(err, core.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestConfigRegistry_Resolve_EngineInterface(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()
	reg.LoadProfiles([]*core.AgentProfile{
		testProfile("worker", core.RoleWorker),
	})

	// Use as flow resolver interface.
	var resolver flowapp.Resolver = reg
	id, err := resolver.Resolve(ctx, &core.Action{AgentRole: "worker"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if id != "worker" {
		t.Fatalf("expected worker, got %s", id)
	}
}

func TestConfigRegistry_LoadBulk(t *testing.T) {
	reg := NewConfigRegistry()

	profiles := []*core.AgentProfile{
		testProfile("p1", core.RoleWorker),
		testProfile("p2", core.RoleGate),
	}
	reg.LoadProfiles(profiles)

	ctx := context.Background()
	pl, _ := reg.ListProfiles(ctx)
	if len(pl) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(pl))
	}
}

func TestConfigRegistry_RejectsInvalidSkillReference(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()

	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)
	dir := filepath.Join(dataDir, "skills", "strict-review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillset.DefaultSkillMD("strict-review")), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	p := testProfile("worker-1", core.RoleWorker, "backend")
	p.Skills = []string{"strict-review", "missing-skill"}
	if err := reg.CreateProfile(ctx, p); !errors.Is(err, core.ErrInvalidSkills) {
		t.Fatalf("expected ErrInvalidSkills, got %v", err)
	}
}
