package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
)

// registerMCPRoutes mounts the MCP Streamable HTTP endpoint on the router.
// Requires Bearer token when auth is enabled.
func registerMCPRoutes(r chi.Router, cfg Config) {
	if cfg.Store == nil {
		return
	}
	deps := mcpserver.Deps{
		Store:        cfg.Store,
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
	if cfg.AuthEnabled && cfg.BearerToken != "" {
		mcpRouter.Use(BearerAuthMiddleware(cfg.BearerToken))
	}
	mcpRouter.Handle("/*", handler)

	r.Mount("/mcp", mcpRouter)
}
