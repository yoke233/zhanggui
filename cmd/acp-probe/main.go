// acp-probe: a small CLI that launches an ACP agent, sends one prompt,
// and dumps every session/update notification as raw JSON to stdout.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

type debugHandler struct {
	count    atomic.Int64
	typeStat sync.Map // type -> count
}

func (d *debugHandler) HandleSessionUpdate(_ context.Context, u acpclient.SessionUpdate) error {
	d.count.Add(1)
	n := int64(1)
	if v, ok := d.typeStat.Load(u.Type); ok {
		n = v.(int64) + 1
	}
	d.typeStat.Store(u.Type, n)

	ts := time.Now().Format("15:04:05.000")
	// Skip verbose chunk output, just count them
	// if u.Type == "agent_message_chunk" || u.Type == "agent_thought_chunk" || u.Type == "user_message_chunk" {
	// 	return nil
	// }
	raw, _ := json.MarshalIndent(u, "", "  ")
	fmt.Fprintf(os.Stdout, "\n=== [%s] #%d  type=%s ===\n%s\n", ts, d.count.Load(), u.Type, string(raw))
	return nil
}

func (d *debugHandler) PrintStats() {
	fmt.Println("\n--- Update Type Stats ---")
	d.typeStat.Range(func(key, value any) bool {
		fmt.Printf("  %-30s  %d\n", key.(string), value.(int64))
		return true
	})
}

func main() {
	agent := "codex"
	if len(os.Args) > 1 {
		agent = os.Args[1]
	}
	workDir, _ := os.MkdirTemp("", "acp-probe-*")

	var cfg acpclient.LaunchConfig
	switch agent {
	case "claude":
		cfg = acpclient.LaunchConfig{
			Command: "npx",
			Args:    []string{"-y", "@zed-industries/claude-agent-acp"},
			WorkDir: workDir,
			Env: map[string]string{
				"CLAUDECODE": "",
			},
		}
	default: // codex
		cfg = acpclient.LaunchConfig{
			Command: "npx",
			Args:    []string{"-y", "@zed-industries/codex-acp@" + codexACPVersion},
			WorkDir: workDir,
			Env:     map[string]string{
				// "CODEX_HOME": `D:\project\ai-workflow\.ai-workflow\codex-home`,
			},
		}
	}
	fmt.Printf(">>> agent: %s\n", agent)

	handler := &acpclient.NopHandler{}
	probe := &debugHandler{}

	fmt.Println(">>> launching ACP agent...")
	client, err := acpclient.New(cfg, handler, acpclient.WithEventHandler(probe))
	if err != nil {
		fmt.Fprintf(os.Stderr, "launch failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Close(ctx)
		cancel()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Println(">>> initializing...")
	if err := client.Initialize(ctx, acpclient.ClientCapabilities{
		FSRead:   true,
		FSWrite:  true,
		Terminal: true,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "initialize failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf(">>> agent supports SSE MCP: %v\n", client.SupportsSSEMCP())

	// Build MCP servers: SSE if agent supports it, otherwise stdio fallback.
	var mcpServers []acpproto.McpServer
	if client.SupportsSSEMCP() {
		fmt.Println(">>> injecting SSE MCP server")
		mcpServers = []acpproto.McpServer{{
			Sse: &acpproto.McpServerSseInline{
				Name:    "ai-workflow-query",
				Type:    "sse",
				Url:     "http://127.0.0.1:8080/api/v1/mcp",
				Headers: []acpproto.HttpHeader{},
			},
		}}
	} else {
		fmt.Println(">>> agent does NOT support SSE, injecting stdio MCP server")
		self, _ := os.Executable()
		mcpServers = []acpproto.McpServer{{
			Stdio: &acpproto.McpServerStdio{
				Name:    "ai-workflow-query",
				Command: self,
				Args:    []string{"mcp-serve"},
				Env: []acpproto.EnvVariable{
					{Name: "AI_WORKFLOW_DB_PATH", Value: os.Getenv("AI_WORKFLOW_DB_PATH")},
				},
			},
		}}
	}

	fmt.Println(">>> creating session...")
	sessionID, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        workDir,
		McpServers: mcpServers,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "new session failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf(">>> session: %s\n", sessionID)

	prompt := "在当前目录创建一个文件 hello.txt，内容写上 Hello from ACP probe test，然后读取确认内容正确。"
	fmt.Printf(">>> sending prompt: %s\n", prompt)
	result, err := client.PromptText(ctx, sessionID, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		os.Exit(1)
	}

	probe.PrintStats()
	fmt.Printf("\n>>> DONE (total updates: %d)\n", probe.count.Load())
	if result != nil {
		fmt.Printf(">>> StopReason: %s\n", result.StopReason)
		fmt.Printf(">>> Text length: %d\n", len(result.Text))
		fmt.Printf(">>> Text:\n%s\n", result.Text)
		if result.Usage != nil {
			raw, _ := json.MarshalIndent(result.Usage, "", "  ")
			fmt.Printf(">>> Usage:\n%s\n", string(raw))
		}
	}
}
