package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options controls MCP server behavior.
type Options struct {
	DevMode    bool
	SourceRoot string // go build working directory
	ServerAddr string // server HTTP address for self_restart
	ConfigDir  string // path to .ai-workflow/ directory
	DBPath     string // SQLite database path
}

// NewServer creates an MCP server exposing query and write tools.
// In dev mode, additional self-build/self-restart tools are registered.
func NewServer(deps Deps, opts Options) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "ai-workflow",
			Version: "0.2.0",
		},
		nil,
	)
	if deps.Store != nil {
		registerQueryTools(server, deps.Store)
		registerProjectTools(server, deps.Store)
		registerDashboardTools(server, deps.Store)
	}
	registerSystemInfoTool(server, opts)
	if deps.IssueManager != nil {
		registerIssueTools(server, deps.IssueManager, deps.Store)
		if deps.Store != nil {
			registerSubmitTaskTool(server, deps.IssueManager, deps.Store)
		}
	}
	if deps.RunExecutor != nil {
		registerRunTools(server, deps.RunExecutor)
	}
	if opts.DevMode {
		registerDevTools(server, opts)
	}
	return server
}
