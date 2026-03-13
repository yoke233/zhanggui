package flow

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ProfileRegistry is a Resolver backed by a static list of AgentProfiles.
type ProfileRegistry struct {
	profiles []*core.AgentProfile
}

// NewProfileRegistry creates a Resolver from a set of agent profiles.
func NewProfileRegistry(profiles []*core.AgentProfile) *ProfileRegistry {
	return &ProfileRegistry{profiles: profiles}
}

// Resolve picks the first profile that matches the action's AgentRole and RequiredCapabilities.
func (r *ProfileRegistry) Resolve(_ context.Context, action *core.Action) (string, error) {
	role := core.AgentRole(action.AgentRole)
	for _, p := range r.profiles {
		if role != "" && p.Role != role {
			continue
		}
		if !p.MatchesRequirements(action.RequiredCapabilities) {
			continue
		}
		return p.ID, nil
	}
	return "", core.ErrNoMatchingAgent
}
