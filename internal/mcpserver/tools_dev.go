//go:build dev

package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// gate is the shared preflight gate instance for dev tools.
var gate = NewPreflightGate()

func registerDevTools(server *mcp.Server, opts Options) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "self_build_frontend",
		Description: "Build the frontend SPA (npm run build) into web/dist/. Frontend is served as static files and is not embedded into backend binary.",
	}, selfBuildFrontendHandler(opts))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "self_build",
		Description: "Build the ai-flow Go binary (backend only, no frontend embedding).",
	}, selfBuildHandler(opts))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "self_preflight",
		Description: "Run full quality gate (go vet, go build, go test, frontend typecheck+build) before restart. Must pass before self_restart is allowed.",
	}, selfPreflightHandler(opts))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "self_restart",
		Description: "Restart the ai-flow server. REQUIRES a successful self_preflight. Use force=true only for emergencies.",
	}, selfRestartHandler(opts))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "self_preflight_status",
		Description: "Check the current preflight gate status without running checks.",
	}, selfPreflightStatusHandler(opts))
}

// --- self_build_frontend ---

type SelfBuildFrontendOutput struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
}

type SelfBuildFrontendInput struct {
	SourceRoot  string `json:"source_root,omitempty" jsonschema:"Override source root; defaults to server option source_root"`
	FrontendDir string `json:"frontend_dir,omitempty" jsonschema:"Frontend workspace path for npm --prefix (relative to source_root, default: web)"`
}

func selfBuildFrontendHandler(opts Options) func(context.Context, *mcp.CallToolRequest, SelfBuildFrontendInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SelfBuildFrontendInput) (*mcp.CallToolResult, any, error) {
		sourceRoot, err := resolveDevToolSourceRoot(opts.SourceRoot, in.SourceRoot)
		if err != nil {
			return errorResult(err.Error())
		}
		frontendDir := resolveDevToolFrontendDir(in.FrontendDir)

		cmd := exec.CommandContext(ctx, "npm", "--prefix", frontendDir, "run", "build")
		cmd.Dir = sourceRoot
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf

		if err := cmd.Run(); err != nil {
			return jsonResult(SelfBuildFrontendOutput{
				Success: false,
				Output:  buf.String(),
			})
		}

		return jsonResult(SelfBuildFrontendOutput{
			Success: true,
			Output:  buf.String(),
		})
	}
}

// --- self_build ---

type SelfBuildInput struct {
	SourceRoot string `json:"source_root,omitempty" jsonschema:"Override source root; defaults to server option source_root"`
}

type SelfBuildOutput struct {
	Success    bool   `json:"success"`
	Output     string `json:"output"`
	Tags       string `json:"tags"`
	BinarySize int64  `json:"binary_size"`
}

func selfBuildHandler(opts Options) func(context.Context, *mcp.CallToolRequest, SelfBuildInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SelfBuildInput) (*mcp.CallToolResult, any, error) {
		sourceRoot, err := resolveDevToolSourceRoot(opts.SourceRoot, in.SourceRoot)
		if err != nil {
			return errorResult(err.Error())
		}
		binaryPath, err := os.Executable()
		if err != nil {
			return nil, nil, fmt.Errorf("resolve executable: %w", err)
		}

		tags := ""
		args := []string{"build", "-o", binaryPath, "./cmd/ai-flow"}

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = sourceRoot
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf

		buildErr := cmd.Run()
		output := buf.String()

		if buildErr != nil {
			return jsonResult(SelfBuildOutput{
				Success: false,
				Output:  output,
				Tags:    tags,
			})
		}

		var size int64
		if info, statErr := os.Stat(binaryPath); statErr == nil {
			size = info.Size()
		}

		return jsonResult(SelfBuildOutput{
			Success:    true,
			Output:     output,
			Tags:       tags,
			BinarySize: size,
		})
	}
}

// --- self_preflight ---

type SelfPreflightInput struct {
	SourceRoot   string `json:"source_root,omitempty" jsonschema:"Override source root; defaults to server option source_root"`
	FrontendDir  string `json:"frontend_dir,omitempty" jsonschema:"Frontend workspace path for npm --prefix (relative to source_root, default: web)"`
	SkipFrontend bool   `json:"skip_frontend,omitempty" jsonschema:"Skip frontend typecheck and build steps"`
}

type SelfPreflightOutput struct {
	Success   bool         `json:"success"`
	CommitSHA string       `json:"commit_sha"`
	Duration  string       `json:"duration"`
	Steps     []StepResult `json:"steps"`
	Message   string       `json:"message"`
}

func selfPreflightHandler(opts Options) func(context.Context, *mcp.CallToolRequest, SelfPreflightInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SelfPreflightInput) (*mcp.CallToolResult, any, error) {
		sourceRoot, err := resolveDevToolSourceRoot(opts.SourceRoot, in.SourceRoot)
		if err != nil {
			return errorResult(err.Error())
		}

		// Broadcast preflight start.
		postSystemEvent(opts.ServerAddr, "preflight_start", map[string]any{
			"message": "Preflight quality gate started",
		})

		progress := func(step StepResult, idx, total int) {
			status := "PASS"
			if !step.Success {
				status = "FAIL"
			}
			postSystemEvent(opts.ServerAddr, "preflight_step", map[string]any{
				"step":     idx + 1,
				"total":    total,
				"name":     step.Name,
				"status":   status,
				"duration": step.Duration,
				"message":  fmt.Sprintf("[%d/%d] %s: %s (%s)", idx+1, total, step.Name, status, step.Duration),
			})
		}

		result, err := gate.Run(ctx, sourceRoot, resolveDevToolFrontendDir(in.FrontendDir), in.SkipFrontend, progress)
		if err != nil {
			return errorResult(fmt.Sprintf("preflight error: %v", err))
		}

		msg := "PASS — self_restart is now allowed"
		event := "preflight_pass"
		if !result.Success {
			msg = "FAIL — self_restart is blocked until all checks pass"
			event = "preflight_fail"
		}

		postSystemEvent(opts.ServerAddr, event, map[string]any{
			"success":    result.Success,
			"commit_sha": result.CommitSHA,
			"duration":   result.Duration.String(),
			"message":    msg,
		})

		return jsonResult(SelfPreflightOutput{
			Success:   result.Success,
			CommitSHA: result.CommitSHA,
			Duration:  result.Duration.String(),
			Steps:     result.Steps,
			Message:   msg,
		})
	}
}

func resolveDevToolSourceRoot(defaultRoot string, overrideRoot string) (string, error) {
	trimmedDefault := strings.TrimSpace(defaultRoot)
	trimmedOverride := strings.TrimSpace(overrideRoot)

	if trimmedOverride == "" {
		if trimmedDefault != "" {
			return filepath.Clean(trimmedDefault), nil
		}
		detected, err := detectDevToolSourceRoot()
		if err != nil {
			return "", err
		}
		return detected, nil
	}

	if filepath.IsAbs(trimmedOverride) {
		return filepath.Clean(trimmedOverride), nil
	}

	if trimmedDefault != "" {
		return filepath.Clean(filepath.Join(trimmedDefault, trimmedOverride)), nil
	}

	absolute, err := filepath.Abs(trimmedOverride)
	if err != nil {
		return "", fmt.Errorf("resolve source_root %q: %w", trimmedOverride, err)
	}
	return filepath.Clean(absolute), nil
}

func resolveDevToolFrontendDir(frontendDir string) string {
	trimmed := strings.TrimSpace(frontendDir)
	if trimmed == "" {
		return "web"
	}
	return filepath.Clean(trimmed)
}

func detectDevToolSourceRoot() (string, error) {
	candidates := make([]string, 0, 2)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if binaryPath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(binaryPath))
	}

	for _, start := range candidates {
		if root, ok := findRepoRoot(start); ok {
			return root, nil
		}
	}

	return "", errors.New("source_root not configured and auto-detect failed (expected go.mod + cmd/ai-flow)")
}

func findRepoRoot(start string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		modPath := filepath.Join(dir, "go.mod")
		cmdPath := filepath.Join(dir, "cmd", "ai-flow")
		if fileExists(modPath) && directoryExists(cmdPath) {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// --- self_preflight_status ---

type SelfPreflightStatusInput struct{}

func selfPreflightStatusHandler(opts Options) func(context.Context, *mcp.CallToolRequest, SelfPreflightStatusInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SelfPreflightStatusInput) (*mcp.CallToolResult, any, error) {
		last := gate.LastResult()
		if last == nil {
			return jsonResult(map[string]any{
				"status":  "no_preflight",
				"message": "no preflight has been run yet",
			})
		}

		// Check against current HEAD.
		currentSHA := ""
		if opts.SourceRoot != "" {
			sha, err := gitHeadSHA(ctx, opts.SourceRoot)
			if err == nil {
				currentSHA = sha
			}
		}

		canRestart, reason := gate.CanRestart(currentSHA)
		return jsonResult(map[string]any{
			"last_success":   last.Success,
			"last_commit":    last.CommitSHA,
			"last_timestamp": last.Timestamp.Format(time.RFC3339),
			"current_commit": currentSHA,
			"can_restart":    canRestart,
			"reason":         reason,
		})
	}
}

// --- self_restart (gated) ---

type SelfRestartInput struct {
	GracefulTimeoutSec int  `json:"graceful_timeout_sec,omitempty" jsonschema:"Graceful shutdown timeout in seconds"`
	Force              bool `json:"force,omitempty" jsonschema:"Bypass preflight gate (emergency only)"`
}

func selfRestartHandler(opts Options) func(context.Context, *mcp.CallToolRequest, SelfRestartInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SelfRestartInput) (*mcp.CallToolResult, any, error) {
		if opts.ServerAddr == "" {
			return errorResult("server_addr not configured")
		}

		// Preflight gate check.
		if !in.Force {
			currentSHA := ""
			if opts.SourceRoot != "" {
				sha, err := gitHeadSHA(ctx, opts.SourceRoot)
				if err == nil {
					currentSHA = sha
				}
			}
			ok, reason := gate.CanRestart(currentSHA)
			if !ok {
				return errorResult(fmt.Sprintf("restart BLOCKED: %s", reason))
			}
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
			"message": "server restart initiated (preflight passed)",
		})
	}
}
