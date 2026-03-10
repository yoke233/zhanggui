package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpointReturns200(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
}

func TestAPIV1HealthEndpointReturns200(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /api/v1/health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
}

func TestStatsEndpointReturns401WithoutToken(t *testing.T) {
	srv := NewServer(Config{
		Token: "test-token",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/stats", nil)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/stats failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401, got %d, body=%s", resp.StatusCode, string(body))
	}
}

func TestStatsEndpointReturns200WithValidToken(t *testing.T) {
	srv := NewServer(Config{
		Token: "test-token",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/stats", nil)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/stats failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
}

func TestStatsEndpointSchema(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/stats")
	if err != nil {
		t.Fatalf("GET /api/v1/stats failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}

	var payload struct {
		TotalRuns   int     `json:"total_Runs"`
		ActiveRuns  int     `json:"active_Runs"`
		SuccessRate float64 `json:"success_rate"`
		AvgDuration string  `json:"avg_duration"`
		TokensUsed  struct {
			Claude int `json:"claude"`
			Codex  int `json:"codex"`
		} `json:"tokens_used"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode stats payload: %v", err)
	}
}

func TestRootPathServesFrontendPage(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected html content type, got %q", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), `<div id="root"></div>`) {
		t.Fatalf("expected SPA root element in body, got %s", string(body))
	}
}

func TestUnknownSPARouteFallsBackToIndex(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/projects/demo/board")
	if err != nil {
		t.Fatalf("GET unknown SPA route failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected html content type, got %q", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), `<div id="root"></div>`) {
		t.Fatalf("expected SPA root element in body, got %s", string(body))
	}
}

func TestUnknownAPIRouteReturns404WithoutSPAFallback(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/unknown")
	if err != nil {
		t.Fatalf("GET unknown api route failed: %v", err)
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
		t.Fatalf("api 404 should not fallback to SPA index, body=%s", string(body))
	}
}

func TestUnknownAPIRouteWithUppercasePrefixReturns404WithoutSPAFallback(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/API/v1/unknown")
	if err != nil {
		t.Fatalf("GET /API/v1/unknown failed: %v", err)
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
		t.Fatalf("/API/v1/unknown 404 should not fallback to SPA index, body=%s", string(body))
	}
}

func TestMixedCaseAPIBasePathReturns404WithoutSPAFallback(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/Api")
	if err != nil {
		t.Fatalf("GET /Api failed: %v", err)
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
		t.Fatalf("/Api 404 should not fallback to SPA index, body=%s", string(body))
	}
}

func TestAPIBasePathReturns404WithoutSPAFallback(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api")
	if err != nil {
		t.Fatalf("GET /api failed: %v", err)
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
		t.Fatalf("/api 404 should not fallback to SPA index, body=%s", string(body))
	}
}

func TestUnknownAPIRouteWithDoubleSlashReturns404WithoutSPAFallback(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "//api/v1/unknown")
	if err != nil {
		t.Fatalf("GET //api/v1/unknown failed: %v", err)
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
		t.Fatalf("//api/v1/unknown 404 should not fallback to SPA index, body=%s", string(body))
	}
}

func TestUnknownAPIRouteWithCleanedTraversalReturns404WithoutSPAFallback(t *testing.T) {
	srv := NewServer(Config{})
	req := httptest.NewRequest(http.MethodGet, "/x/../api/v1/unknown", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	resp := rec.Result()
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
		t.Fatalf("/x/../api/v1/unknown 404 should not fallback to SPA index, body=%s", string(body))
	}
}

func TestAPIRouteStillReturnsJSONWhenFrontendEnabled(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/stats")
	if err != nil {
		t.Fatalf("GET /api/v1/stats failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected JSON content type, got %q", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if strings.Contains(string(body), `<div id="root"></div>`) {
		t.Fatalf("api response should not be SPA html, body=%s", string(body))
	}
}

func TestStaticAssetPathServesAssetFile(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	indexResp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer indexResp.Body.Close()

	indexBody, err := io.ReadAll(indexResp.Body)
	if err != nil {
		t.Fatalf("read index body: %v", err)
	}

	assetPath := firstAssetPath(string(indexBody))
	if assetPath == "" {
		t.Fatalf("index page does not contain a /assets/ path, body=%s", string(indexBody))
	}

	assetResp, err := http.Get(ts.URL + assetPath)
	if err != nil {
		t.Fatalf("GET %s failed: %v", assetPath, err)
	}
	defer assetResp.Body.Close()

	if assetResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(assetResp.Body)
		t.Fatalf("expected 200 for asset %s, got %d, body=%s", assetPath, assetResp.StatusCode, string(body))
	}
	if got := assetResp.Header.Get("Content-Type"); strings.Contains(got, "text/html") {
		t.Fatalf("expected non-html content type for static asset, got %q", got)
	}
}

func TestMissingAssetPathReturns404WithoutSPAFallback(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/assets/not-found.js")
	if err != nil {
		t.Fatalf("GET missing asset failed: %v", err)
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
		t.Fatalf("missing asset should not fallback to SPA index, body=%s", string(body))
	}
}

func TestSPARouteWithDotFallsBackToIndex(t *testing.T) {
	srv := NewServer(Config{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/releases/v1.2")
	if err != nil {
		t.Fatalf("GET /releases/v1.2 failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected html content type, got %q", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), `<div id="root"></div>`) {
		t.Fatalf("expected SPA root element in body, got %s", string(body))
	}
}

func firstAssetPath(body string) string {
	const prefix = "/assets/"
	idx := strings.Index(body, prefix)
	if idx == -1 {
		return ""
	}

	rest := body[idx:]
	end := len(rest)
	for i, ch := range rest {
		switch ch {
		case '"', '\'', '<', '>', ' ', '\n', '\r', '\t':
			end = i
			goto done
		}
	}
done:
	if end <= 0 {
		return ""
	}
	return rest[:end]
}
