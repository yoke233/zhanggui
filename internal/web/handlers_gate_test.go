package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

type stubGateResolver struct {
	issueID  string
	gateName string
	action   string
	reason   string
	result   *core.Issue
	err      error
}

func (s *stubGateResolver) ResolveGate(_ context.Context, issueID, gateName, action, reason string) (*core.Issue, error) {
	s.issueID = issueID
	s.gateName = gateName
	s.action = action
	s.reason = reason
	return s.result, s.err
}

func withRouteParams(req *http.Request, params map[string]string) *http.Request {
	routeCtx := chi.NewRouteContext()
	for key, value := range params {
		routeCtx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func TestGateHandlersResolveGateDelegatesToResolver(t *testing.T) {
	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	resolver := &stubGateResolver{
		result: &core.Issue{ID: "issue-1", Status: core.IssueStatusQueued},
	}
	handler := &gateHandlers{store: store, resolver: resolver}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues/issue-1/gates/peer_review/resolve", strings.NewReader(`{"action":"pass","reason":"looks good"}`))
	req = withRouteParams(req, map[string]string{"id": "issue-1", "gateName": "peer_review"})
	w := httptest.NewRecorder()

	handler.resolveGate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if resolver.issueID != "issue-1" || resolver.gateName != "peer_review" || resolver.action != "pass" || resolver.reason != "looks good" {
		t.Fatalf("resolver args = %#v", resolver)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["issue_status"] != string(core.IssueStatusQueued) {
		t.Fatalf("issue_status = %q, want %q", body["issue_status"], core.IssueStatusQueued)
	}
}

func TestGateHandlersResolveGateRequiresResolver(t *testing.T) {
	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	handler := &gateHandlers{store: store}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues/issue-1/gates/peer_review/resolve", strings.NewReader(`{"action":"pass"}`))
	req = withRouteParams(req, map[string]string{"id": "issue-1", "gateName": "peer_review"})
	w := httptest.NewRecorder()

	handler.resolveGate(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body=%s", w.Code, w.Body.String())
	}
}
