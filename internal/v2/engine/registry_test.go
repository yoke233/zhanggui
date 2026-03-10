package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

func testDriver(id string) *core.AgentDriver {
	return &core.AgentDriver{
		ID:            id,
		LaunchCommand: "npx",
		LaunchArgs:    []string{"-y", "@test/" + id},
		CapabilitiesMax: core.DriverCapabilities{
			FSRead: true, FSWrite: true, Terminal: true,
		},
	}
}

func testProfile(id, driverID string, role core.AgentRole, caps ...string) *core.AgentProfile {
	return &core.AgentProfile{
		ID:           id,
		Name:         id,
		DriverID:     driverID,
		Role:         role,
		Capabilities: caps,
	}
}

func TestConfigRegistry_DriverCRUD(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()

	d := testDriver("claude-acp")

	// Create
	if err := reg.CreateDriver(ctx, d); err != nil {
		t.Fatalf("CreateDriver: %v", err)
	}

	// Duplicate
	if err := reg.CreateDriver(ctx, d); !errors.Is(err, core.ErrDuplicateDriver) {
		t.Fatalf("expected ErrDuplicateDriver, got %v", err)
	}

	// Get
	got, err := reg.GetDriver(ctx, "claude-acp")
	if err != nil {
		t.Fatalf("GetDriver: %v", err)
	}
	if got.LaunchCommand != "npx" {
		t.Fatalf("expected npx, got %s", got.LaunchCommand)
	}

	// Get not found
	_, err = reg.GetDriver(ctx, "nope")
	if !errors.Is(err, core.ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}

	// List
	list, err := reg.ListDrivers(ctx)
	if err != nil {
		t.Fatalf("ListDrivers: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	// Update
	d2 := testDriver("claude-acp")
	d2.LaunchCommand = "node"
	if err := reg.UpdateDriver(ctx, d2); err != nil {
		t.Fatalf("UpdateDriver: %v", err)
	}
	got, _ = reg.GetDriver(ctx, "claude-acp")
	if got.LaunchCommand != "node" {
		t.Fatalf("expected node, got %s", got.LaunchCommand)
	}

	// Update not found
	if err := reg.UpdateDriver(ctx, testDriver("nope")); !errors.Is(err, core.ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}

	// Delete not found
	if err := reg.DeleteDriver(ctx, "nope"); !errors.Is(err, core.ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}

	// Delete — add a profile referencing it first
	p := testProfile("worker", "claude-acp", core.RoleWorker)
	_ = reg.CreateProfile(ctx, p)
	if err := reg.DeleteDriver(ctx, "claude-acp"); !errors.Is(err, core.ErrDriverInUse) {
		t.Fatalf("expected ErrDriverInUse, got %v", err)
	}
	_ = reg.DeleteProfile(ctx, "worker")

	// Now delete should succeed
	if err := reg.DeleteDriver(ctx, "claude-acp"); err != nil {
		t.Fatalf("DeleteDriver: %v", err)
	}
	list, _ = reg.ListDrivers(ctx)
	if len(list) != 0 {
		t.Fatalf("expected 0, got %d", len(list))
	}
}

func TestConfigRegistry_ProfileCRUD(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()
	reg.LoadDrivers([]*core.AgentDriver{testDriver("claude-acp")})

	p := testProfile("worker-1", "claude-acp", core.RoleWorker, "dev.backend")

	// Create
	if err := reg.CreateProfile(ctx, p); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	// Duplicate
	if err := reg.CreateProfile(ctx, p); !errors.Is(err, core.ErrDuplicateProfile) {
		t.Fatalf("expected ErrDuplicateProfile, got %v", err)
	}

	// Create with missing driver
	bad := testProfile("orphan", "nope", core.RoleWorker)
	if err := reg.CreateProfile(ctx, bad); !errors.Is(err, core.ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}

	// Get
	got, err := reg.GetProfile(ctx, "worker-1")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Role != core.RoleWorker {
		t.Fatalf("expected worker, got %s", got.Role)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0] != "dev.backend" {
		t.Fatalf("unexpected capabilities: %v", got.Capabilities)
	}

	// List
	list, _ := reg.ListProfiles(ctx)
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	// Update
	p2 := testProfile("worker-1", "claude-acp", core.RoleWorker, "dev.backend", "dev.frontend")
	if err := reg.UpdateProfile(ctx, p2); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got, _ = reg.GetProfile(ctx, "worker-1")
	if len(got.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(got.Capabilities))
	}

	// Update not found
	if err := reg.UpdateProfile(ctx, testProfile("nope", "claude-acp", core.RoleWorker)); !errors.Is(err, core.ErrProfileNotFound) {
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

	// Driver with read-only capabilities.
	d := &core.AgentDriver{
		ID:            "read-only",
		LaunchCommand: "cat",
		CapabilitiesMax: core.DriverCapabilities{
			FSRead: true, FSWrite: false, Terminal: false,
		},
	}
	reg.LoadDrivers([]*core.AgentDriver{d})

	// Profile that requests write — should fail.
	p := &core.AgentProfile{
		ID:             "writer",
		DriverID:       "read-only",
		Role:           core.RoleWorker,
		ActionsAllowed: []core.Action{core.ActionFSWrite},
	}
	if err := reg.CreateProfile(ctx, p); !errors.Is(err, core.ErrCapabilityOverflow) {
		t.Fatalf("expected ErrCapabilityOverflow, got %v", err)
	}

	// Profile that only reads — should succeed.
	p2 := &core.AgentProfile{
		ID:             "reader",
		DriverID:       "read-only",
		Role:           core.RoleSupport,
		ActionsAllowed: []core.Action{core.ActionReadContext},
	}
	if err := reg.CreateProfile(ctx, p2); err != nil {
		t.Fatalf("CreateProfile reader: %v", err)
	}
}

func TestConfigRegistry_ResolveForStep(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()
	reg.LoadDrivers([]*core.AgentDriver{testDriver("claude-acp"), testDriver("codex-acp")})
	reg.LoadProfiles([]*core.AgentProfile{
		testProfile("lead", "claude-acp", core.RoleLead, "planning"),
		testProfile("worker-be", "codex-acp", core.RoleWorker, "dev.backend"),
		testProfile("worker-fe", "claude-acp", core.RoleWorker, "dev.frontend"),
		testProfile("gate", "claude-acp", core.RoleGate, "code.review"),
	})

	tests := []struct {
		name      string
		step      *core.Step
		wantID    string
		wantErr   error
	}{
		{
			name:   "match by role",
			step:   &core.Step{AgentRole: "lead"},
			wantID: "lead",
		},
		{
			name:   "match by role + capability",
			step:   &core.Step{AgentRole: "worker", RequiredCapabilities: []string{"dev.backend"}},
			wantID: "worker-be",
		},
		{
			name:   "match by capability only",
			step:   &core.Step{RequiredCapabilities: []string{"dev.frontend"}},
			wantID: "worker-fe",
		},
		{
			name:    "no match",
			step:    &core.Step{AgentRole: "worker", RequiredCapabilities: []string{"dev.mobile"}},
			wantErr: core.ErrNoMatchingAgent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, d, err := reg.ResolveForStep(ctx, tt.step)
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
			if d == nil {
				t.Fatal("expected non-nil driver")
			}
		})
	}
}

func TestConfigRegistry_ResolveByID(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()
	reg.LoadDrivers([]*core.AgentDriver{testDriver("claude-acp")})
	reg.LoadProfiles([]*core.AgentProfile{
		testProfile("lead", "claude-acp", core.RoleLead),
	})

	p, d, err := reg.ResolveByID(ctx, "lead")
	if err != nil {
		t.Fatalf("ResolveByID: %v", err)
	}
	if p.ID != "lead" {
		t.Fatalf("expected lead, got %s", p.ID)
	}
	if d.ID != "claude-acp" {
		t.Fatalf("expected claude-acp, got %s", d.ID)
	}

	_, _, err = reg.ResolveByID(ctx, "nope")
	if !errors.Is(err, core.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestConfigRegistry_Resolve_EngineInterface(t *testing.T) {
	ctx := context.Background()
	reg := NewConfigRegistry()
	reg.LoadDrivers([]*core.AgentDriver{testDriver("claude-acp")})
	reg.LoadProfiles([]*core.AgentProfile{
		testProfile("worker", "claude-acp", core.RoleWorker),
	})

	// Use as engine.Resolver interface.
	var resolver Resolver = reg
	id, err := resolver.Resolve(ctx, &core.Step{AgentRole: "worker"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if id != "worker" {
		t.Fatalf("expected worker, got %s", id)
	}
}

func TestConfigRegistry_LoadBulk(t *testing.T) {
	reg := NewConfigRegistry()

	drivers := []*core.AgentDriver{testDriver("a"), testDriver("b")}
	profiles := []*core.AgentProfile{
		testProfile("p1", "a", core.RoleWorker),
		testProfile("p2", "b", core.RoleGate),
	}
	reg.LoadDrivers(drivers)
	reg.LoadProfiles(profiles)

	ctx := context.Background()
	dl, _ := reg.ListDrivers(ctx)
	pl, _ := reg.ListProfiles(ctx)
	if len(dl) != 2 {
		t.Fatalf("expected 2 drivers, got %d", len(dl))
	}
	if len(pl) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(pl))
	}
}
