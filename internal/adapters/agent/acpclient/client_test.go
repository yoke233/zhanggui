package acpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
)

func TestClientLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h := &recordingHandler{}
	c, err := New(testLaunchConfig(t), h, WithEventHandler(h))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	defer func() {
		if err := c.Close(context.Background()); err != nil {
			t.Fatalf("close client failed: %v", err)
		}
	}()

	if err := c.Initialize(ctx, ClientCapabilities{FSRead: true, FSWrite: true, Terminal: true}); err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	sess, err := c.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        t.TempDir(),
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	if sess == "" {
		t.Fatal("expected non-empty session id")
	}

	got, err := c.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sess,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: "hello"}},
		},
		Meta: map[string]any{
			"role_id": "worker",
		},
	})
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil prompt result")
	}
	if !strings.Contains(got.Text, "worker") {
		t.Fatalf("expected role metadata in response text, got %q", got.Text)
	}
	if h.writeCount() == 0 {
		t.Fatal("expected write-file tool call to be routed to handler")
	}
	if h.updateCount() == 0 {
		t.Fatal("expected session/update callback to be invoked")
	}
}

func TestClientCloseIsIdempotent(t *testing.T) {
	c, err := New(testLaunchConfig(t), &NopHandler{})
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}

func TestDecodeACPNotificationExtractsChunkText(t *testing.T) {
	cases := []struct {
		name     string
		wantType string
		wantText string
		update   acpproto.SessionUpdate
	}{
		{
			name:     "agent_message_chunk",
			wantType: "agent_message_chunk",
			wantText: "hello",
			update: acpproto.SessionUpdate{
				AgentMessageChunk: &acpproto.SessionUpdateAgentMessageChunk{
					Content: acpproto.ContentBlock{Text: &acpproto.ContentBlockText{Text: "hello"}},
				},
			},
		},
		{
			name:     "agent_thought_chunk",
			wantType: "agent_thought_chunk",
			wantText: "think",
			update: acpproto.SessionUpdate{
				AgentThoughtChunk: &acpproto.SessionUpdateAgentThoughtChunk{
					Content: acpproto.ContentBlock{Text: &acpproto.ContentBlockText{Text: "think"}},
				},
			},
		},
		{
			name:     "user_message_chunk",
			wantType: "user_message_chunk",
			wantText: "user says",
			update: acpproto.SessionUpdate{
				UserMessageChunk: &acpproto.SessionUpdateUserMessageChunk{
					Content: acpproto.ContentBlock{Text: &acpproto.ContentBlockText{Text: "user says"}},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			update, ok := decodeACPNotificationFromStruct(acpproto.SessionNotification{
				SessionId: "session-1",
				Update:    tc.update,
			})
			if !ok {
				t.Fatal("expected decode to succeed")
			}
			if update.Type != tc.wantType {
				t.Fatalf("update.Type = %q, want %q", update.Type, tc.wantType)
			}
			if update.Text != tc.wantText {
				t.Fatalf("update.Text = %q, want %q", update.Text, tc.wantText)
			}
			if len(update.RawJSON) == 0 {
				t.Fatal("expected RawJSON to be populated")
			}
			var raw map[string]any
			if err := json.Unmarshal(update.RawJSON, &raw); err != nil {
				t.Fatalf("unmarshal RawJSON: %v", err)
			}
			if got := raw["sessionUpdate"]; got != tc.wantType {
				t.Fatalf("raw sessionUpdate = %v, want %q", got, tc.wantType)
			}
		})
	}
}

func TestDecodeACPNotificationExtractsAvailableCommands(t *testing.T) {
	update, ok := decodeACPNotificationFromStruct(acpproto.SessionNotification{
		SessionId: "session-commands",
		Update: acpproto.SessionUpdate{
			AvailableCommandsUpdate: &acpproto.SessionAvailableCommandsUpdate{
				SessionUpdate: "available_commands_update",
				AvailableCommands: []acpproto.AvailableCommand{
					{
						Name:        "create_plan",
						Description: "Create a plan",
						Input: &acpproto.AvailableCommandInput{
							Unstructured: &acpproto.UnstructuredCommandInput{Hint: "topic"},
						},
					},
				},
			},
		},
	})
	if !ok {
		t.Fatal("expected decode to succeed")
	}
	if update.Type != "available_commands_update" {
		t.Fatalf("update.Type = %q, want available_commands_update", update.Type)
	}
	if len(update.Commands) != 1 {
		t.Fatalf("len(update.Commands) = %d, want 1", len(update.Commands))
	}
	if update.Commands[0].Name != "create_plan" {
		t.Fatalf("command name = %q, want create_plan", update.Commands[0].Name)
	}
	if len(update.RawJSON) == 0 {
		t.Fatal("expected RawJSON to be populated")
	}
}

func TestDecodeACPNotificationExtractsConfigOptions(t *testing.T) {
	update, ok := decodeACPNotificationFromStruct(acpproto.SessionNotification{
		SessionId: "session-config",
		Update: acpproto.SessionUpdate{
			ConfigOptionUpdate: &acpproto.SessionConfigOptionUpdate{
				SessionUpdate: "config_option_update",
				ConfigOptions: []acpproto.SessionConfigOption{
					{
						Select: &acpproto.SessionConfigOptionSelect{
							Type:         "select",
							Id:           acpproto.SessionConfigId("model"),
							Name:         "Model",
							CurrentValue: acpproto.SessionConfigValueId("model-1"),
							Options: acpproto.SessionConfigSelectOptions{
								Ungrouped: &acpproto.SessionConfigSelectOptionsUngrouped{
									{
										Value: acpproto.SessionConfigValueId("model-1"),
										Name:  "Model 1",
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if !ok {
		t.Fatal("expected decode to succeed")
	}
	if update.Type != "config_option_update" {
		t.Fatalf("update.Type = %q, want config_option_update", update.Type)
	}
	if len(update.ConfigOptions) != 1 {
		t.Fatalf("len(update.ConfigOptions) = %d, want 1", len(update.ConfigOptions))
	}
	if update.ConfigOptions[0].Id != acpproto.SessionConfigId("model") {
		t.Fatalf("config option id = %q, want model", update.ConfigOptions[0].Id)
	}
	if len(update.RawJSON) == 0 {
		t.Fatal("expected RawJSON to be populated")
	}
}

func TestClientNewWithIOCloseHook(t *testing.T) {
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer serverRead.Close()
	defer clientWrite.Close()
	defer clientRead.Close()
	defer serverWrite.Close()

	closeCalls := 0
	c, err := NewWithIO(LaunchConfig{}, &NopHandler{}, clientWrite, clientRead, WithCloseHook(func(context.Context) error {
		closeCalls++
		return nil
	}))
	if err != nil {
		t.Fatalf("NewWithIO returned error: %v", err)
	}

	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("close hook calls = %d, want 1", closeCalls)
	}
}

func testLaunchConfig(t *testing.T) LaunchConfig {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	acpDir := filepath.Dir(thisFile)
	repoRoot := filepath.Clean(filepath.Join(acpDir, "..", "..", "..", ".."))
	fakeAgentPath := filepath.Join(repoRoot, "internal", "adapters", "agent", "acpclient", "testdata", "fake_agent.go")
	return LaunchConfig{
		Command: "go",
		Args:    []string{"run", fakeAgentPath},
		WorkDir: repoRoot,
	}
}

type recordingHandler struct {
	mu            sync.Mutex
	writeFileHits int
	updateHits    int
}

func (h *recordingHandler) ReadTextFile(context.Context, acpproto.ReadTextFileRequest) (acpproto.ReadTextFileResponse, error) {
	return acpproto.ReadTextFileResponse{Content: ""}, nil
}

func (h *recordingHandler) WriteTextFile(context.Context, acpproto.WriteTextFileRequest) (acpproto.WriteTextFileResponse, error) {
	h.mu.Lock()
	h.writeFileHits++
	h.mu.Unlock()
	return acpproto.WriteTextFileResponse{}, nil
}

func (h *recordingHandler) RequestPermission(context.Context, acpproto.RequestPermissionRequest) (acpproto.RequestPermissionResponse, error) {
	return acpproto.RequestPermissionResponse{
		Outcome: acpproto.RequestPermissionOutcome{
			Cancelled: &acpproto.RequestPermissionOutcomeCancelled{Outcome: "cancelled"},
		},
	}, nil
}

func (h *recordingHandler) CreateTerminal(context.Context, acpproto.CreateTerminalRequest) (acpproto.CreateTerminalResponse, error) {
	return acpproto.CreateTerminalResponse{TerminalId: "t1"}, nil
}

func (h *recordingHandler) KillTerminalCommand(context.Context, acpproto.KillTerminalCommandRequest) (acpproto.KillTerminalCommandResponse, error) {
	return acpproto.KillTerminalCommandResponse{}, nil
}

func (h *recordingHandler) TerminalOutput(context.Context, acpproto.TerminalOutputRequest) (acpproto.TerminalOutputResponse, error) {
	return acpproto.TerminalOutputResponse{}, nil
}

func (h *recordingHandler) ReleaseTerminal(context.Context, acpproto.ReleaseTerminalRequest) (acpproto.ReleaseTerminalResponse, error) {
	return acpproto.ReleaseTerminalResponse{}, nil
}

func (h *recordingHandler) WaitForTerminalExit(context.Context, acpproto.WaitForTerminalExitRequest) (acpproto.WaitForTerminalExitResponse, error) {
	return acpproto.WaitForTerminalExitResponse{}, nil
}

func (h *recordingHandler) SessionUpdate(context.Context, acpproto.SessionNotification) error {
	return nil
}

func (h *recordingHandler) HandleSessionUpdate(context.Context, SessionUpdate) error {
	h.mu.Lock()
	h.updateHits++
	h.mu.Unlock()
	return nil
}

func (h *recordingHandler) writeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.writeFileHits
}

func (h *recordingHandler) updateCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.updateHits
}

func TestClientNewSessionRetriesWithoutMetadataOnInvalidParams(t *testing.T) {
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer clientRead.Close()
	defer clientWrite.Close()
	defer serverRead.Close()
	defer serverWrite.Close()

	transport := NewTransport(clientWrite, clientRead)
	defer func() { _ = transport.Close() }()

	go func() {
		reader := bufio.NewReader(serverRead)
		first := readRPCLine(t, reader)
		if first.Method != "session/new" {
			t.Errorf("expected first method session/new, got %q", first.Method)
			return
		}
		if first.Params.Metadata["role_id"] == "" {
			t.Errorf("expected first request _meta.role_id")
			return
		}
		_ = writeLineJSON(serverWrite, map[string]any{
			"jsonrpc": "2.0",
			"id":      first.ID,
			"error": map[string]any{
				"code":    -32602,
				"message": "Invalid params",
			},
		})

		second := readRPCLine(t, reader)
		if second.Method != "session/new" {
			t.Errorf("expected second method session/new, got %q", second.Method)
			return
		}
		if len(second.Params.Metadata) != 0 {
			t.Errorf("expected second request _meta omitted, got %#v", second.Params.Metadata)
			return
		}
		_ = writeLineJSON(serverWrite, map[string]any{
			"jsonrpc": "2.0",
			"id":      second.ID,
			"result": map[string]any{
				"sessionId": "sid-new-1",
			},
		})
	}()

	client := &Client{
		transport:  transport,
		activeText: make(map[string]*strings.Builder),
	}
	sessionID, err := client.NewSession(context.Background(), acpproto.NewSessionRequest{
		Cwd:        "D:\\project\\ai-workflow",
		McpServers: []acpproto.McpServer{},
		Meta: map[string]any{
			"role_id": "TeamLeader",
		},
	})
	if err != nil {
		t.Fatalf("NewSession returned error: %v", err)
	}
	if string(sessionID) != "sid-new-1" {
		t.Fatalf("expected sid-new-1, got %q", string(sessionID))
	}
}

func TestClientPromptRetriesWithoutMetadataOnInvalidParams(t *testing.T) {
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer clientRead.Close()
	defer clientWrite.Close()
	defer serverRead.Close()
	defer serverWrite.Close()

	transport := NewTransport(clientWrite, clientRead)
	defer func() { _ = transport.Close() }()

	go func() {
		reader := bufio.NewReader(serverRead)
		first := readRPCLine(t, reader)
		if first.Method != "session/prompt" {
			t.Errorf("expected first method session/prompt, got %q", first.Method)
			return
		}
		if first.Params.Metadata["role_id"] == "" {
			t.Errorf("expected first prompt _meta.role_id")
			return
		}
		_ = writeLineJSON(serverWrite, map[string]any{
			"jsonrpc": "2.0",
			"id":      first.ID,
			"error": map[string]any{
				"code":    -32602,
				"message": "Invalid params",
			},
		})

		second := readRPCLine(t, reader)
		if second.Method != "session/prompt" {
			t.Errorf("expected second method session/prompt, got %q", second.Method)
			return
		}
		if len(second.Params.Metadata) != 0 {
			t.Errorf("expected second prompt _meta omitted, got %#v", second.Params.Metadata)
			return
		}
		_ = writeLineJSON(serverWrite, map[string]any{
			"jsonrpc": "2.0",
			"id":      second.ID,
			"result": map[string]any{
				"requestId":  "req-compat-1",
				"stopReason": "end_turn",
				"text":       "compat-ok",
			},
		})
	}()

	client := &Client{
		transport:  transport,
		activeText: make(map[string]*strings.Builder),
	}
	result, err := client.Prompt(context.Background(), acpproto.PromptRequest{
		SessionId: "sid-1",
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: "hello"}},
		},
		Meta: map[string]any{
			"role_id": "TeamLeader",
		},
	})
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil prompt result")
	}
	if result.Text != "compat-ok" {
		t.Fatalf("expected compat-ok, got %q", result.Text)
	}
}

type rpcLine struct {
	ID     any    `json:"id"`
	Method string `json:"method"`
	Params struct {
		Metadata map[string]any `json:"_meta"`
	} `json:"params"`
}

func readRPCLine(t *testing.T, reader *bufio.Reader) rpcLine {
	t.Helper()
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read rpc line: %v", err)
	}
	var msg rpcLine
	if err := json.Unmarshal(line, &msg); err != nil {
		t.Fatalf("decode rpc line: %v", err)
	}
	return msg
}

func TestHandleRequestPermissionResponsePassthrough(t *testing.T) {
	handler := &permissionAllowOnceHandler{}
	client := &Client{handler: handler}
	resp, err := client.handleRequest(context.Background(), "session/request_permission", json.RawMessage(`{
		"sessionId":"sess-1",
		"toolCall":{"toolCallId":"perm-1"},
		"options":[
			{"optionId":"allow_once","kind":"allow_once"},
			{"optionId":"reject_once","kind":"reject_once"}
		]
	}`))
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}

	out, ok := resp.(acpproto.RequestPermissionResponse)
	if !ok {
		t.Fatalf("response type = %T, want acpproto.RequestPermissionResponse", resp)
	}
	if out.Outcome.Selected == nil || string(out.Outcome.Selected.OptionId) != "allow_once" {
		t.Fatalf("unexpected permission response: %#v", out)
	}
}

func TestHandleRequestTerminalCreateAcceptsCommandStringPlusArgs(t *testing.T) {
	handler := &compatTerminalHandler{}
	client := &Client{handler: handler}

	_, err := client.handleRequest(context.Background(), "terminal/create", json.RawMessage(`{
		"sessionId":"sess-1",
		"command":"cmd",
		"args":["/C","echo hi"],
		"cwd":"D:/repo/demo"
	}`))
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}

	if got := handler.createReq.Command; got != "cmd" {
		t.Fatalf("create command = %q, want %q", got, "cmd")
	}
	if got := strings.Join(handler.createReq.Args, "\x00"); got != strings.Join([]string{"/C", "echo hi"}, "\x00") {
		t.Fatalf("create args = %v, want %v", handler.createReq.Args, []string{"/C", "echo hi"})
	}
	if handler.createReq.Cwd == nil || *handler.createReq.Cwd != "D:/repo/demo" {
		t.Fatalf("create cwd = %#v, want %q", handler.createReq.Cwd, "D:/repo/demo")
	}
}

func TestHandleRequestTerminalLifecycleDispatch(t *testing.T) {
	handler := &compatTerminalHandler{}
	client := &Client{handler: handler}

	if _, err := client.handleRequest(context.Background(), "terminal/output", json.RawMessage(`{"terminalId":"t-1"}`)); err != nil {
		t.Fatalf("terminal/output error: %v", err)
	}
	if _, err := client.handleRequest(context.Background(), "terminal/wait_for_exit", json.RawMessage(`{"terminalId":"t-1"}`)); err != nil {
		t.Fatalf("terminal/wait_for_exit error: %v", err)
	}
	if _, err := client.handleRequest(context.Background(), "terminal/kill", json.RawMessage(`{"terminalId":"t-1"}`)); err != nil {
		t.Fatalf("terminal/kill error: %v", err)
	}
	if _, err := client.handleRequest(context.Background(), "terminal/release", json.RawMessage(`{"terminalId":"t-1"}`)); err != nil {
		t.Fatalf("terminal/release error: %v", err)
	}

	if handler.outputHits != 1 || handler.waitHits != 1 || handler.killHits != 1 || handler.releaseHits != 1 {
		t.Fatalf("unexpected lifecycle hits output=%d wait=%d kill=%d release=%d", handler.outputHits, handler.waitHits, handler.killHits, handler.releaseHits)
	}
}

type permissionAllowOnceHandler struct {
	NopHandler
}

func (h *permissionAllowOnceHandler) RequestPermission(context.Context, acpproto.RequestPermissionRequest) (acpproto.RequestPermissionResponse, error) {
	return acpproto.RequestPermissionResponse{
		Outcome: acpproto.RequestPermissionOutcome{
			Selected: &acpproto.RequestPermissionOutcomeSelected{
				Outcome:  "selected",
				OptionId: "allow_once",
			},
		},
	}, nil
}

type compatTerminalHandler struct {
	NopHandler

	createReq   acpproto.CreateTerminalRequest
	outputHits  int
	waitHits    int
	killHits    int
	releaseHits int
}

func (h *compatTerminalHandler) CreateTerminal(_ context.Context, req acpproto.CreateTerminalRequest) (acpproto.CreateTerminalResponse, error) {
	h.createReq = req
	return acpproto.CreateTerminalResponse{TerminalId: "t-1"}, nil
}

func (h *compatTerminalHandler) TerminalOutput(context.Context, acpproto.TerminalOutputRequest) (acpproto.TerminalOutputResponse, error) {
	h.outputHits++
	return acpproto.TerminalOutputResponse{Output: "ok"}, nil
}

func (h *compatTerminalHandler) WaitForTerminalExit(context.Context, acpproto.WaitForTerminalExitRequest) (acpproto.WaitForTerminalExitResponse, error) {
	h.waitHits++
	exitCode := 0
	return acpproto.WaitForTerminalExitResponse{ExitCode: &exitCode}, nil
}

func (h *compatTerminalHandler) KillTerminalCommand(context.Context, acpproto.KillTerminalCommandRequest) (acpproto.KillTerminalCommandResponse, error) {
	h.killHits++
	return acpproto.KillTerminalCommandResponse{}, nil
}

func (h *compatTerminalHandler) ReleaseTerminal(context.Context, acpproto.ReleaseTerminalRequest) (acpproto.ReleaseTerminalResponse, error) {
	h.releaseHits++
	return acpproto.ReleaseTerminalResponse{}, nil
}
