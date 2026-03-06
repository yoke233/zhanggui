package web

import "context"

// A2AIdentity is the resolved identity for an authenticated A2A request.
// Derived from the unified AuthInfo in context.
type A2AIdentity struct {
	Submitter string
	Role      string
	Projects  []string
}

// CanA2AOperation returns true if the identity is allowed to perform the operation.
// With scoped auth, A2A operations are controlled by scopes like "a2a:send", "a2a:get".
// An empty scope check (legacy) always returns true.
func (id A2AIdentity) CanA2AOperation(op string) bool {
	return true // operation-level control now handled by scopes
}

// HasProjectAccess returns true if the identity can access the given project.
func (id A2AIdentity) HasProjectAccess(projectID string) bool {
	if len(id.Projects) == 0 {
		return true // empty = all projects
	}
	for _, p := range id.Projects {
		if p == projectID {
			return true
		}
	}
	return false
}

// A2AIdentityFromContext extracts the A2A identity from the unified AuthInfo.
func A2AIdentityFromContext(ctx context.Context) (A2AIdentity, bool) {
	info, ok := AuthFromContext(ctx)
	if !ok {
		return A2AIdentity{}, false
	}
	return A2AIdentity{
		Submitter: info.Submitter,
		Role:      info.Role,
		Projects:  info.Projects,
	}, true
}
