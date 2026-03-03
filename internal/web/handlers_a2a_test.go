package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestA2ADisabled_RoutesReturn404WithoutSPAFallback(t *testing.T) {
	srv := NewServer(Config{A2AEnabled: false})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "jsonrpc route",
			method: http.MethodPost,
			path:   "/api/v1/a2a",
			body:   `{"jsonrpc":"2.0","id":"1","method":"message/send"}`,
		},
		{
			name:   "agent card route",
			method: http.MethodGet,
			path:   "/.well-known/agent-card.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("create request failed: %v", err)
			}
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 404, got %d, body=%s", resp.StatusCode, string(body))
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if strings.Contains(string(body), `<div id="root"></div>`) {
				t.Fatalf("expected hard 404 without SPA fallback, body=%s", string(body))
			}
		})
	}
}

func TestA2AEnabled_RequiresToken(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		A2AToken:   "a2a-token",
		A2AVersion: "0.3",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":"1","method":"message/send"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/a2a", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401, got %d, body=%s", resp.StatusCode, string(body))
	}
}

func TestA2AEnabled_MethodNotFoundReturns32601(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		A2AToken:   "a2a-token",
		A2AVersion: "0.3",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":"req-1","method":"unknown/method"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/a2a", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer a2a-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if payload["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc=2.0, got %v", payload["jsonrpc"])
	}
	if payload["id"] != "req-1" {
		t.Fatalf("expected id=req-1, got %v", payload["id"])
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %#v", payload["error"])
	}
	if code, ok := errObj["code"].(float64); !ok || int(code) != -32601 {
		t.Fatalf("expected error.code=-32601, got %#v", errObj["code"])
	}
}

func TestA2AEnabled_AgentCardReturnsJSON(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		A2AToken:   "a2a-token",
		A2AVersion: "0.3",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected JSON content type, got %q", got)
	}

	var card map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode agent card failed: %v", err)
	}

	urlRaw, _ := card["url"].(string)
	if !strings.Contains(urlRaw, "/api/v1/a2a") {
		t.Fatalf("expected card url contains /api/v1/a2a, got %q", urlRaw)
	}
	versionRaw, _ := card["protocolVersion"].(string)
	if versionRaw != "0.3" {
		t.Fatalf("expected card protocolVersion=0.3, got %q", versionRaw)
	}
}

func mustMarshalJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json failed: %v", err)
	}
	return string(bytes.TrimSpace(data))
}
