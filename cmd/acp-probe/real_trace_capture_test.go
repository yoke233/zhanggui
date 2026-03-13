//go:build probe

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

type rawTraceRecorder struct {
	mu      sync.Mutex
	records []acpclient.JSONTraceRecord
}

func (r *rawTraceRecorder) RecordJSONTrace(record acpclient.JSONTraceRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
}

func (r *rawTraceRecorder) Snapshot() []acpclient.JSONTraceRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]acpclient.JSONTraceRecord, len(r.records))
	copy(out, r.records)
	return out
}

type realTraceLaunch struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	WorkDir string   `json:"work_dir"`
	EnvKeys []string `json:"env_keys,omitempty"`
}

type realTraceCapture struct {
	CapturedAt     string                      `json:"captured_at"`
	Agent          string                      `json:"agent"`
	Status         string                      `json:"status"`
	Error          string                      `json:"error,omitempty"`
	Launch         realTraceLaunch             `json:"launch"`
	Prompt         string                      `json:"prompt"`
	SupportsSSEMCP bool                        `json:"supports_sse_mcp"`
	SessionID      string                      `json:"session_id,omitempty"`
	Result         *FixturePromptResult        `json:"result,omitempty"`
	Trace          []acpclient.JSONTraceRecord `json:"trace"`
	Events         []FixtureEvent              `json:"events,omitempty"`
}

func claudeLaunchConfig(workDir string) acpclient.LaunchConfig {
	return acpclient.LaunchConfig{
		Command: "npx",
		Args:    []string{"-y", "@zed-industries/claude-agent-acp"},
		WorkDir: workDir,
		Env: map[string]string{
			"CLAUDECODE": "",
		},
	}
}

func TestCaptureRealACPJSONTranscripts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real ACP transcript capture in short mode")
	}

	repoRoot := repoRootFromCaller(t)
	outDir := filepath.Join(repoRoot, "docs", "reports", "artifacts", "acp-real-traces")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}

	cases := []struct {
		agent string
		build func(string) acpclient.LaunchConfig
	}{
		{agent: "codex-acp", build: codexLaunchConfig},
		{agent: "claude-acp", build: claudeLaunchConfig},
	}

	prompt := "Reply with exactly: ACP_REAL_TRACE_OK"
	captures := make([]realTraceCapture, 0, len(cases))
	failures := make([]string, 0)

	for _, tc := range cases {
		tc := tc
		t.Run(tc.agent, func(t *testing.T) {
			capture := captureRealTrace(t, tc.agent, tc.build, prompt)
			captures = append(captures, capture)

			outPath := filepath.Join(outDir, tc.agent+".json")
			writeJSONFile(t, outPath, capture)
			t.Logf("wrote trace artifact: %s", outPath)

			if capture.Status != "passed" {
				failures = append(failures, fmt.Sprintf("%s: %s", tc.agent, capture.Error))
			}
		})
	}

	sort.Slice(captures, func(i, j int) bool {
		return captures[i].Agent < captures[j].Agent
	})

	reportPath := filepath.Join(repoRoot, "docs", "reports", "acp-real-traces.md")
	writeMarkdownReport(t, reportPath, captures)
	t.Logf("wrote markdown report: %s", reportPath)

	if len(failures) > 0 {
		t.Fatalf("real ACP capture failed: %s", strings.Join(failures, "; "))
	}
}

func captureRealTrace(
	t *testing.T,
	agent string,
	buildConfig func(string) acpclient.LaunchConfig,
	prompt string,
) (capture realTraceCapture) {
	t.Helper()

	workDir, err := os.MkdirTemp("", "acp-real-trace-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(workDir)

	cfg := buildConfig(workDir)
	capture = realTraceCapture{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Agent:      agent,
		Status:     "failed",
		Launch: realTraceLaunch{
			Command: cfg.Command,
			Args:    append([]string(nil), cfg.Args...),
			WorkDir: cfg.WorkDir,
			EnvKeys: sortedKeys(cfg.Env),
		},
		Prompt: prompt,
	}

	traceRecorder := &rawTraceRecorder{}
	eventRecorder := newCaptureRecorder()
	handler := &acpclient.NopHandler{}

	client, err := acpclient.New(
		cfg,
		handler,
		acpclient.WithEventHandler(eventRecorder),
		acpclient.WithTraceRecorder(traceRecorder),
	)
	if err != nil {
		capture.Error = fmt.Sprintf("create client: %v", err)
		capture.Trace = traceRecorder.Snapshot()
		capture.Events = eventRecorder.Snapshot()
		return capture
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := client.Close(ctx); closeErr != nil && capture.Error == "" {
			capture.Error = fmt.Sprintf("close client: %v", closeErr)
		}
		capture.Trace = traceRecorder.Snapshot()
		capture.Events = eventRecorder.Snapshot()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if err := client.Initialize(ctx, acpclient.ClientCapabilities{
		FSRead:   true,
		FSWrite:  true,
		Terminal: true,
	}); err != nil {
		capture.Error = fmt.Sprintf("initialize: %v", err)
		return
	}
	capture.SupportsSSEMCP = client.SupportsSSEMCP()

	sessionID, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        workDir,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		capture.Error = fmt.Sprintf("new session: %v", err)
		return
	}
	capture.SessionID = string(sessionID)

	result, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sessionID,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: prompt}},
		},
	})
	if err != nil {
		capture.Error = fmt.Sprintf("prompt: %v", err)
		return
	}

	if result != nil {
		capture.Result = &FixturePromptResult{
			Text:       result.Text,
			StopReason: string(result.StopReason),
		}
	}
	capture.Status = "passed"
	return
}

func writeJSONFile(t *testing.T, path string, payload any) {
	t.Helper()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal json %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write json %s: %v", path, err)
	}
}

func writeMarkdownReport(t *testing.T, path string, captures []realTraceCapture) {
	t.Helper()

	var sb strings.Builder
	sb.WriteString("# Real ACP JSON Traces\n\n")
	sb.WriteString("这份文档记录真实 ACP 代理在一次最小会话中的完整 JSON-RPC 收发留档。\n\n")
	sb.WriteString("重跑命令：\n\n")
	sb.WriteString("```powershell\n")
	sb.WriteString("go test -tags probe ./cmd/acp-probe -run TestCaptureRealACPJSONTranscripts -v -timeout 600s\n")
	sb.WriteString("```\n\n")
	sb.WriteString("说明：\n\n")
	sb.WriteString("- `send` 表示 ai-workflow 发给 ACP agent 的 JSON。\n")
	sb.WriteString("- `recv` 表示 ACP agent 回给 ai-workflow 的 JSON，包含 response、notification，以及 agent 反向发起的 request。\n")
	sb.WriteString("- 完整原文保存在 `docs/reports/artifacts/acp-real-traces/*.json`。\n\n")
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Agent | Status | Session | Trace Count | Event Count | Artifact |\n")
	sb.WriteString("| --- | --- | --- | ---: | ---: | --- |\n")
	for _, capture := range captures {
		artifact := fmt.Sprintf("[`%s.json`](./artifacts/acp-real-traces/%s.json)", capture.Agent, capture.Agent)
		sb.WriteString(fmt.Sprintf(
			"| `%s` | `%s` | `%s` | %d | %d | %s |\n",
			capture.Agent,
			capture.Status,
			blankAsDash(capture.SessionID),
			len(capture.Trace),
			len(capture.Events),
			artifact,
		))
	}
	sb.WriteString("\n")

	for _, capture := range captures {
		sb.WriteString(fmt.Sprintf("## %s\n\n", capture.Agent))
		sb.WriteString(fmt.Sprintf("- Captured At: `%s`\n", capture.CapturedAt))
		sb.WriteString(fmt.Sprintf("- Status: `%s`\n", capture.Status))
		sb.WriteString(fmt.Sprintf("- Launch: `%s %s`\n", capture.Launch.Command, strings.Join(capture.Launch.Args, " ")))
		if len(capture.Launch.EnvKeys) > 0 {
			sb.WriteString(fmt.Sprintf("- Env Keys: `%s`\n", strings.Join(capture.Launch.EnvKeys, "`, `")))
		}
		sb.WriteString(fmt.Sprintf("- Supports SSE MCP: `%t`\n", capture.SupportsSSEMCP))
		sb.WriteString(fmt.Sprintf("- Prompt: `%s`\n", capture.Prompt))
		if capture.Result != nil {
			sb.WriteString(fmt.Sprintf("- Stop Reason: `%s`\n", capture.Result.StopReason))
			sb.WriteString(fmt.Sprintf("- Result Text: `%s`\n", strings.TrimSpace(capture.Result.Text)))
		}
		if capture.Error != "" {
			sb.WriteString(fmt.Sprintf("- Error: `%s`\n", capture.Error))
		}
		sb.WriteString("\n")

		sb.WriteString("### Trace Flow\n\n")
		for _, line := range summarizeTrace(capture.Trace) {
			sb.WriteString("- " + line + "\n")
		}
		sb.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write markdown report %s: %v", path, err)
	}
}

func summarizeTrace(records []acpclient.JSONTraceRecord) []string {
	if len(records) == 0 {
		return []string{"没有捕获到 JSON trace。"}
	}
	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, fmt.Sprintf(
			"`#%d` `%s` `%s` `%s`",
			record.Sequence,
			record.Direction,
			record.Timestamp,
			traceLabel(record.JSON),
		))
	}
	return lines
}

func traceLabel(raw json.RawMessage) string {
	var payload struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "invalid-json"
	}
	if payload.Method != "" && len(payload.ID) == 0 {
		return "notification " + payload.Method
	}
	if payload.Method != "" {
		return "request " + payload.Method + " id=" + strings.TrimSpace(string(payload.ID))
	}
	if len(payload.ID) > 0 {
		return "response id=" + strings.TrimSpace(string(payload.ID))
	}
	return "unknown-envelope"
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func blankAsDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func repoRootFromCaller(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
