package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// Ensure ConfigRegistry satisfies both AgentRegistry and the engine's Resolver interface.
var (
	_ core.AgentRegistry = (*ConfigRegistry)(nil)
	_ Resolver           = (*ConfigRegistry)(nil)
)

// ConfigRegistry is an in-memory AgentRegistry loaded from TOML config.
// It supports full CRUD for drivers and profiles, and resolution for steps.
type ConfigRegistry struct {
	mu       sync.RWMutex
	drivers  map[string]*core.AgentDriver
	profiles map[string]*core.AgentProfile
}

// NewConfigRegistry creates an empty ConfigRegistry.
func NewConfigRegistry() *ConfigRegistry {
	return &ConfigRegistry{
		drivers:  make(map[string]*core.AgentDriver),
		profiles: make(map[string]*core.AgentProfile),
	}
}

// LoadDrivers bulk-loads drivers, replacing any existing entries with the same ID.
func (r *ConfigRegistry) LoadDrivers(drivers []*core.AgentDriver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range drivers {
		r.drivers[d.ID] = cloneDriver(d)
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

// ---------- Driver CRUD ----------

func (r *ConfigRegistry) GetDriver(_ context.Context, id string) (*core.AgentDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.drivers[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", core.ErrDriverNotFound, id)
	}
	return cloneDriver(d), nil
}

func (r *ConfigRegistry) ListDrivers(_ context.Context) ([]*core.AgentDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*core.AgentDriver, 0, len(r.drivers))
	for _, d := range r.drivers {
		out = append(out, cloneDriver(d))
	}
	return out, nil
}

func (r *ConfigRegistry) CreateDriver(_ context.Context, d *core.AgentDriver) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.drivers[d.ID]; exists {
		return fmt.Errorf("%w: %q", core.ErrDuplicateDriver, d.ID)
	}
	r.drivers[d.ID] = cloneDriver(d)
	return nil
}

func (r *ConfigRegistry) UpdateDriver(_ context.Context, d *core.AgentDriver) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.drivers[d.ID]; !exists {
		return fmt.Errorf("%w: %q", core.ErrDriverNotFound, d.ID)
	}
	r.drivers[d.ID] = cloneDriver(d)
	return nil
}

func (r *ConfigRegistry) DeleteDriver(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.drivers[id]; !exists {
		return fmt.Errorf("%w: %q", core.ErrDriverNotFound, id)
	}
	// Prevent deletion if any profile references this driver.
	for _, p := range r.profiles {
		if p.DriverID == id {
			return fmt.Errorf("%w: driver %q is used by profile %q", core.ErrDriverInUse, id, p.ID)
		}
	}
	delete(r.drivers, id)
	return nil
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

// ResolveForStep picks the first profile matching the step's AgentRole + RequiredCapabilities.
func (r *ConfigRegistry) ResolveForStep(_ context.Context, step *core.Step) (*core.AgentProfile, *core.AgentDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role := core.AgentRole(step.AgentRole)
	for _, p := range r.profiles {
		if role != "" && p.Role != role {
			continue
		}
		if !p.MatchesRequirements(step.RequiredCapabilities) {
			continue
		}
		d, ok := r.drivers[p.DriverID]
		if !ok {
			continue // skip profiles with missing drivers
		}
		return cloneProfile(p), cloneDriver(d), nil
	}
	return nil, nil, core.ErrNoMatchingAgent
}

// ResolveByID returns a specific profile and its driver.
func (r *ConfigRegistry) ResolveByID(_ context.Context, profileID string) (*core.AgentProfile, *core.AgentDriver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.profiles[profileID]
	if !ok {
		return nil, nil, fmt.Errorf("%w: %q", core.ErrProfileNotFound, profileID)
	}
	d, ok := r.drivers[p.DriverID]
	if !ok {
		return nil, nil, fmt.Errorf("%w: profile %q references driver %q", core.ErrDriverNotFound, profileID, p.DriverID)
	}
	return cloneProfile(p), cloneDriver(d), nil
}

// Resolve implements the engine.Resolver interface for backward compatibility with FlowEngine.
func (r *ConfigRegistry) Resolve(ctx context.Context, step *core.Step) (string, error) {
	p, _, err := r.ResolveForStep(ctx, step)
	if err != nil {
		return "", err
	}
	return p.ID, nil
}

// ---------- validation ----------

// validateProfileLocked checks that the profile references a valid driver and
// its capabilities don't overflow the driver's max. Must be called with mu held.
func (r *ConfigRegistry) validateProfileLocked(p *core.AgentProfile) error {
	d, ok := r.drivers[p.DriverID]
	if !ok {
		return fmt.Errorf("%w: profile %q references driver %q", core.ErrDriverNotFound, p.ID, p.DriverID)
	}
	profileCaps := p.EffectiveCapabilities()
	if !d.CapabilitiesMax.Covers(profileCaps) {
		return fmt.Errorf("%w: profile %q exceeds driver %q", core.ErrCapabilityOverflow, p.ID, d.ID)
	}
	return nil
}

// ---------- clone helpers ----------

func cloneDriver(d *core.AgentDriver) *core.AgentDriver {
	cp := *d
	if d.LaunchArgs != nil {
		cp.LaunchArgs = append([]string(nil), d.LaunchArgs...)
	}
	if d.Env != nil {
		cp.Env = make(map[string]string, len(d.Env))
		for k, v := range d.Env {
			cp.Env[k] = v
		}
	}
	return &cp
}

func cloneProfile(p *core.AgentProfile) *core.AgentProfile {
	cp := *p
	if p.Capabilities != nil {
		cp.Capabilities = append([]string(nil), p.Capabilities...)
	}
	if p.ActionsAllowed != nil {
		cp.ActionsAllowed = append([]core.Action(nil), p.ActionsAllowed...)
	}
	if p.MCP.Tools != nil {
		cp.MCP.Tools = append([]string(nil), p.MCP.Tools...)
	}
	return &cp
}
