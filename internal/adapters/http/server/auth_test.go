package httpx

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yoke233/zhanggui/internal/platform/config"
)

func TestTokenAuthMiddleware_DoesNotWarnForWebSocketQueryToken(t *testing.T) {
	registry := NewTokenRegistry(map[string]config.TokenEntry{
		"admin": {Token: "secret-token", Scopes: []string{"*"}},
	})
	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)

	handler := TokenAuthMiddleware(registry, WithAuthLogger(logger))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ws?token=secret-token", nil)
	req.Header.Set("Upgrade", "websocket")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if strings.Contains(logs.String(), "SECURITY WARNING") {
		t.Fatalf("unexpected security warning for websocket query token: %s", logs.String())
	}
}

func TestTokenAuthMiddleware_WarnsForHTTPQueryToken(t *testing.T) {
	registry := NewTokenRegistry(map[string]config.TokenEntry{
		"admin": {Token: "secret-token", Scopes: []string{"*"}},
	})
	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)

	handler := TokenAuthMiddleware(registry, WithAuthLogger(logger))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/projects?token=secret-token", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if !strings.Contains(logs.String(), "SECURITY WARNING") {
		t.Fatalf("expected security warning for http query token, got logs: %s", logs.String())
	}
}

func TestTokenRegistry_GeneratedScopedTokenSurvivesRegistryRebuild(t *testing.T) {
	tokens := map[string]config.TokenEntry{
		"admin": {Token: "persistent-admin-token", Scopes: []string{"*"}},
	}
	issuer := NewTokenRegistry(tokens)
	token, err := issuer.GenerateScopedToken("agent-action-42", []string{"action:42"}, "agent/run-7")
	if err != nil {
		t.Fatalf("GenerateScopedToken() error = %v", err)
	}

	reloaded := NewTokenRegistry(tokens)
	info, ok := reloaded.Lookup(token)
	if !ok {
		t.Fatal("Lookup() = false, want true after registry rebuild")
	}
	if info.Role != "agent-action-42" {
		t.Fatalf("Role = %q, want %q", info.Role, "agent-action-42")
	}
	if len(info.Scopes) != 1 || info.Scopes[0] != "action:42" {
		t.Fatalf("Scopes = %#v, want [action:42]", info.Scopes)
	}
	if info.Submitter != "agent/run-7" {
		t.Fatalf("Submitter = %q, want %q", info.Submitter, "agent/run-7")
	}
}

func TestTokenRegistry_RemoveTokenRevokesGeneratedScopedTokenInProcess(t *testing.T) {
	registry := NewTokenRegistry(map[string]config.TokenEntry{
		"admin": {Token: "persistent-admin-token", Scopes: []string{"*"}},
	})
	token, err := registry.GenerateScopedToken("agent-action-42", []string{"action:42"}, "agent/run-7")
	if err != nil {
		t.Fatalf("GenerateScopedToken() error = %v", err)
	}
	registry.RemoveToken(token)
	if _, ok := registry.Lookup(token); ok {
		t.Fatal("Lookup() = true, want false after RemoveToken")
	}
}
