//go:build dev

package mcpserver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDevTools(server *mcp.Server, opts Options) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "self_build",
		Description: "Build the ai-flow binary from source (dev mode only)",
	}, selfBuildHandler(opts))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "self_restart",
		Description: "Restart the ai-flow server via admin endpoint (dev mode only)",
	}, selfRestartHandler(opts))
}

type SelfBuildInput struct {
	Flags []string `json:"flags,omitempty" jsonschema:"Extra go build flags"`
}

type SelfBuildOutput struct {
	Success    bool   `json:"success"`
	Output     string `json:"output"`
	BinarySize int64  `json:"binary_size"`
}

func selfBuildHandler(opts Options) func(context.Context, *mcp.CallToolRequest, SelfBuildInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SelfBuildInput) (*mcp.CallToolResult, any, error) {
		if opts.SourceRoot == "" {
			return errorResult("source_root not configured")
		}
		binaryPath, err := os.Executable()
		if err != nil {
			return nil, nil, fmt.Errorf("resolve executable: %w", err)
		}

		args := []string{"build", "-o", binaryPath}
		args = append(args, in.Flags...)
		args = append(args, "./cmd/ai-flow")

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = opts.SourceRoot
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf

		buildErr := cmd.Run()
		output := buf.String()

		if buildErr != nil {
			return jsonResult(SelfBuildOutput{
				Success: false,
				Output:  output,
			})
		}

		var size int64
		if info, statErr := os.Stat(binaryPath); statErr == nil {
			size = info.Size()
		}

		return jsonResult(SelfBuildOutput{
			Success:    true,
			Output:     output,
			BinarySize: size,
		})
	}
}

type SelfRestartInput struct {
	GracefulTimeoutSec int `json:"graceful_timeout_sec,omitempty" jsonschema:"Graceful shutdown timeout in seconds"`
}

func selfRestartHandler(opts Options) func(context.Context, *mcp.CallToolRequest, SelfRestartInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SelfRestartInput) (*mcp.CallToolResult, any, error) {
		if opts.ServerAddr == "" {
			return errorResult("server_addr not configured")
		}

		timeout := 10 * time.Second
		if in.GracefulTimeoutSec > 0 {
			timeout = time.Duration(in.GracefulTimeoutSec) * time.Second
		}

		url := opts.ServerAddr + "/api/v1/admin/ops/restart"
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("create restart request: %w", err)
		}

		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(httpReq)
		if err != nil {
			return jsonResult(map[string]any{
				"status":  "request_sent",
				"message": "restart signal sent, server may be restarting",
				"error":   err.Error(),
			})
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			return errorResult(fmt.Sprintf("restart endpoint returned %d", resp.StatusCode))
		}

		return jsonResult(map[string]any{
			"status":  "restarting",
			"message": "server restart initiated",
		})
	}
}
