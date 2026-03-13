package core

import (
	"context"
	"errors"
)

var (
	ErrDriverNotFound     = errors.New("agent driver not found")
	ErrProfileNotFound    = errors.New("agent profile not found")
	ErrDuplicateDriver    = errors.New("duplicate agent driver ID")
	ErrDuplicateProfile   = errors.New("duplicate agent profile ID")
	ErrCapabilityOverflow = errors.New("profile capabilities exceed driver capabilities_max")
	ErrDriverInUse        = errors.New("driver is referenced by one or more profiles")
	ErrInvalidSkills      = errors.New("profile references invalid or missing skills")
)

// AgentRegistry manages agent drivers and profiles with CRUD and resolution.
type AgentRegistry interface {
	// Driver CRUD
	GetDriver(ctx context.Context, id string) (*AgentDriver, error)
	ListDrivers(ctx context.Context) ([]*AgentDriver, error)
	CreateDriver(ctx context.Context, d *AgentDriver) error
	UpdateDriver(ctx context.Context, d *AgentDriver) error
	DeleteDriver(ctx context.Context, id string) error

	// Profile CRUD
	GetProfile(ctx context.Context, id string) (*AgentProfile, error)
	ListProfiles(ctx context.Context) ([]*AgentProfile, error)
	CreateProfile(ctx context.Context, p *AgentProfile) error
	UpdateProfile(ctx context.Context, p *AgentProfile) error
	DeleteProfile(ctx context.Context, id string) error

	// Resolution
	// ResolveForAction picks the best profile matching the action's AgentRole + RequiredCapabilities,
	// and returns the resolved profile together with its driver.
	ResolveForAction(ctx context.Context, action *Action) (*AgentProfile, *AgentDriver, error)

	// ResolveByID returns a specific profile and its driver by profile ID.
	ResolveByID(ctx context.Context, profileID string) (*AgentProfile, *AgentDriver, error)
}
