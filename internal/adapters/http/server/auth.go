package httpx

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/yoke233/ai-workflow/internal/platform/config"
)

const (
	ScopeAll   = "*"
	ScopeAdmin = "admin"
)

type authContextKey string

const authInfoKey authContextKey = "auth_info"

type AuthInfo struct {
	Role      string
	Scopes    []string
	Submitter string
	Projects  []string
}

func (a AuthInfo) HasScope(required string) bool {
	return scopeMatches(a.Scopes, required)
}

func AuthFromContext(ctx context.Context) (AuthInfo, bool) {
	info, ok := ctx.Value(authInfoKey).(AuthInfo)
	return info, ok
}

type TokenRegistry struct {
	mu      sync.RWMutex
	entries map[string]tokenRegistryEntry
}

type tokenRegistryEntry struct {
	role      string
	scopes    []string
	submitter string
	projects  []string
}

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

// GenerateScopedToken creates a random token with the given scopes, registers it,
// and returns the token string. The caller should call RemoveToken when done.
func (r *TokenRegistry) GenerateScopedToken(role string, scopes []string, submitter string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	r.mu.Lock()
	r.entries[token] = tokenRegistryEntry{
		role:      role,
		scopes:    scopes,
		submitter: submitter,
	}
	r.mu.Unlock()
	return token, nil
}

// RemoveToken removes a previously added runtime token.
func (r *TokenRegistry) RemoveToken(token string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.entries, token)
	r.mu.Unlock()
}

func (r *TokenRegistry) Lookup(token string) (AuthInfo, bool) {
	if r == nil {
		return AuthInfo{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
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

func (r *TokenRegistry) IsEmpty() bool {
	if r == nil {
		return true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries) == 0
}

func TokenAuthMiddleware(registry *TokenRegistry, opts ...AuthMiddlewareOption) func(http.Handler) http.Handler {
	cfg := authMiddlewareCfg{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, source := extractRequestTokenWithSource(r)
			if token == "" {
				if cfg.rateLimiter != nil {
					ip := extractClientIP(r)
					cfg.rateLimiter.RecordFailure(ip)
				}
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			info, ok := registry.Lookup(token)
			if !ok {
				if cfg.rateLimiter != nil {
					ip := extractClientIP(r)
					if cfg.rateLimiter.RecordFailure(ip) && cfg.logger != nil {
						cfg.logger.Printf("SECURITY: IP %s blocked after too many failed auth attempts", ip)
					}
				}
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			// Successful auth — reset rate limiter for this IP.
			if cfg.rateLimiter != nil {
				cfg.rateLimiter.Reset(extractClientIP(r))
			}
			if source == "query" && cfg.logger != nil {
				cfg.logger.Printf("SECURITY WARNING: token passed via URL query parameter from %s — use Authorization header instead", extractClientIP(r))
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authInfoKey, info)))
		})
	}
}

type authMiddlewareCfg struct {
	rateLimiter *RateLimiter
	logger      *log.Logger
}

// AuthMiddlewareOption configures TokenAuthMiddleware behavior.
type AuthMiddlewareOption func(*authMiddlewareCfg)

// WithRateLimiter attaches a rate limiter to the auth middleware.
func WithRateLimiter(rl *RateLimiter) AuthMiddlewareOption {
	return func(c *authMiddlewareCfg) { c.rateLimiter = rl }
}

// WithAuthLogger sets a logger for security-relevant auth events.
func WithAuthLogger(l *log.Logger) AuthMiddlewareOption {
	return func(c *authMiddlewareCfg) { c.logger = l }
}

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info, ok := AuthFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if !info.HasScope(scope) {
				WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_scope": scope})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractRequestToken(r *http.Request) string {
	tok, _ := extractRequestTokenWithSource(r)
	return tok
}

// extractRequestTokenWithSource returns the token and its source ("query" or "header").
func extractRequestTokenWithSource(r *http.Request) (string, string) {
	if tok := strings.TrimSpace(r.URL.Query().Get("token")); tok != "" {
		return tok, "query"
	}
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return "", ""
	}
	return strings.TrimSpace(auth[len(prefix):]), "header"
}

func scopeMatches(userScopes []string, required string) bool {
	for _, s := range userScopes {
		if s == ScopeAll || s == required {
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
