package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
)

// registerMCPRoutes mounts the MCP Streamable HTTP endpoint.
// Called inside /api/v1 group which already has TokenAuthMiddleware.
func registerMCPRoutes(r chi.Router, cfg Config) {
	if cfg.Store == nil {
		return
	}
	deps := mcpserver.Deps{
		Store:        cfg.Store,
		ContextStore: cfg.ContextStore,
		IssueManager: cfg.MCPDeps.IssueManager,
		RunExecutor:  cfg.MCPDeps.RunExecutor,
	}
	server := mcpserver.NewServer(deps, mcpserver.Options{
		DevMode:    cfg.MCPServerOpts.DevMode,
		SourceRoot: cfg.MCPServerOpts.SourceRoot,
		ServerAddr: cfg.MCPServerOpts.ServerAddr,
		ConfigDir:  cfg.MCPServerOpts.ConfigDir,
		DBPath:     cfg.MCPServerOpts.DBPath,
	})
	handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		return server
	}, nil)

	mcpRouter := chi.NewRouter()
	mcpRouter.Use(RequireScope(ScopeMCP))
	mcpRouter.Handle("/*", handler)

	r.Mount("/mcp", mcpRouter)
}
