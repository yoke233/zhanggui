package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/mcpserver"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func cmdMCPServe() error {
	dbPath := os.Getenv("AI_WORKFLOW_DB_PATH")
	if dbPath == "" {
		return fmt.Errorf("AI_WORKFLOW_DB_PATH environment variable is required")
	}
	store, err := storesqlite.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	configDir, _ := resolveDataDir()
	// Stdio mode: only Store is available (no IssueManager/RunExecutor).
	// Write tools will not be registered.
	deps := mcpserver.Deps{Store: store}
	server := mcpserver.NewServer(deps, mcpserver.Options{
		DevMode:    os.Getenv("AI_WORKFLOW_DEV_MODE") == "true",
		SourceRoot: os.Getenv("AI_WORKFLOW_SOURCE_ROOT"),
		ServerAddr: os.Getenv("AI_WORKFLOW_SERVER_ADDR"),
		ConfigDir:  configDir,
		DBPath:     dbPath,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return server.Run(ctx, &mcp.StdioTransport{})
}
