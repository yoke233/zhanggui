package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchURLContent_Success(t *testing.T) {
	ssrfCheckEnabled = false
	defer func() { ssrfCheckEnabled = true }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	body, mediaType, err := fetchURLContent(context.Background(), ts.URL+"/test.txt", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(body))
	}
	if mediaType != "text/plain" {
		t.Errorf("expected 'text/plain', got %q", mediaType)
	}
}

func TestFetchURLContent_ExceedsMaxBytes(t *testing.T) {
	ssrfCheckEnabled = false
	defer func() { ssrfCheckEnabled = true }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer ts.Close()

	_, _, err := fetchURLContent(context.Background(), ts.URL, 50)
	if err == nil {
		t.Fatal("expected error for exceeding max bytes")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected exceeds error, got: %v", err)
	}
}

func TestFetchURLContent_UnsupportedScheme(t *testing.T) {
	_, _, err := fetchURLContent(context.Background(), "ftp://example.com/file", 1<<20)
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %v", err)
	}
}

func TestFetchURLContent_PrivateIP(t *testing.T) {
	ssrfCheckEnabled = true
	_, _, err := fetchURLContent(context.Background(), "http://127.0.0.1/secret", 1<<20)
	if err == nil {
		t.Fatal("expected error for private IP")
	}
	if !strings.Contains(err.Error(), "private") {
		t.Errorf("expected private address error, got: %v", err)
	}
}

func TestFetchURLContent_NonOKStatus(t *testing.T) {
	ssrfCheckEnabled = false
	defer func() { ssrfCheckEnabled = true }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, _, err := fetchURLContent(context.Background(), ts.URL, 1<<20)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error, got: %v", err)
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		host    string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"8.8.8.8", false},
		{"::1", true},
	}
	for _, tt := range tests {
		got := isPrivateIP(tt.host)
		if got != tt.private {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tt.host, got, tt.private)
		}
	}
}
