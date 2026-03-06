package teamleader

import (
	"os"
	"strings"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
)

// MCPEnvConfig holds environment configuration for the MCP server.
type MCPEnvConfig struct {
	DBPath     string
	DevMode    bool
	SourceRoot string
	ServerAddr string // e.g. "http://127.0.0.1:8080" — when set, prefer SSE over stdio
}

// MCPToolsFromRoleConfig returns an McpServer config for the ACP session.
// If ServerAddr is set and agentSupportsSSE is true, it returns an SSE transport
// pointing to the server's MCP endpoint. Otherwise it falls back to stdio subprocess.
func MCPToolsFromRoleConfig(role acpclient.RoleProfile, mcpEnv MCPEnvConfig, agentSupportsSSE bool) []acpproto.McpServer {
	if len(role.MCPTools) == 0 {
		return nil
	}

	// SSE mode: connect to the running web server's MCP endpoint directly.
	if addr := strings.TrimSpace(mcpEnv.ServerAddr); addr != "" && agentSupportsSSE {
		url := strings.TrimRight(addr, "/") + "/api/v1/mcp"
		return []acpproto.McpServer{{
			Sse: &acpproto.McpServerSseInline{
				Name:    "ai-workflow-query",
				Type:    "sse",
				Url:     url,
				Headers: []acpproto.HttpHeader{},
			},
		}}
	}

	// Stdio fallback: spawn the current binary as mcp-serve subprocess.
	if mcpEnv.DBPath == "" {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil
	}

	env := []acpproto.EnvVariable{
		{Name: "AI_WORKFLOW_DB_PATH", Value: mcpEnv.DBPath},
	}
	if mcpEnv.DevMode {
		env = append(env,
			acpproto.EnvVariable{Name: "AI_WORKFLOW_DEV_MODE", Value: "true"},
			acpproto.EnvVariable{Name: "AI_WORKFLOW_SOURCE_ROOT", Value: mcpEnv.SourceRoot},
			acpproto.EnvVariable{Name: "AI_WORKFLOW_SERVER_ADDR", Value: mcpEnv.ServerAddr},
		)
	}

	return []acpproto.McpServer{{
		Stdio: &acpproto.McpServerStdio{
			Name:    "ai-workflow-query",
			Command: self,
			Args:    []string{"mcp-serve"},
			Env:     env,
		},
	}}
}
