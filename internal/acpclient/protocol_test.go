package acpclient

import "testing"

func TestPromptRequestMetadataIncludesRole(t *testing.T) {
	req := PromptRequest{
		SessionID: "s1",
		Prompt:    "hello",
		Metadata: map[string]string{
			"role_id": "worker",
		},
	}
	params := req.ToParams()
	if got := params.Metadata["role_id"]; got != "worker" {
		t.Fatalf("expected role metadata to be worker, got %q", got)
	}
}

func TestNewSessionRequestToParamsPreservesMCPServers(t *testing.T) {
	req := NewSessionRequest{
		CWD: "/tmp/demo",
		MCPServers: []MCPServerConfig{
			{Name: "query", Command: "node", Args: []string{"mcp.js"}},
		},
	}

	params := req.ToParams()
	if got := len(params.MCPServers); got != 1 {
		t.Fatalf("expected one mcp server, got %d", got)
	}
	if params.MCPServers[0].Name != "query" {
		t.Fatalf("expected mcp server name query, got %q", params.MCPServers[0].Name)
	}
}
