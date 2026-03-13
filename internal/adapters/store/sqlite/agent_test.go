package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	skillset "github.com/yoke233/ai-workflow/internal/skills"
)

func newAgentTestStore(t *testing.T) *Store {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)
	createTestSkill(t, filepath.Join(dataDir, "skills"), "strict-review")
	createTestSkill(t, filepath.Join(dataDir, "skills"), "writing-wave-plans")

	dbPath := filepath.Join(dataDir, "agent_test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func createTestSkill(t *testing.T, root string, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillset.DefaultSkillMD(name)), 0o644); err != nil {
		t.Fatalf("write skill %s: %v", name, err)
	}
}

func testDriver(id string) *core.AgentDriver {
	return &core.AgentDriver{
		ID:            id,
		LaunchCommand: "npx",
		LaunchArgs:    []string{"-y", "@test/" + id},
		Env:           map[string]string{"KEY": "val"},
		CapabilitiesMax: core.DriverCapabilities{
			FSRead: true, FSWrite: true, Terminal: true,
		},
	}
}

func testProfile(id, driverID string, role core.AgentRole, caps ...string) *core.AgentProfile {
	return &core.AgentProfile{
		ID:             id,
		Name:           id,
		DriverID:       driverID,
		Role:           role,
		Capabilities:   caps,
		ActionsAllowed: []core.AgentAction{core.AgentActionReadContext, core.AgentActionFSWrite},
		PromptTemplate: "test-tmpl",
		Skills:         []string{"strict-review"},
		Session: core.ProfileSession{
			Reuse:    true,
			MaxTurns: 10,
			IdleTTL:  5 * time.Minute,
		},
		MCP: core.ProfileMCP{
			Enabled: true,
			Tools:   []string{"tool-a", "tool-b"},
		},
	}
}

func TestDriverCRUD(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	d := testDriver("claude-acp")

	// Create
	if err := s.CreateDriver(ctx, d); err != nil {
		t.Fatalf("CreateDriver: %v", err)
	}

	// Duplicate
	if err := s.CreateDriver(ctx, d); !errors.Is(err, core.ErrDuplicateDriver) {
		t.Fatalf("expected ErrDuplicateDriver, got %v", err)
	}

	// Get
	got, err := s.GetDriver(ctx, "claude-acp")
	if err != nil {
		t.Fatalf("GetDriver: %v", err)
	}
	if got.LaunchCommand != "npx" {
		t.Fatalf("expected npx, got %s", got.LaunchCommand)
	}
	if len(got.LaunchArgs) != 2 {
		t.Fatalf("expected 2 args, got %d", len(got.LaunchArgs))
	}
	if got.Env["KEY"] != "val" {
		t.Fatalf("expected env KEY=val, got %v", got.Env)
	}
	if !got.CapabilitiesMax.FSRead || !got.CapabilitiesMax.FSWrite || !got.CapabilitiesMax.Terminal {
		t.Fatalf("expected all caps true, got %+v", got.CapabilitiesMax)
	}

	// List
	list, err := s.ListDrivers(ctx)
	if err != nil {
		t.Fatalf("ListDrivers: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	// Update
	d.LaunchCommand = "node"
	d.CapabilitiesMax.Terminal = false
	if err := s.UpdateDriver(ctx, d); err != nil {
		t.Fatalf("UpdateDriver: %v", err)
	}
	got, _ = s.GetDriver(ctx, "claude-acp")
	if got.LaunchCommand != "node" {
		t.Fatalf("expected node, got %s", got.LaunchCommand)
	}
	if got.CapabilitiesMax.Terminal {
		t.Fatal("expected terminal=false after update")
	}

	// Update nonexistent
	if err := s.UpdateDriver(ctx, &core.AgentDriver{ID: "nope"}); !errors.Is(err, core.ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}

	// Delete with profile referencing → should fail
	p := testProfile("worker", "claude-acp", core.RoleWorker)
	if err := s.CreateProfile(ctx, p); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if err := s.DeleteDriver(ctx, "claude-acp"); !errors.Is(err, core.ErrDriverInUse) {
		t.Fatalf("expected ErrDriverInUse, got %v", err)
	}

	// Delete profile first, then driver
	if err := s.DeleteProfile(ctx, "worker"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if err := s.DeleteDriver(ctx, "claude-acp"); err != nil {
		t.Fatalf("DeleteDriver: %v", err)
	}

	// Get nonexistent
	if _, err := s.GetDriver(ctx, "claude-acp"); !errors.Is(err, core.ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}

	// Delete nonexistent
	if err := s.DeleteDriver(ctx, "nope"); !errors.Is(err, core.ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}
}

func TestProfileCRUD(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	d := testDriver("claude-acp")
	s.CreateDriver(ctx, d)

	p := testProfile("worker-1", "claude-acp", core.RoleWorker, "go", "backend")

	// Create
	if err := s.CreateProfile(ctx, p); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	// Duplicate
	if err := s.CreateProfile(ctx, p); !errors.Is(err, core.ErrDuplicateProfile) {
		t.Fatalf("expected ErrDuplicateProfile, got %v", err)
	}

	// Create with missing driver
	bad := testProfile("orphan", "nope", core.RoleWorker)
	if err := s.CreateProfile(ctx, bad); !errors.Is(err, core.ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}

	// Get
	got, err := s.GetProfile(ctx, "worker-1")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Role != core.RoleWorker {
		t.Fatalf("expected worker, got %s", got.Role)
	}
	if len(got.Capabilities) != 2 || got.Capabilities[0] != "go" {
		t.Fatalf("unexpected capabilities: %v", got.Capabilities)
	}
	if got.PromptTemplate != "test-tmpl" {
		t.Fatalf("expected test-tmpl, got %s", got.PromptTemplate)
	}
	if !got.Session.Reuse || got.Session.MaxTurns != 10 {
		t.Fatalf("session not preserved: %+v", got.Session)
	}
	if got.Session.IdleTTL != 5*time.Minute {
		t.Fatalf("idle_ttl not preserved: %v", got.Session.IdleTTL)
	}
	if !got.MCP.Enabled || len(got.MCP.Tools) != 2 {
		t.Fatalf("mcp not preserved: %+v", got.MCP)
	}
	if len(got.ActionsAllowed) != 2 {
		t.Fatalf("actions not preserved: %v", got.ActionsAllowed)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "strict-review" {
		t.Fatalf("skills not preserved: %v", got.Skills)
	}

	// List
	list, _ := s.ListProfiles(ctx)
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	// Update
	p.Capabilities = []string{"go", "backend", "api"}
	p.Session.MaxTurns = 20
	p.Skills = []string{"strict-review", "writing-wave-plans"}
	if err := s.UpdateProfile(ctx, p); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got, _ = s.GetProfile(ctx, "worker-1")
	if len(got.Capabilities) != 3 {
		t.Fatalf("expected 3 caps, got %d", len(got.Capabilities))
	}
	if got.Session.MaxTurns != 20 {
		t.Fatalf("expected max_turns=20, got %d", got.Session.MaxTurns)
	}
	if len(got.Skills) != 2 {
		t.Fatalf("expected 2 skills after update, got %d", len(got.Skills))
	}

	// Update nonexistent
	if err := s.UpdateProfile(ctx, &core.AgentProfile{ID: "nope", DriverID: "claude-acp", Role: core.RoleWorker}); !errors.Is(err, core.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}

	// Delete
	if err := s.DeleteProfile(ctx, "worker-1"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if _, err := s.GetProfile(ctx, "worker-1"); !errors.Is(err, core.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound after delete")
	}

	// Delete nonexistent
	if err := s.DeleteProfile(ctx, "nope"); !errors.Is(err, core.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestResolveForAction(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	s.CreateDriver(ctx, testDriver("claude-acp"))
	s.CreateDriver(ctx, testDriver("codex-acp"))

	s.CreateProfile(ctx, testProfile("be-worker", "claude-acp", core.RoleWorker, "go", "backend"))
	s.CreateProfile(ctx, testProfile("fe-worker", "codex-acp", core.RoleWorker, "react", "frontend"))
	s.CreateProfile(ctx, testProfile("reviewer", "claude-acp", core.RoleGate, "review"))

	tests := []struct {
		name    string
		step    *core.Action
		wantID  string
		wantErr bool
	}{
		{
			name:   "match role + capability",
			step:   &core.Action{AgentRole: "worker", RequiredCapabilities: []string{"go"}},
			wantID: "be-worker",
		},
		{
			name:   "match capability only (no role filter)",
			step:   &core.Action{RequiredCapabilities: []string{"react"}},
			wantID: "fe-worker",
		},
		{
			name:   "match role only",
			step:   &core.Action{AgentRole: "gate"},
			wantID: "reviewer",
		},
		{
			name:    "no match",
			step:    &core.Action{AgentRole: "worker", RequiredCapabilities: []string{"k8s"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, d, err := s.ResolveForAction(ctx, tt.step)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveForAction: %v", err)
			}
			if p.ID != tt.wantID {
				t.Fatalf("expected profile %q, got %q", tt.wantID, p.ID)
			}
			if d == nil {
				t.Fatal("expected driver, got nil")
			}
		})
	}
}

func TestResolveByID(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	s.CreateDriver(ctx, testDriver("claude-acp"))
	s.CreateProfile(ctx, testProfile("worker", "claude-acp", core.RoleWorker, "go"))

	p, d, err := s.ResolveByID(ctx, "worker")
	if err != nil {
		t.Fatalf("ResolveByID: %v", err)
	}
	if p.ID != "worker" {
		t.Fatalf("expected worker, got %s", p.ID)
	}
	if d.ID != "claude-acp" {
		t.Fatalf("expected claude-acp driver, got %s", d.ID)
	}

	// Nonexistent
	_, _, err = s.ResolveByID(ctx, "nope")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestUpsertDriver(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	d := testDriver("claude-acp")
	// First upsert = insert
	if err := s.UpsertDriver(ctx, d); err != nil {
		t.Fatalf("UpsertDriver (insert): %v", err)
	}
	got, _ := s.GetDriver(ctx, "claude-acp")
	if got.LaunchCommand != "npx" {
		t.Fatalf("expected npx, got %s", got.LaunchCommand)
	}

	// Second upsert = update
	d.LaunchCommand = "node"
	if err := s.UpsertDriver(ctx, d); err != nil {
		t.Fatalf("UpsertDriver (update): %v", err)
	}
	got, _ = s.GetDriver(ctx, "claude-acp")
	if got.LaunchCommand != "node" {
		t.Fatalf("expected node after upsert, got %s", got.LaunchCommand)
	}
}

func TestUpsertProfile(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	s.CreateDriver(ctx, testDriver("claude-acp"))

	p := testProfile("worker", "claude-acp", core.RoleWorker, "go")
	// First upsert = insert
	if err := s.UpsertProfile(ctx, p); err != nil {
		t.Fatalf("UpsertProfile (insert): %v", err)
	}
	got, _ := s.GetProfile(ctx, "worker")
	if len(got.Capabilities) != 1 {
		t.Fatalf("expected 1 cap, got %d", len(got.Capabilities))
	}
	if len(got.Skills) != 1 || got.Skills[0] != "strict-review" {
		t.Fatalf("expected skills preserved, got %v", got.Skills)
	}

	// Second upsert = update
	p.Capabilities = []string{"go", "backend"}
	p.Skills = []string{"writing-wave-plans"}
	if err := s.UpsertProfile(ctx, p); err != nil {
		t.Fatalf("UpsertProfile (update): %v", err)
	}
	got, _ = s.GetProfile(ctx, "worker")
	if len(got.Capabilities) != 2 {
		t.Fatalf("expected 2 caps after upsert, got %d", len(got.Capabilities))
	}
	if len(got.Skills) != 1 || got.Skills[0] != "writing-wave-plans" {
		t.Fatalf("expected skills updated after upsert, got %v", got.Skills)
	}
}

func TestCapabilityOverflow(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	// Driver with only FSRead
	d := &core.AgentDriver{
		ID:            "readonly",
		LaunchCommand: "cat",
		CapabilitiesMax: core.DriverCapabilities{
			FSRead: true, FSWrite: false, Terminal: false,
		},
	}
	s.CreateDriver(ctx, d)

	// Profile that requires FSWrite → should fail
	p := &core.AgentProfile{
		ID:             "writer",
		DriverID:       "readonly",
		Role:           core.RoleWorker,
		ActionsAllowed: []core.AgentAction{core.AgentActionFSWrite}, // requires FSWrite
	}
	if err := s.CreateProfile(ctx, p); !errors.Is(err, core.ErrCapabilityOverflow) {
		t.Fatalf("expected ErrCapabilityOverflow, got %v", err)
	}
}

func TestProfileCRUDRejectsInvalidSkillReference(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	s.CreateDriver(ctx, testDriver("claude-acp"))

	p := testProfile("broken-worker", "claude-acp", core.RoleWorker, "go")
	p.Skills = []string{"missing-skill"}
	if err := s.CreateProfile(ctx, p); !errors.Is(err, core.ErrInvalidSkills) {
		t.Fatalf("expected ErrInvalidSkills, got %v", err)
	}
}
