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

func testDriverConfig() core.DriverConfig {
	return core.DriverConfig{
		LaunchCommand: "npx",
		LaunchArgs:    []string{"-y", "@test/claude-acp"},
		Env:           map[string]string{"KEY": "val"},
		CapabilitiesMax: core.DriverCapabilities{
			FSRead: true, FSWrite: true, Terminal: true,
		},
	}
}

func testProfile(id string, role core.AgentRole, caps ...string) *core.AgentProfile {
	return &core.AgentProfile{
		ID:             id,
		Name:           id,
		Driver:         testDriverConfig(),
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

func TestProfileCRUD(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	p := testProfile("worker-1", core.RoleWorker, "go", "backend")

	// Create
	if err := s.CreateProfile(ctx, p); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	// Duplicate
	if err := s.CreateProfile(ctx, p); !errors.Is(err, core.ErrDuplicateProfile) {
		t.Fatalf("expected ErrDuplicateProfile, got %v", err)
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
	// Verify embedded driver config round-trips.
	if got.Driver.LaunchCommand != "npx" {
		t.Fatalf("expected driver launch_command npx, got %s", got.Driver.LaunchCommand)
	}
	if len(got.Driver.LaunchArgs) != 2 {
		t.Fatalf("expected 2 driver args, got %d", len(got.Driver.LaunchArgs))
	}
	if got.Driver.Env["KEY"] != "val" {
		t.Fatalf("expected driver env KEY=val, got %v", got.Driver.Env)
	}
	if !got.Driver.CapabilitiesMax.FSRead || !got.Driver.CapabilitiesMax.FSWrite || !got.Driver.CapabilitiesMax.Terminal {
		t.Fatalf("expected all driver caps true, got %+v", got.Driver.CapabilitiesMax)
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
	if err := s.UpdateProfile(ctx, &core.AgentProfile{ID: "nope", Role: core.RoleWorker}); !errors.Is(err, core.ErrProfileNotFound) {
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

	s.CreateProfile(ctx, testProfile("be-worker", core.RoleWorker, "go", "backend"))

	feProfile := testProfile("fe-worker", core.RoleWorker, "react", "frontend")
	feProfile.Driver.LaunchCommand = "codex"
	s.CreateProfile(ctx, feProfile)

	s.CreateProfile(ctx, testProfile("reviewer", core.RoleGate, "review"))

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
			p, err := s.ResolveForAction(ctx, tt.step)
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
		})
	}
}

func TestResolveByID(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	s.CreateProfile(ctx, testProfile("worker", core.RoleWorker, "go"))

	p, err := s.ResolveByID(ctx, "worker")
	if err != nil {
		t.Fatalf("ResolveByID: %v", err)
	}
	if p.ID != "worker" {
		t.Fatalf("expected worker, got %s", p.ID)
	}

	// Nonexistent
	_, err = s.ResolveByID(ctx, "nope")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestUpsertProfile(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	p := testProfile("worker", core.RoleWorker, "go")
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

	// Profile with driver that only has FSRead
	p := &core.AgentProfile{
		ID:   "writer",
		Role: core.RoleWorker,
		Driver: core.DriverConfig{
			LaunchCommand: "cat",
			CapabilitiesMax: core.DriverCapabilities{
				FSRead: true, FSWrite: false, Terminal: false,
			},
		},
		ActionsAllowed: []core.AgentAction{core.AgentActionFSWrite}, // requires FSWrite
	}
	if err := s.CreateProfile(ctx, p); !errors.Is(err, core.ErrCapabilityOverflow) {
		t.Fatalf("expected ErrCapabilityOverflow, got %v", err)
	}
}

func TestProfileCRUDRejectsInvalidSkillReference(t *testing.T) {
	s := newAgentTestStore(t)
	ctx := context.Background()

	p := testProfile("broken-worker", core.RoleWorker, "go")
	p.Skills = []string{"missing-skill"}
	if err := s.CreateProfile(ctx, p); !errors.Is(err, core.ErrInvalidSkills) {
		t.Fatalf("expected ErrInvalidSkills, got %v", err)
	}
}
