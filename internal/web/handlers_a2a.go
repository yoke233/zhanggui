package web

import (
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/go-chi/chi/v5"
)

func registerA2ARoutes(r chi.Router, cfg Config) {
	if !cfg.A2AEnabled {
		r.Handle("/api/v1/a2a", http.HandlerFunc(handleA2ADisabled))
		r.Handle("/.well-known/agent-card.json", http.HandlerFunc(handleA2ADisabled))
		return
	}

	r.Get("/.well-known/agent-card.json", handleA2AAgentCard(cfg))
	r.With(BearerAuthMiddleware(cfg.A2AToken)).Post("/api/v1/a2a", handleA2AJSONRPC)
}

func handleA2ADisabled(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func handleA2AAgentCard(cfg Config) http.HandlerFunc {
	version := strings.TrimSpace(cfg.A2AVersion)
	if version == "" {
		version = "0.3"
	}

	return func(w http.ResponseWriter, r *http.Request) {
		card := &a2a.AgentCard{
			Name:               "ai-workflow",
			Description:        "ai-workflow a2a endpoint",
			URL:                requestAbsoluteURL(r, "/api/v1/a2a"),
			PreferredTransport: a2a.TransportProtocolJSONRPC,
			ProtocolVersion:    version,
			Capabilities:       a2a.AgentCapabilities{Streaming: true},
			DefaultInputModes:  []string{"text/plain"},
			DefaultOutputModes: []string{"text/plain"},
			Skills:             []a2a.AgentSkill{},
			Version:            "0.1.0",
		}
		writeJSON(w, http.StatusOK, card)
	}
}

func handleA2AJSONRPC(w http.ResponseWriter, r *http.Request) {
	req, err := decodeA2ARPCRequest(r)
	if err != nil {
		writeA2ARPCError(w, nil, a2aRPCInvalidRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.JSONRPC) != "" && req.JSONRPC != a2aJSONRPCVersion {
		writeA2ARPCError(w, req.ID, a2aRPCInvalidRequest, "invalid request")
		return
	}

	method := strings.TrimSpace(req.Method)
	switch method {
	case a2aMethodMessageSend, a2aMethodMessageStream, a2aMethodTasksGet, a2aMethodTasksCancel:
		// Wave 1 只做入口与错误模型接线，具体方法在后续 wave 落地。
		writeA2ARPCError(w, req.ID, a2aRPCMethodNotFound, "method not found")
	default:
		writeA2ARPCError(w, req.ID, a2aRPCMethodNotFound, "method not found")
	}
}

func requestAbsoluteURL(r *http.Request, path string) string {
	if r == nil || strings.TrimSpace(r.Host) == "" {
		return path
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}
