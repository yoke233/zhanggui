package core

import (
	"context"
	"errors"
)

var (
	ErrProfileNotFound    = errors.New("agent profile not found")
	ErrDuplicateProfile   = errors.New("duplicate agent profile ID")
	ErrCapabilityOverflow = errors.New("profile capabilities exceed driver capabilities_max")
	ErrInvalidSkills      = errors.New("profile references invalid or missing skills")
)

// AgentRegistry manages agent profiles with CRUD and resolution.
type AgentRegistry interface {
	// Profile CRUD
	GetProfile(ctx context.Context, id string) (*AgentProfile, error)
	ListProfiles(ctx context.Context) ([]*AgentProfile, error)
	CreateProfile(ctx context.Context, p *AgentProfile) error
	UpdateProfile(ctx context.Context, p *AgentProfile) error
	DeleteProfile(ctx context.Context, id string) error

	// Resolution
	// ResolveForAction picks the best profile matching the action's AgentRole + RequiredCapabilities.
	ResolveForAction(ctx context.Context, action *Action) (*AgentProfile, error)

	// ResolveByID returns a specific profile by profile ID.
	ResolveByID(ctx context.Context, profileID string) (*AgentProfile, error)
}
