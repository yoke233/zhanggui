package teamleader

import (
	"os"
	"testing"

	"github.com/yoke233/ai-workflow/internal/acpclient"
)

func TestMCPToolsFromRoleConfig(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	role := acpclient.RoleProfile{MCPEnabled: true}
	env := MCPEnvConfig{DBPath: "/tmp/test.db"}

	got := MCPToolsFromRoleConfig(role, env, false)
	if len(got) != 1 {
		t.Fatalf("expected 1 mcp server, got %d", len(got))
	}
	srv := got[0]
	if srv.Stdio == nil {
		t.Fatal("expected stdio server")
	}
	if srv.Stdio.Name != "ai-workflow-query" {
		t.Fatalf("name = %q, want %q", srv.Stdio.Name, "ai-workflow-query")
	}
	if srv.Stdio.Command != self {
		t.Fatalf("command = %q, want %q", srv.Stdio.Command, self)
	}
	if len(srv.Stdio.Args) != 1 || srv.Stdio.Args[0] != "mcp-serve" {
		t.Fatalf("args = %v, want [mcp-serve]", srv.Stdio.Args)
	}
	foundDB := false
	for _, e := range srv.Stdio.Env {
		if e.Name == "AI_WORKFLOW_DB_PATH" && e.Value == "/tmp/test.db" {
			foundDB = true
		}
	}
	if !foundDB {
		t.Fatal("AI_WORKFLOW_DB_PATH not found in env")
	}
}

func TestMCPToolsFromRoleConfig_Disabled(t *testing.T) {
	role := acpclient.RoleProfile{MCPEnabled: false}
	got := MCPToolsFromRoleConfig(role, MCPEnvConfig{DBPath: "/tmp/test.db"}, false)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestMCPToolsFromRoleConfig_EmptyDBPath(t *testing.T) {
	role := acpclient.RoleProfile{MCPEnabled: true}
	got := MCPToolsFromRoleConfig(role, MCPEnvConfig{}, false)
	if got != nil {
		t.Fatalf("expected nil for empty DBPath, got %v", got)
	}
}

func TestMCPToolsFromRoleConfig_SSEMode(t *testing.T) {
	role := acpclient.RoleProfile{MCPEnabled: true}
	env := MCPEnvConfig{
		DBPath:     "/tmp/test.db",
		ServerAddr: "http://localhost:8080",
	}

	got := MCPToolsFromRoleConfig(role, env, true)
	if len(got) != 1 {
		t.Fatalf("expected 1 server, got %d", len(got))
	}
	srv := got[0]
	if srv.Sse == nil {
		t.Fatal("expected SSE config, got nil")
	}
	if srv.Sse.Name != "ai-workflow-query" {
		t.Errorf("name = %q, want %q", srv.Sse.Name, "ai-workflow-query")
	}
	if srv.Sse.Url != "http://localhost:8080/api/v1/mcp" {
		t.Errorf("url = %q, want %q", srv.Sse.Url, "http://localhost:8080/api/v1/mcp")
	}
	if srv.Stdio != nil {
		t.Error("expected no stdio config in SSE mode")
	}
}

func TestMCPToolsFromRoleConfig_SSEModeUnsupported(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	role := acpclient.RoleProfile{MCPEnabled: true}
	env := MCPEnvConfig{
		DBPath:     "/tmp/test.db",
		ServerAddr: "http://localhost:8080",
	}

	got := MCPToolsFromRoleConfig(role, env, false)
	if len(got) != 1 {
		t.Fatalf("expected 1 server, got %d", len(got))
	}
	srv := got[0]
	if srv.Stdio == nil {
		t.Fatal("expected stdio fallback when agent does not support SSE")
	}
	if srv.Stdio.Command != self {
		t.Errorf("command = %q, want %q", srv.Stdio.Command, self)
	}
	if srv.Sse != nil {
		t.Error("expected no SSE config when agent does not support SSE")
	}
}

func TestMCPToolsFromRoleConfig_DevMode(t *testing.T) {
	role := acpclient.RoleProfile{MCPEnabled: true}
	env := MCPEnvConfig{
		DBPath:     "/tmp/test.db",
		DevMode:    true,
		SourceRoot: "/src",
	}

	got := MCPToolsFromRoleConfig(role, env, false)
	if len(got) != 1 {
		t.Fatalf("expected 1 server, got %d", len(got))
	}
	if got[0].Stdio == nil {
		t.Fatal("expected stdio config for dev mode without ServerAddr")
	}
	envVars := got[0].Stdio.Env
	wantKeys := map[string]string{
		"AI_WORKFLOW_DB_PATH":     "/tmp/test.db",
		"AI_WORKFLOW_DEV_MODE":    "true",
		"AI_WORKFLOW_SOURCE_ROOT": "/src",
	}
	found := map[string]string{}
	for _, e := range envVars {
		found[e.Name] = e.Value
	}
	for k, v := range wantKeys {
		if found[k] != v {
			t.Errorf("env %s = %q, want %q", k, found[k], v)
		}
	}
}
