package agentruntime

import (
	"context"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestBuildExecutorConsumerConfig_UsesFilterSubjectsForMultipleAgents(t *testing.T) {
	cfg := buildExecutorConsumerConfig("aiworkflow", 3, []string{
		"aiworkflow.invocation.submit.claude",
		"aiworkflow.invocation.submit.codex",
	})
	if cfg.FilterSubject != "" {
		t.Fatalf("FilterSubject = %q, want empty", cfg.FilterSubject)
	}
	if len(cfg.FilterSubjects) != 2 {
		t.Fatalf("FilterSubjects length = %d, want 2", len(cfg.FilterSubjects))
	}
}

func TestResolveExecutionProfile_PrefersExplicitProfileID(t *testing.T) {
	registry := &stubAgentRegistry{
		profile: &core.AgentProfile{ID: "worker-go", DriverID: "claude"},
		driver:  &core.AgentDriver{ID: "claude"},
	}
	profile, driver, err := resolveExecutionProfile(context.Background(), registry, &natsInvocationMessage{
		ProfileID: "worker-go",
		AgentID:   "claude",
	})
	if err != nil {
		t.Fatalf("resolveExecutionProfile returned error: %v", err)
	}
	if registry.lastResolvedID != "worker-go" {
		t.Fatalf("resolved ID = %q, want %q", registry.lastResolvedID, "worker-go")
	}
	if profile.ID != "worker-go" || driver.ID != "claude" {
		t.Fatalf("unexpected profile/driver: %#v %#v", profile, driver)
	}
}

type stubAgentRegistry struct {
	profile        *core.AgentProfile
	driver         *core.AgentDriver
	lastResolvedID string
}

func (s *stubAgentRegistry) GetDriver(ctx context.Context, id string) (*core.AgentDriver, error) {
	return s.driver, nil
}

func (s *stubAgentRegistry) ListDrivers(ctx context.Context) ([]*core.AgentDriver, error) {
	return []*core.AgentDriver{s.driver}, nil
}

func (s *stubAgentRegistry) CreateDriver(ctx context.Context, d *core.AgentDriver) error {
	return nil
}

func (s *stubAgentRegistry) UpdateDriver(ctx context.Context, d *core.AgentDriver) error {
	return nil
}

func (s *stubAgentRegistry) DeleteDriver(ctx context.Context, id string) error {
	return nil
}

func (s *stubAgentRegistry) GetProfile(ctx context.Context, id string) (*core.AgentProfile, error) {
	return s.profile, nil
}

func (s *stubAgentRegistry) ListProfiles(ctx context.Context) ([]*core.AgentProfile, error) {
	return []*core.AgentProfile{s.profile}, nil
}

func (s *stubAgentRegistry) CreateProfile(ctx context.Context, p *core.AgentProfile) error {
	return nil
}

func (s *stubAgentRegistry) UpdateProfile(ctx context.Context, p *core.AgentProfile) error {
	return nil
}

func (s *stubAgentRegistry) DeleteProfile(ctx context.Context, id string) error {
	return nil
}

func (s *stubAgentRegistry) ResolveForStep(ctx context.Context, step *core.Step) (*core.AgentProfile, *core.AgentDriver, error) {
	return s.profile, s.driver, nil
}

func (s *stubAgentRegistry) ResolveByID(ctx context.Context, profileID string) (*core.AgentProfile, *core.AgentDriver, error) {
	s.lastResolvedID = profileID
	if s.profile == nil || s.profile.ID != profileID {
		return nil, nil, core.ErrProfileNotFound
	}
	return s.profile, s.driver, nil
}
