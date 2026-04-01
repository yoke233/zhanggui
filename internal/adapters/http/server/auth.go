package httpx

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yoke233/zhanggui/internal/platform/config"
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
	mu            sync.RWMutex
	entries       map[string]tokenRegistryEntry
	revoked       map[string]struct{}
	signingSecret []byte
}

type tokenRegistryEntry struct {
	role      string
	scopes    []string
	submitter string
	projects  []string
}

const (
	signedScopedTokenPrefix = "rt1"
	scopedTokenTTL          = 30 * 24 * time.Hour
)

type signedScopedTokenClaims struct {
	Role      string   `json:"role"`
	Scopes    []string `json:"scopes,omitempty"`
	Submitter string   `json:"submitter,omitempty"`
	Exp       int64    `json:"exp"`
}

func NewTokenRegistry(tokens map[string]config.TokenEntry) *TokenRegistry {
	entries := make(map[string]tokenRegistryEntry, len(tokens))
	var signingSecret string
	for role, entry := range tokens {
		tok := strings.TrimSpace(entry.Token)
		if tok == "" {
			continue
		}
		if role == "admin" && signingSecret == "" {
			signingSecret = tok
		}
		entries[tok] = tokenRegistryEntry{
			role:      role,
			scopes:    entry.Scopes,
			submitter: entry.Submitter,
			projects:  entry.Projects,
		}
	}
	return &TokenRegistry{
		entries:       entries,
		revoked:       map[string]struct{}{},
		signingSecret: []byte(signingSecret),
	}
}

// GenerateScopedToken creates a random token with the given scopes, registers it,
// and returns the token string. The caller should call RemoveToken when done.
func (r *TokenRegistry) GenerateScopedToken(role string, scopes []string, submitter string) (string, error) {
	if r != nil && len(r.signingSecret) > 0 {
		claims := signedScopedTokenClaims{
			Role:      role,
			Scopes:    append([]string(nil), scopes...),
			Submitter: submitter,
			Exp:       time.Now().UTC().Add(scopedTokenTTL).Unix(),
		}
		return r.signClaims(claims)
	}
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
	if token = strings.TrimSpace(token); token != "" {
		r.revoked[token] = struct{}{}
	}
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
	if _, revoked := r.revoked[trimmed]; revoked {
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
	if info, ok := r.lookupSignedScopedToken(trimmed); ok {
		return info, true
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

func (r *TokenRegistry) signClaims(claims signedScopedTokenClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, r.signingSecret)
	_, _ = mac.Write(payload)
	sig := mac.Sum(nil)
	return signedScopedTokenPrefix + "." +
		base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(sig), nil
}

func (r *TokenRegistry) lookupSignedScopedToken(token string) (AuthInfo, bool) {
	if len(r.signingSecret) == 0 {
		return AuthInfo{}, false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != signedScopedTokenPrefix {
		return AuthInfo{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AuthInfo{}, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return AuthInfo{}, false
	}
	mac := hmac.New(sha256.New, r.signingSecret)
	_, _ = mac.Write(payload)
	expected := mac.Sum(nil)
	if subtle.ConstantTimeCompare(sig, expected) != 1 {
		return AuthInfo{}, false
	}
	var claims signedScopedTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return AuthInfo{}, false
	}
	if claims.Exp <= 0 || time.Now().UTC().Unix() > claims.Exp {
		return AuthInfo{}, false
	}
	return AuthInfo{
		Role:      claims.Role,
		Scopes:    append([]string(nil), claims.Scopes...),
		Submitter: claims.Submitter,
	}, true
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
			if source == "query" && cfg.logger != nil && !isWebSocketUpgrade(r) {
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

func isWebSocketUpgrade(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
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
