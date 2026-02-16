package cmd

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zhanggui/internal/usecase/outbox"
)

type stubQualityWebhookService struct {
	called bool
	input  outbox.IngestQualityEventInput
	result outbox.IngestQualityEventResult
	err    error
}

func (s *stubQualityWebhookService) IngestQualityEvent(_ context.Context, input outbox.IngestQualityEventInput) (outbox.IngestQualityEventResult, error) {
	s.called = true
	s.input = input
	if s.err != nil {
		return outbox.IngestQualityEventResult{}, s.err
	}
	out := s.result
	if strings.TrimSpace(out.IssueRef) == "" {
		out.IssueRef = input.IssueRef
	}
	return out, nil
}

func TestQualityWebhookGitHubSignaturePass(t *testing.T) {
	t.Parallel()

	payload := `{"review":{"id":"42","state":"approved"}}`
	secret := "local-dev-secret"
	svc := &stubQualityWebhookService{
		result: outbox.IngestQualityEventResult{
			IssueRef:   "local#1",
			Duplicate:  false,
			Marker:     "review:approved",
			RoutedRole: "integrator",
		},
	}

	handler := newOutboxQualityWebhookHandler(svc, qualityWebhookAuthConfig{
		GitHubSecret: secret,
	})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github?issue_ref=local%231", strings.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", testGitHubSignature(secret, []byte(payload)))
	req.Header.Set("X-GitHub-Delivery", "delivery-42")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if !svc.called {
		t.Fatal("service called = false, want true")
	}
	if svc.input.Source != "github" {
		t.Fatalf("source = %q, want github", svc.input.Source)
	}
	if svc.input.IssueRef != "local#1" {
		t.Fatalf("issue_ref = %q, want local#1", svc.input.IssueRef)
	}
	if svc.input.ExternalEventID != "delivery-42" {
		t.Fatalf("external_event_id = %q, want delivery-42", svc.input.ExternalEventID)
	}
	if svc.input.Payload != payload {
		t.Fatalf("payload = %q, want %q", svc.input.Payload, payload)
	}
	if svc.input.Category != "" || svc.input.Result != "" {
		t.Fatalf("normal ingest should not force webhook audit fields, got category=%q result=%q", svc.input.Category, svc.input.Result)
	}

	body := decodeWebhookJSONBody(t, resp.Body.Bytes())
	if body["issue_ref"] != "local#1" {
		t.Fatalf("response issue_ref = %#v, want local#1", body["issue_ref"])
	}
	if body["marker"] != "review:approved" {
		t.Fatalf("response marker = %#v, want review:approved", body["marker"])
	}
	if body["routed_role"] != "integrator" {
		t.Fatalf("response routed_role = %#v, want integrator", body["routed_role"])
	}
}

func TestQualityWebhookGitHubSignatureFail(t *testing.T) {
	t.Parallel()

	payload := `{"review":{"id":"42","state":"approved"}}`
	secret := "local-dev-secret"
	svc := &stubQualityWebhookService{}

	handler := newOutboxQualityWebhookHandler(svc, qualityWebhookAuthConfig{
		GitHubSecret: secret,
	})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github?issue_ref=local%231", strings.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusUnauthorized, resp.Body.String())
	}
	if !svc.called {
		t.Fatal("service called = false, want true")
	}
	if svc.input.IssueRef != "local#1" {
		t.Fatalf("issue_ref = %q, want local#1", svc.input.IssueRef)
	}
	if svc.input.Source != "github" {
		t.Fatalf("source = %q, want github", svc.input.Source)
	}
	if svc.input.Category != "webhook" {
		t.Fatalf("category = %q, want webhook", svc.input.Category)
	}
	if svc.input.Result != "auth_rejected" {
		t.Fatalf("result = %q, want auth_rejected", svc.input.Result)
	}
	if !strings.Contains(svc.input.Summary, "invalid X-Hub-Signature-256") {
		t.Fatalf("summary = %q, want contains invalid X-Hub-Signature-256", svc.input.Summary)
	}
	if svc.input.Payload != payload {
		t.Fatalf("payload = %q, want %q", svc.input.Payload, payload)
	}
}

func TestQualityWebhookGitLabTokenPass(t *testing.T) {
	t.Parallel()

	payload := `{"object_kind":"pipeline","object_attributes":{"status":"success"}}`
	token := "gitlab-local-token"
	svc := &stubQualityWebhookService{
		result: outbox.IngestQualityEventResult{
			IssueRef:   "local#1",
			Duplicate:  true,
			Marker:     "qa:pass",
			RoutedRole: "integrator",
		},
	}

	handler := newOutboxQualityWebhookHandler(svc, qualityWebhookAuthConfig{
		GitLabToken: token,
	})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab?issue_ref=local%231", strings.NewReader(payload))
	req.Header.Set("X-Gitlab-Token", token)
	req.Header.Set("X-Gitlab-Event-UUID", "gl-event-1")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if !svc.called {
		t.Fatal("service called = false, want true")
	}
	if svc.input.Source != "gitlab" {
		t.Fatalf("source = %q, want gitlab", svc.input.Source)
	}
	if svc.input.ExternalEventID != "gl-event-1" {
		t.Fatalf("external_event_id = %q, want gl-event-1", svc.input.ExternalEventID)
	}
	if svc.input.Category != "" || svc.input.Result != "" {
		t.Fatalf("normal ingest should not force webhook audit fields, got category=%q result=%q", svc.input.Category, svc.input.Result)
	}
}

func TestQualityWebhookGitLabTokenFail(t *testing.T) {
	t.Parallel()

	payload := `{"object_kind":"pipeline","object_attributes":{"status":"success"}}`
	svc := &stubQualityWebhookService{}
	handler := newOutboxQualityWebhookHandler(svc, qualityWebhookAuthConfig{
		GitLabToken: "gitlab-local-token",
	})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab?issue_ref=local%231", strings.NewReader(payload))
	req.Header.Set("X-Gitlab-Token", "wrong-token")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusUnauthorized, resp.Body.String())
	}
	if !svc.called {
		t.Fatal("service called = false, want true")
	}
	if svc.input.IssueRef != "local#1" {
		t.Fatalf("issue_ref = %q, want local#1", svc.input.IssueRef)
	}
	if svc.input.Source != "gitlab" {
		t.Fatalf("source = %q, want gitlab", svc.input.Source)
	}
	if svc.input.Category != "webhook" {
		t.Fatalf("category = %q, want webhook", svc.input.Category)
	}
	if svc.input.Result != "auth_rejected" {
		t.Fatalf("result = %q, want auth_rejected", svc.input.Result)
	}
	if !strings.Contains(svc.input.Summary, "invalid X-Gitlab-Token") {
		t.Fatalf("summary = %q, want contains invalid X-Gitlab-Token", svc.input.Summary)
	}
	if svc.input.Payload != payload {
		t.Fatalf("payload = %q, want %q", svc.input.Payload, payload)
	}
}

func TestQualityWebhookMissingIssueRefReturns400(t *testing.T) {
	t.Parallel()

	svc := &stubQualityWebhookService{}
	handler := newOutboxQualityWebhookHandler(svc, qualityWebhookAuthConfig{})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(`{}`))

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if svc.called {
		t.Fatal("service called = true, want false")
	}
}

func testGitHubSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func decodeWebhookJSONBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal response json: %v; body=%q", err, string(raw))
	}
	return out
}
