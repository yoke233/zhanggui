package agent

import (
	"context"
	"fmt"
	"sync"

	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
	skillset "github.com/yoke233/ai-workflow/internal/skills"
)

// Ensure ConfigRegistry satisfies both AgentRegistry and the flow resolver interface.
var (
	_ core.AgentRegistry = (*ConfigRegistry)(nil)
	_ flowapp.Resolver   = (*ConfigRegistry)(nil)
)

// ConfigRegistry is an in-memory AgentRegistry loaded from TOML config.
// It supports full CRUD for profiles, and resolution for actions.
type ConfigRegistry struct {
	mu       sync.RWMutex
	profiles map[string]*core.AgentProfile
}

// NewConfigRegistry creates an empty ConfigRegistry.
func NewConfigRegistry() *ConfigRegistry {
	return &ConfigRegistry{
		profiles: make(map[string]*core.AgentProfile),
	}
}

// LoadProfiles bulk-loads profiles, replacing any existing entries with the same ID.
func (r *ConfigRegistry) LoadProfiles(profiles []*core.AgentProfile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range profiles {
		r.profiles[p.ID] = cloneProfile(p)
	}
}

// ---------- Profile CRUD ----------

func (r *ConfigRegistry) GetProfile(_ context.Context, id string) (*core.AgentProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", core.ErrProfileNotFound, id)
	}
	return cloneProfile(p), nil
}

func (r *ConfigRegistry) ListProfiles(_ context.Context) ([]*core.AgentProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*core.AgentProfile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, cloneProfile(p))
	}
	return out, nil
}

func (r *ConfigRegistry) CreateProfile(_ context.Context, p *core.AgentProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.profiles[p.ID]; exists {
		return fmt.Errorf("%w: %q", core.ErrDuplicateProfile, p.ID)
	}
	if err := r.validateProfileLocked(p); err != nil {
		return err
	}
	r.profiles[p.ID] = cloneProfile(p)
	return nil
}

func (r *ConfigRegistry) UpdateProfile(_ context.Context, p *core.AgentProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.profiles[p.ID]; !exists {
		return fmt.Errorf("%w: %q", core.ErrProfileNotFound, p.ID)
	}
	if err := r.validateProfileLocked(p); err != nil {
		return err
	}
	r.profiles[p.ID] = cloneProfile(p)
	return nil
}

func (r *ConfigRegistry) DeleteProfile(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.profiles[id]; !exists {
		return fmt.Errorf("%w: %q", core.ErrProfileNotFound, id)
	}
	delete(r.profiles, id)
	return nil
}

// ---------- Resolution ----------

// ResolveForAction picks the first profile matching the action's AgentRole + RequiredCapabilities.
func (r *ConfigRegistry) ResolveForAction(_ context.Context, action *core.Action) (*core.AgentProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role := core.AgentRole(action.AgentRole)
	for _, p := range r.profiles {
		if role != "" && p.Role != role {
			continue
		}
		if !p.MatchesRequirements(action.RequiredCapabilities) {
			continue
		}
		return cloneProfile(p), nil
	}
	return nil, core.ErrNoMatchingAgent
}

// ResolveByID returns a specific profile.
func (r *ConfigRegistry) ResolveByID(_ context.Context, profileID string) (*core.AgentProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.profiles[profileID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", core.ErrProfileNotFound, profileID)
	}
	return cloneProfile(p), nil
}

// Resolve implements the flow resolver interface.
func (r *ConfigRegistry) Resolve(ctx context.Context, action *core.Action) (string, error) {
	p, err := r.ResolveForAction(ctx, action)
	if err != nil {
		return "", err
	}
	return p.ID, nil
}

// ---------- validation ----------

// validateProfileLocked checks that the profile's capabilities don't overflow
// the driver's max. Must be called with mu held.
func (r *ConfigRegistry) validateProfileLocked(p *core.AgentProfile) error {
	profileCaps := p.EffectiveCapabilities()
	if !p.Driver.CapabilitiesMax.Covers(profileCaps) {
		return fmt.Errorf("%w: profile %q exceeds driver capabilities_max", core.ErrCapabilityOverflow, p.ID)
	}
	if err := skillset.ValidateProfileSkills(p.Skills); err != nil {
		return err
	}
	return nil
}

// ---------- clone helpers ----------

func cloneProfile(p *core.AgentProfile) *core.AgentProfile {
	cp := *p
	if p.Capabilities != nil {
		cp.Capabilities = append([]string(nil), p.Capabilities...)
	}
	if p.ActionsAllowed != nil {
		cp.ActionsAllowed = append([]core.AgentAction(nil), p.ActionsAllowed...)
	}
	if p.Skills != nil {
		cp.Skills = append([]string(nil), p.Skills...)
	}
	if p.MCP.Tools != nil {
		cp.MCP.Tools = append([]string(nil), p.MCP.Tools...)
	}
	if p.Driver.LaunchArgs != nil {
		cp.Driver.LaunchArgs = append([]string(nil), p.Driver.LaunchArgs...)
	}
	if p.Driver.Env != nil {
		cp.Driver.Env = make(map[string]string, len(p.Driver.Env))
		for k, v := range p.Driver.Env {
			cp.Driver.Env[k] = v
		}
	}
	return &cp
}
