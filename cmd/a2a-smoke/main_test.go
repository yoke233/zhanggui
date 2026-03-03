package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
)

func TestResolveAgentCard_FromRPCURL(t *testing.T) {
	cfg := smokeConfig{
		RPCURL:     "https://agent.example.com/rpc",
		A2AVersion: "0.3",
	}

	card, err := resolveAgentCard(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveAgentCard() error = %v", err)
	}
	if card.URL != cfg.RPCURL {
		t.Fatalf("card.URL = %q, want %q", card.URL, cfg.RPCURL)
	}
	if card.PreferredTransport != a2a.TransportProtocolJSONRPC {
		t.Fatalf("card.PreferredTransport = %q, want %q", card.PreferredTransport, a2a.TransportProtocolJSONRPC)
	}
	if card.ProtocolVersion != cfg.A2AVersion {
		t.Fatalf("card.ProtocolVersion = %q, want %q", card.ProtocolVersion, cfg.A2AVersion)
	}
}

func TestRunSmoke_SendAndPollTask(t *testing.T) {
	t.Helper()

	var rpcCallCount int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent-card.json":
			writeJSON(w, http.StatusOK, &a2a.AgentCard{
				Name:               "test-agent",
				Description:        "test",
				URL:                srv.URL + "/rpc",
				PreferredTransport: a2a.TransportProtocolJSONRPC,
				ProtocolVersion:    "0.3",
				Capabilities:       a2a.AgentCapabilities{Streaming: true},
				DefaultInputModes:  []string{"text/plain"},
				DefaultOutputModes: []string{"text/plain"},
				Skills:             []a2a.AgentSkill{},
				Version:            "0.1.0",
			})
			return
		case "/rpc":
			id := decodeRPCID(t, r)
			rpcCallCount++

			if rpcCallCount == 1 {
				writeRPCResult(w, id, &a2a.Task{
					ID:        a2a.TaskID("task-1"),
					ContextID: "ctx-1",
					Status: a2a.TaskStatus{
						State: a2a.TaskStateWorking,
					},
				})
				return
			}

			writeRPCResult(w, id, &a2a.Task{
				ID:        a2a.TaskID("task-1"),
				ContextID: "ctx-1",
				Status: a2a.TaskStatus{
					State: a2a.TaskStateCompleted,
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	cfg := smokeConfig{
		CardBaseURL:  srv.URL,
		Prompt:       "hello a2a",
		A2AVersion:   "0.3",
		Timeout:      5 * time.Second,
		PollInterval: 10 * time.Millisecond,
		MaxPoll:      3,
		HTTPClient:   srv.Client(),
	}

	if err := runSmoke(context.Background(), cfg, &out); err != nil {
		t.Fatalf("runSmoke() error = %v", err)
	}
	if rpcCallCount < 2 {
		t.Fatalf("rpc call count = %d, want >= 2", rpcCallCount)
	}
	if !strings.Contains(out.String(), string(a2a.TaskStateCompleted)) {
		t.Fatalf("output does not contain terminal state, out=%s", out.String())
	}
}

func TestRunSmoke_InjectsA2AVersionHeader(t *testing.T) {
	t.Helper()

	var cardVersionHeader string
	var rpcVersionHeader string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent-card.json":
			cardVersionHeader = r.Header.Get(a2aVersionHeader)
			writeJSON(w, http.StatusOK, &a2a.AgentCard{
				Name:               "test-agent",
				Description:        "test",
				URL:                srv.URL + "/rpc",
				PreferredTransport: a2a.TransportProtocolJSONRPC,
				ProtocolVersion:    "0.3",
				Capabilities:       a2a.AgentCapabilities{Streaming: true},
				DefaultInputModes:  []string{"text/plain"},
				DefaultOutputModes: []string{"text/plain"},
				Skills:             []a2a.AgentSkill{},
				Version:            "0.1.0",
			})
			return
		case "/rpc":
			rpcVersionHeader = r.Header.Get(a2aVersionHeader)
			id := decodeRPCID(t, r)
			writeRPCResult(w, id, &a2a.Task{
				ID:        a2a.TaskID("task-2"),
				ContextID: "ctx-2",
				Status: a2a.TaskStatus{
					State: a2a.TaskStateCompleted,
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	cfg := smokeConfig{
		CardBaseURL: srv.URL,
		Prompt:      "hello header",
		A2AVersion:  "0.3",
		Timeout:     5 * time.Second,
		MaxPoll:     1,
		HTTPClient:  srv.Client(),
	}

	if err := runSmoke(context.Background(), cfg, &bytes.Buffer{}); err != nil {
		t.Fatalf("runSmoke() error = %v", err)
	}
	if cardVersionHeader != "0.3" {
		t.Fatalf("card request header %q = %q, want %q", a2aVersionHeader, cardVersionHeader, "0.3")
	}
	if rpcVersionHeader != "0.3" {
		t.Fatalf("rpc request header %q = %q, want %q", a2aVersionHeader, rpcVersionHeader, "0.3")
	}
}

func TestTokenHeader_InjectsBearerForCardAndRPC(t *testing.T) {
	t.Helper()

	var cardAuthHeader string
	var rpcAuthHeader string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent-card.json":
			cardAuthHeader = r.Header.Get("Authorization")
			writeJSON(w, http.StatusOK, &a2a.AgentCard{
				Name:               "test-agent",
				Description:        "test",
				URL:                srv.URL + "/rpc",
				PreferredTransport: a2a.TransportProtocolJSONRPC,
				ProtocolVersion:    "0.3",
				Capabilities:       a2a.AgentCapabilities{Streaming: true},
				DefaultInputModes:  []string{"text/plain"},
				DefaultOutputModes: []string{"text/plain"},
				Skills:             []a2a.AgentSkill{},
				Version:            "0.1.0",
			})
			return
		case "/rpc":
			rpcAuthHeader = r.Header.Get("Authorization")
			id := decodeRPCID(t, r)
			writeRPCResult(w, id, &a2a.Task{
				ID:        a2a.TaskID("task-token"),
				ContextID: "ctx-token",
				Status: a2a.TaskStatus{
					State: a2a.TaskStateCompleted,
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	cfg := smokeConfig{
		CardBaseURL: srv.URL,
		Prompt:      "hello token",
		A2AVersion:  "0.3",
		Token:       "demo-token",
		Timeout:     5 * time.Second,
		MaxPoll:     1,
		HTTPClient:  srv.Client(),
	}

	if err := runSmoke(context.Background(), cfg, &bytes.Buffer{}); err != nil {
		t.Fatalf("runSmoke() error = %v", err)
	}

	if cardAuthHeader != "Bearer demo-token" {
		t.Fatalf("card authorization header = %q, want %q", cardAuthHeader, "Bearer demo-token")
	}
	if rpcAuthHeader != "Bearer demo-token" {
		t.Fatalf("rpc authorization header = %q, want %q", rpcAuthHeader, "Bearer demo-token")
	}
}

func decodeRPCID(t *testing.T, r *http.Request) any {
	t.Helper()

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("decode rpc request: %v", err)
	}
	id, ok := req["id"]
	if !ok {
		t.Fatalf("rpc request missing id")
	}
	return id
}

func writeRPCResult(w http.ResponseWriter, id any, result any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
