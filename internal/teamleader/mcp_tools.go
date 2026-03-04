package teamleader

import (
	"strings"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
)

const (
	internalMCPServerCommand = "internal"
	mcpToolEnvKey            = "AI_WORKFLOW_MCP_TOOL"
)

var supportedMCPQueryTools = map[string]struct{}{
	"query_issues":        {},
	"query_issue_detail":  {},
	"query_Runs":          {},
	"query_Run_logs":      {},
	"query_project_stats": {},
}

// MCPToolsFromRoleConfig maps role.mcp.tools to NewSessionRequest.MCPServers.
func MCPToolsFromRoleConfig(role acpclient.RoleProfile) []acpproto.McpServer {
	if len(role.MCPTools) == 0 {
		return nil
	}

	servers := make([]acpproto.McpServer, 0, len(role.MCPTools))
	seen := make(map[string]struct{}, len(role.MCPTools))

	for _, rawTool := range role.MCPTools {
		tool := strings.TrimSpace(rawTool)
		if tool == "" {
			continue
		}
		if _, ok := supportedMCPQueryTools[tool]; !ok {
			continue
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}

		servers = append(servers, acpproto.McpServer{
			Stdio: &acpproto.McpServerStdio{
				Name:    "workflow-query-" + tool,
				Command: internalMCPServerCommand,
				Args:    []string{},
				Env: []acpproto.EnvVariable{
					{Name: mcpToolEnvKey, Value: tool},
				},
			},
		})
	}

	if len(servers) == 0 {
		return nil
	}
	return servers
}
