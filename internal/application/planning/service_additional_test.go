package planning

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

type planningLLMStub struct {
	raw    json.RawMessage
	err    error
	prompt string
	tools  []ToolDef
	calls  int
}

func (s *planningLLMStub) Complete(_ context.Context, prompt string, tools []ToolDef) (json.RawMessage, error) {
	s.calls++
	s.prompt = prompt
	s.tools = tools
	if s.err != nil {
		return nil, s.err
	}
	return s.raw, nil
}

type planningRegistryStub struct {
	profiles []*core.AgentProfile
	err      error
}

func (s *planningRegistryStub) GetProfile(context.Context, string) (*core.AgentProfile, error) {
	return nil, core.ErrProfileNotFound
}

func (s *planningRegistryStub) ListProfiles(context.Context) ([]*core.AgentProfile, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.profiles, nil
}

func (s *planningRegistryStub) CreateProfile(context.Context, *core.AgentProfile) error { return nil }
func (s *planningRegistryStub) UpdateProfile(context.Context, *core.AgentProfile) error { return nil }
func (s *planningRegistryStub) DeleteProfile(context.Context, string) error             { return nil }
func (s *planningRegistryStub) ResolveForAction(context.Context, *core.Action) (*core.AgentProfile, error) {
	return nil, core.ErrNoMatchingAgent
}
func (s *planningRegistryStub) ResolveByID(context.Context, string) (*core.AgentProfile, error) {
	return nil, core.ErrProfileNotFound
}

type planningStoreWrapper struct {
	core.Store
	createActionErr error
}

func (s *planningStoreWrapper) CreateAction(ctx context.Context, a *core.Action) (int64, error) {
	if s.createActionErr != nil {
		return 0, s.createActionErr
	}
	return s.Store.CreateAction(ctx, a)
}

func newPlanningStore(t *testing.T) core.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "planning.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewServiceAndListProfiles(t *testing.T) {
	llm := &planningLLMStub{}
	svc := NewService(llm, nil)
	if svc.llm != llm || svc.registry != nil {
		t.Fatalf("NewService() = %#v", svc)
	}

	profiles, err := svc.listProfiles(context.Background())
	if err != nil {
		t.Fatalf("listProfiles(nil registry) error = %v", err)
	}
	if profiles != nil {
		t.Fatalf("listProfiles(nil registry) = %#v, want nil", profiles)
	}

	registry := &planningRegistryStub{err: errors.New("registry failed")}
	svc = NewService(llm, registry)
	if _, err := svc.listProfiles(context.Background()); err == nil || err.Error() != "registry failed" {
		t.Fatalf("listProfiles(error) = %v, want registry failed", err)
	}
}

func TestServiceGenerate(t *testing.T) {
	ctx := context.Background()

	t.Run("nil llm", func(t *testing.T) {
		svc := NewService(nil, nil)
		if _, err := svc.Generate(ctx, GenerateInput{Description: "build api"}); err == nil || !strings.Contains(err.Error(), "llm completer is nil") {
			t.Fatalf("Generate(nil llm) error = %v", err)
		}
	})

	t.Run("list profiles error", func(t *testing.T) {
		svc := NewService(&planningLLMStub{}, &planningRegistryStub{err: errors.New("registry failed")})
		if _, err := svc.Generate(ctx, GenerateInput{Description: "build api"}); err == nil || !strings.Contains(err.Error(), "list profiles") {
			t.Fatalf("Generate(list profiles error) = %v", err)
		}
	})

	t.Run("llm error", func(t *testing.T) {
		svc := NewService(&planningLLMStub{err: errors.New("upstream failed")}, nil)
		if _, err := svc.Generate(ctx, GenerateInput{Description: "build api"}); err == nil || !strings.Contains(err.Error(), "llm call failed") {
			t.Fatalf("Generate(llm error) = %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		svc := NewService(&planningLLMStub{raw: json.RawMessage(`not-json`)}, nil)
		if _, err := svc.Generate(ctx, GenerateInput{Description: "build api"}); err == nil || !strings.Contains(err.Error(), "parse llm output") {
			t.Fatalf("Generate(invalid json) = %v", err)
		}
	})

	t.Run("zero steps", func(t *testing.T) {
		svc := NewService(&planningLLMStub{raw: json.RawMessage(`{"steps":[]}`)}, nil)
		if _, err := svc.Generate(ctx, GenerateInput{Description: "build api"}); err == nil || !strings.Contains(err.Error(), "zero steps") {
			t.Fatalf("Generate(zero steps) = %v", err)
		}
	})

	t.Run("invalid dag", func(t *testing.T) {
		svc := NewService(&planningLLMStub{raw: json.RawMessage(`{"steps":[{"name":"build","type":"weird"}]}`)}, nil)
		if _, err := svc.Generate(ctx, GenerateInput{Description: "build api"}); err == nil || !strings.Contains(err.Error(), "invalid type") {
			t.Fatalf("Generate(invalid dag) = %v", err)
		}
	})

	t.Run("capability fit error", func(t *testing.T) {
		svc := NewService(
			&planningLLMStub{raw: json.RawMessage(`{"steps":[{"name":"frontend","type":"exec","agent_role":"worker","required_capabilities":["react"]}]}`)},
			&planningRegistryStub{profiles: []*core.AgentProfile{{ID: "worker", Role: core.RoleWorker, Capabilities: []string{"go"}}}},
		)
		if _, err := svc.Generate(ctx, GenerateInput{Description: "build ui"}); err == nil || !strings.Contains(err.Error(), "no matching agent profile") {
			t.Fatalf("Generate(capability fit error) = %v", err)
		}
	})

	t.Run("success with profiles", func(t *testing.T) {
		llm := &planningLLMStub{raw: json.RawMessage(`{"steps":[{"name":"implement-api","type":"exec","agent_role":"worker","required_capabilities":["backend"],"description":"ship api","acceptance_criteria":["tests pass"]}]}`)}
		svc := NewService(llm, &planningRegistryStub{profiles: []*core.AgentProfile{
			{ID: "backend-worker", Role: core.RoleWorker, Capabilities: []string{"backend"}},
		}})

		dag, err := svc.Generate(ctx, GenerateInput{Description: "build api"})
		if err != nil {
			t.Fatalf("Generate(success) error = %v", err)
		}
		if len(dag.Steps) != 1 || dag.Steps[0].Name != "implement-api" {
			t.Fatalf("Generate(success) = %#v", dag)
		}
		if llm.calls != 1 {
			t.Fatalf("llm calls = %d, want 1", llm.calls)
		}
		if !strings.Contains(llm.prompt, "Available agent profiles") || !strings.Contains(llm.prompt, "backend-worker") {
			t.Fatalf("prompt = %q", llm.prompt)
		}
		if len(llm.tools) != 1 {
			t.Fatalf("tools = %#v", llm.tools)
		}
	})

	t.Run("uses plan skill prompt when available", func(t *testing.T) {
		skillsRoot := filepath.Join(t.TempDir(), "skills")
		skillDir := filepath.Join(skillsRoot, "plan-actions")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir skill dir: %v", err)
		}
		const skillBody = `---
name: plan-actions
description: custom planning guidance
---

# Custom Planning Guidance

Always identify the primary deliverable first.
`
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
			t.Fatalf("write skill: %v", err)
		}

		llm := &planningLLMStub{raw: json.RawMessage(`{"steps":[{"name":"implement-api","type":"exec","agent_role":"worker","required_capabilities":["backend"],"description":"ship api","acceptance_criteria":["tests pass"]}]}`)}
		svc := NewService(
			llm,
			&planningRegistryStub{profiles: []*core.AgentProfile{{ID: "backend-worker", Role: core.RoleWorker, Capabilities: []string{"backend"}}}},
			WithPlanningSkillsRoot(skillsRoot),
		)

		if _, err := svc.Generate(ctx, GenerateInput{Description: "build api"}); err != nil {
			t.Fatalf("Generate(skill prompt) error = %v", err)
		}
		if !strings.Contains(llm.prompt, "Custom Planning Guidance") {
			t.Fatalf("prompt should include plan skill guidance, got %q", llm.prompt)
		}
		if strings.Contains(llm.prompt, "Use this workflow to convert a task description") {
			t.Fatalf("prompt should prefer skill guidance over fallback, got %q", llm.prompt)
		}
	})
}

func TestServiceMaterializeAdditionalBranches(t *testing.T) {
	ctx := context.Background()
	store := newPlanningStore(t)
	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{Title: "planning", Status: core.WorkItemOpen})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	svc := NewService(nil, nil)

	if _, err := svc.Materialize(ctx, store, workItemID, nil); err == nil || !strings.Contains(err.Error(), "generated dag is nil") {
		t.Fatalf("Materialize(nil dag) = %v", err)
	}

	if _, err := svc.Materialize(ctx, store, workItemID, &GeneratedDAG{Steps: []GeneratedStep{{Name: "", Type: "exec"}}}); err == nil || !strings.Contains(err.Error(), "empty name") {
		t.Fatalf("Materialize(invalid dag) = %v", err)
	}

	steps, err := svc.Materialize(ctx, store, workItemID, &GeneratedDAG{
		Steps: []GeneratedStep{
			{Name: "composite-step", Type: "composite", AgentRole: "worker", Description: "delegated workflow"},
		},
	})
	if err != nil {
		t.Fatalf("Materialize(composite type) error = %v", err)
	}
	if len(steps) != 1 || steps[0].Type != core.ActionType("composite") {
		t.Fatalf("Materialize(composite type) steps = %#v", steps)
	}

	failingStore := &planningStoreWrapper{
		Store:           store,
		createActionErr: errors.New("create failed"),
	}
	if _, err := svc.Materialize(ctx, failingStore, workItemID, &GeneratedDAG{
		Steps: []GeneratedStep{{Name: "will-fail", Type: "exec"}},
	}); err == nil || !strings.Contains(err.Error(), `create step "will-fail"`) {
		t.Fatalf("Materialize(create failure) = %v", err)
	}
}
