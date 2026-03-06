package web

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/yoke233/ai-workflow/internal/config"
)

// Scope constants for permission checks.
const (
	ScopeAll          = "*"
	ScopeIssuesRead   = "issues:read"
	ScopeIssuesWrite  = "issues:write"
	ScopeRunsRead     = "runs:read"
	ScopeRunsWrite    = "runs:write"
	ScopeProjectsRead = "projects:read"
	ScopeProjectsWrite = "projects:write"
	ScopeChatRead     = "chat:read"
	ScopeChatWrite    = "chat:write"
	ScopeAdmin        = "admin"
	ScopeMCP          = "mcp"
	ScopeA2A          = "a2a"
)

type authContextKey string

const authInfoKey authContextKey = "auth_info"

// AuthInfo carries the authenticated identity through request context.
type AuthInfo struct {
	Role      string   // role name from secrets (e.g. "admin", "viewer")
	Scopes    []string // granted scopes
	Submitter string   // identity for A2A/audit (optional)
	Projects  []string // project whitelist; empty = all projects (optional)
}

// HasScope returns true if the auth info grants the required scope.
func (a AuthInfo) HasScope(required string) bool {
	return scopeMatches(a.Scopes, required)
}

// HasProjectAccess returns true if the auth info grants access to the given project.
func (a AuthInfo) HasProjectAccess(projectID string) bool {
	if len(a.Projects) == 0 {
		return true
	}
	for _, p := range a.Projects {
		if p == projectID {
			return true
		}
	}
	return false
}

// AuthFromContext extracts AuthInfo from the request context.
func AuthFromContext(ctx context.Context) (AuthInfo, bool) {
	info, ok := ctx.Value(authInfoKey).(AuthInfo)
	return info, ok
}

// TokenRegistry maps bearer tokens to their role and scopes.
type TokenRegistry struct {
	entries map[string]tokenRegistryEntry // token value → entry
}

type tokenRegistryEntry struct {
	role      string
	scopes    []string
	submitter string
	projects  []string
}

// NewTokenRegistry builds a registry from secrets token entries.
func NewTokenRegistry(tokens map[string]config.TokenEntry) *TokenRegistry {
	entries := make(map[string]tokenRegistryEntry, len(tokens))
	for role, entry := range tokens {
		tok := strings.TrimSpace(entry.Token)
		if tok == "" {
			continue
		}
		entries[tok] = tokenRegistryEntry{
			role:      role,
			scopes:    entry.Scopes,
			submitter: entry.Submitter,
			projects:  entry.Projects,
		}
	}
	return &TokenRegistry{entries: entries}
}

// Lookup finds a token in the registry using constant-time comparison.
func (r *TokenRegistry) Lookup(token string) (AuthInfo, bool) {
	if r == nil || len(r.entries) == 0 {
		return AuthInfo{}, false
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return AuthInfo{}, false
	}
	for registered, entry := range r.entries {
		if subtle.ConstantTimeCompare([]byte(trimmed), []byte(registered)) == 1 {
			return AuthInfo{
				Role:      entry.role,
				Scopes:    entry.scopes,
				Submitter: entry.submitter,
				Projects:  entry.projects,
			}, true
		}
	}
	return AuthInfo{}, false
}

// IsEmpty returns true when no tokens are registered.
func (r *TokenRegistry) IsEmpty() bool {
	return r == nil || len(r.entries) == 0
}

// TokenAuthMiddleware validates the bearer token and injects AuthInfo into context.
// Returns 401 if the token is missing or invalid.
func TokenAuthMiddleware(registry *TokenRegistry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractRequestToken(r)
			if token == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			info, ok := registry.Lookup(token)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			ctx := context.WithValue(r.Context(), authInfoKey, info)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireScope returns middleware that checks if the authenticated user
// has the required scope. Returns 403 if the scope is not granted.
// If no AuthInfo is present (auth not configured), the request passes through.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info, ok := AuthFromContext(r.Context())
			if !ok {
				// No auth middleware configured — pass through.
				next.ServeHTTP(w, r)
				return
			}
			if !info.HasScope(scope) {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error":          "forbidden",
					"required_scope": scope,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractRequestToken extracts the bearer token from Authorization header
// or ?token= query parameter.
func extractRequestToken(r *http.Request) string {
	// Try query param first (used by WebSocket and MCP clients).
	if tok := strings.TrimSpace(r.URL.Query().Get("token")); tok != "" {
		return tok
	}
	// Then Authorization header.
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(auth[len(prefix):])
}

// scopeMatches checks if any of the user's scopes grant the required scope.
//
// Matching rules:
//   - "*" matches everything
//   - "issues:*" matches "issues:read", "issues:write", etc.
//   - Exact match: "issues:read" matches "issues:read"
func scopeMatches(userScopes []string, required string) bool {
	for _, s := range userScopes {
		if s == "*" {
			return true
		}
		if s == required {
			return true
		}
		if strings.HasSuffix(s, ":*") {
			prefix := strings.TrimSuffix(s, "*")
			if strings.HasPrefix(required, prefix) {
				return true
			}
		}
	}
	return false
}
