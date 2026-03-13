package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	acpproto "github.com/coder/acp-go-sdk"
)

type Option func(*Client)

func WithEventHandler(h EventHandler) Option {
	return func(c *Client) {
		c.eventHandler = h
	}
}

func WithTraceRecorder(r TraceRecorder) Option {
	return func(c *Client) {
		c.traceRecorder = r
	}
}

func WithCloseHook(hook func(context.Context) error) Option {
	return func(c *Client) {
		c.closeHook = hook
	}
}

type Client struct {
	cfg     LaunchConfig
	handler acpproto.Client

	eventHandler  EventHandler
	traceRecorder TraceRecorder

	cmd       *exec.Cmd
	transport *Transport

	waitCh chan error

	closeHook func(context.Context) error

	closeOnce sync.Once
	closeErr  error

	promptMu   sync.Mutex
	activeText map[string]*strings.Builder

	agentCaps acpproto.AgentCapabilities
}

func New(cfg LaunchConfig, h acpproto.Client, opts ...Option) (*Client, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, errors.New("launch command is required")
	}
	if h == nil {
		h = &NopHandler{}
	}

	cmd := exec.Command(cfg.Command, cfg.Args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	cmd.Env = mergeEnv(cfg.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start agent process: %w", err)
	}

	c := &Client{
		cfg:        cfg,
		handler:    h,
		cmd:        cmd,
		waitCh:     make(chan error, 1),
		activeText: make(map[string]*strings.Builder),
	}
	for _, opt := range opts {
		opt(c)
	}

	c.transport = NewTransport(stdin, stdout, newTraceRelay(c.traceRecorder))
	c.transport.SetRequestHandler(c.handleRequest)
	c.transport.SetNotificationHandler(c.handleNotification)

	go func() {
		stderrData, _ := io.ReadAll(stderr)
		if len(stderrData) > 0 {
			fmt.Fprintf(os.Stderr, "[acp-stderr][%s] %s\n", cfg.Command, string(stderrData))
		}
	}()
	go func() {
		c.waitCh <- cmd.Wait()
	}()

	return c, nil
}

func NewWithIO(cfg LaunchConfig, h acpproto.Client, writer io.WriteCloser, reader io.Reader, opts ...Option) (*Client, error) {
	if writer == nil || reader == nil {
		return nil, errors.New("io streams are required")
	}
	if h == nil {
		h = &NopHandler{}
	}

	c := &Client{
		cfg:        cfg,
		handler:    h,
		activeText: make(map[string]*strings.Builder),
	}
	for _, opt := range opts {
		opt(c)
	}

	c.transport = NewTransport(writer, reader, newTraceRelay(c.traceRecorder))
	c.transport.SetRequestHandler(c.handleRequest)
	c.transport.SetNotificationHandler(c.handleNotification)
	return c, nil
}

func (c *Client) Initialize(ctx context.Context, caps ClientCapabilities) error {
	params := acpproto.InitializeRequest{
		ProtocolVersion:    acpproto.ProtocolVersionNumber,
		ClientCapabilities: toACPClientCapabilities(caps),
		ClientInfo: &acpproto.Implementation{
			Name:    "ai-workflow",
			Title:   acpproto.Ptr("AI Workflow"),
			Version: "0.1.0",
		},
	}
	raw, err := c.transport.Call(ctx, "initialize", params)
	if err != nil {
		return err
	}
	var resp acpproto.InitializeResponse
	if raw != nil {
		if jsonErr := json.Unmarshal(raw, &resp); jsonErr == nil {
			c.agentCaps = resp.AgentCapabilities
		}
	}
	return nil
}

// SupportsSSEMCP reports whether the agent advertised SSE MCP capability.
func (c *Client) SupportsSSEMCP() bool {
	return c.agentCaps.McpCapabilities.Sse
}

func (c *Client) NewSession(ctx context.Context, req acpproto.NewSessionRequest) (acpproto.SessionId, error) {
	result, err := c.NewSessionResult(ctx, req)
	if err != nil {
		return "", err
	}
	return result.SessionID, nil
}

func (c *Client) NewSessionResult(ctx context.Context, req acpproto.NewSessionRequest) (SessionResult, error) {
	if req.McpServers == nil {
		req.McpServers = []acpproto.McpServer{}
	}

	raw, err := c.transport.Call(ctx, "session/new", req)
	if err != nil && len(req.Meta) > 0 && isInvalidParamsRPCError(err) {
		reqWithoutMeta := req
		reqWithoutMeta.Meta = nil
		raw, err = c.transport.Call(ctx, "session/new", reqWithoutMeta)
	}
	if err != nil {
		return SessionResult{}, err
	}

	slog.Debug("acpclient: session/new raw response", "json", string(raw))

	var modern acpproto.NewSessionResponse
	if err := json.Unmarshal(raw, &modern); err == nil {
		if trimmed := strings.TrimSpace(string(modern.SessionId)); trimmed != "" {
			return SessionResult{
				SessionID:     modern.SessionId,
				ConfigOptions: selectConfigOptions(modern.ConfigOptions),
				Modes:         modern.Modes,
			}, nil
		}
	}

	var legacy struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return SessionResult{}, fmt.Errorf("decode session/new result: %w", err)
	}
	if strings.TrimSpace(legacy.SessionID) == "" {
		return SessionResult{}, errors.New("session/new returned empty sessionId")
	}
	return SessionResult{SessionID: acpproto.SessionId(strings.TrimSpace(legacy.SessionID))}, nil
}

func (c *Client) LoadSession(ctx context.Context, req acpproto.LoadSessionRequest) (acpproto.SessionId, error) {
	result, err := c.LoadSessionResult(ctx, req)
	if err != nil {
		return "", err
	}
	return result.SessionID, nil
}

func (c *Client) LoadSessionResult(ctx context.Context, req acpproto.LoadSessionRequest) (SessionResult, error) {
	if req.McpServers == nil {
		req.McpServers = []acpproto.McpServer{}
	}

	raw, err := c.transport.Call(ctx, "session/load", req)
	if err != nil && len(req.Meta) > 0 && isInvalidParamsRPCError(err) {
		reqWithoutMeta := req
		reqWithoutMeta.Meta = nil
		raw, err = c.transport.Call(ctx, "session/load", reqWithoutMeta)
	}
	if err != nil {
		return SessionResult{}, err
	}

	slog.Debug("acpclient: session/load raw response", "json", string(raw))

	// Try modern format first so we capture configOptions / modes.
	var modern acpproto.LoadSessionResponse
	if err := json.Unmarshal(raw, &modern); err == nil {
		sid := req.SessionId
		if strings.TrimSpace(string(sid)) == "" {
			return SessionResult{}, errors.New("session/load returned empty sessionId")
		}
		return SessionResult{
			SessionID:     sid,
			ConfigOptions: selectConfigOptions(modern.ConfigOptions),
			Modes:         modern.Modes,
		}, nil
	}

	// Fallback: legacy response with only sessionId.
	var legacy struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return SessionResult{}, fmt.Errorf("decode session/load result: %w", err)
	}
	if strings.TrimSpace(legacy.SessionID) == "" {
		return SessionResult{}, errors.New("session/load returned empty sessionId")
	}
	return SessionResult{SessionID: acpproto.SessionId(strings.TrimSpace(legacy.SessionID))}, nil
}

func (c *Client) SetConfigOption(ctx context.Context, req acpproto.SetSessionConfigOptionRequest) ([]acpproto.SessionConfigOptionSelect, error) {
	raw, err := c.transport.Call(ctx, "session/set_config_option", req)
	if err != nil {
		return nil, err
	}

	var resp acpproto.SetSessionConfigOptionResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode session/set_config_option result: %w", err)
	}
	return selectConfigOptions(resp.ConfigOptions), nil
}

func (c *Client) SetSessionMode(ctx context.Context, req acpproto.SetSessionModeRequest) error {
	_, err := c.transport.Call(ctx, "session/set_mode", req)
	return err
}

func (c *Client) Prompt(ctx context.Context, req acpproto.PromptRequest) (*PromptResult, error) {
	if strings.TrimSpace(string(req.SessionId)) == "" {
		return nil, errors.New("session id is required")
	}
	sessionID := string(req.SessionId)
	c.startCollect(sessionID)
	defer c.stopCollect(sessionID)

	raw, err := c.transport.Call(ctx, "session/prompt", req)
	if err != nil && len(req.Meta) > 0 && isInvalidParamsRPCError(err) {
		reqWithoutMeta := req
		reqWithoutMeta.Meta = nil
		raw, err = c.transport.Call(ctx, "session/prompt", reqWithoutMeta)
	}
	if err != nil {
		return nil, err
	}

	var modern acpproto.PromptResponse
	if err := json.Unmarshal(raw, &modern); err == nil && modern.StopReason != "" {
		text := c.collectedText(sessionID)
		usage := modern.Usage
		var compat struct {
			Text  string          `json:"text"`
			Usage *acpproto.Usage `json:"usage"`
		}
		if err := json.Unmarshal(raw, &compat); err == nil {
			if text == "" {
				text = compat.Text
			}
			if usage == nil {
				usage = compat.Usage
			}
		}
		return &PromptResult{
			Text:       text,
			Usage:      usage,
			StopReason: modern.StopReason,
		}, nil
	}

	var legacy struct {
		StopReason string          `json:"stopReason"`
		Usage      *acpproto.Usage `json:"usage"`
		Text       string          `json:"text"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("decode session/prompt result: %w", err)
	}

	text := c.collectedText(sessionID)
	if text == "" {
		text = legacy.Text
	}

	return &PromptResult{
		Text:       text,
		Usage:      legacy.Usage,
		StopReason: acpproto.StopReason(strings.TrimSpace(legacy.StopReason)),
	}, nil
}

func (c *Client) Cancel(_ context.Context, req acpproto.CancelNotification) error {
	return c.transport.Notify("session/cancel", req)
}

func (c *Client) Close(ctx context.Context) error {
	c.closeOnce.Do(func() {
		if c.transport != nil {
			_ = c.transport.Close()
		}

		if c.cmd != nil && c.cmd.Process != nil {
			select {
			case err := <-c.waitCh:
				if err != nil && !isProcessExit(err) {
					c.closeErr = err
				}
			default:
				_ = c.cmd.Process.Kill()
				select {
				case err := <-c.waitCh:
					if err != nil && !isProcessExit(err) {
						c.closeErr = err
					}
				case <-ctx.Done():
					c.closeErr = ctx.Err()
				}
			}
		}

		if c.closeHook != nil {
			if err := c.closeHook(ctx); err != nil && c.closeErr == nil {
				c.closeErr = err
			}
		}
	})
	return c.closeErr
}

func (c *Client) handleRequest(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "fs/read_file", "fs/read_text_file":
		var req acpproto.ReadTextFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.ReadTextFile(ctx, req)
	case "fs/write_file", "fs/write_text_file":
		var req acpproto.WriteTextFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.WriteTextFile(ctx, req)
	case "session/request_permission", "request_permission":
		var req acpproto.RequestPermissionRequest
		if len(params) > 0 {
			_ = json.Unmarshal(params, &req)
		}
		resp, err := c.handler.RequestPermission(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	case "terminal/create":
		req, err := decodeTerminalCreateRequest(params)
		if err != nil {
			return nil, err
		}
		return c.handler.CreateTerminal(ctx, req)
	case "terminal/kill":
		var req acpproto.KillTerminalCommandRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.KillTerminalCommand(ctx, req)
	case "terminal/output":
		var req acpproto.TerminalOutputRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.TerminalOutput(ctx, req)
	case "terminal/release":
		var req acpproto.ReleaseTerminalRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.ReleaseTerminal(ctx, req)
	case "terminal/wait_for_exit":
		var req acpproto.WaitForTerminalExitRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.WaitForTerminalExit(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func (c *Client) handleNotification(ctx context.Context, method string, params json.RawMessage) {
	if method != "session/update" {
		return
	}

	if update, ok := decodeACPNotification(params); ok {
		if c.eventHandler != nil {
			_ = c.eventHandler.HandleSessionUpdate(ctx, update)
		}
		if update.Type == "agent_message_chunk" && update.Text != "" {
			c.appendText(update.SessionID, update.Text)
		}
		return
	}

	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type payload struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"update"`
	}
	type updatePayload struct {
		SessionUpdate string          `json:"sessionUpdate"`
		Content       json.RawMessage `json:"content"`
		Status        string          `json:"status"`
	}

	var in payload
	if err := json.Unmarshal(params, &in); err != nil {
		return
	}
	if len(in.Update) == 0 {
		return
	}

	var updatePayloadData updatePayload
	if err := json.Unmarshal(in.Update, &updatePayloadData); err != nil {
		return
	}

	var text string
	if len(updatePayloadData.Content) > 0 {
		var cblock contentBlock
		if err := json.Unmarshal(updatePayloadData.Content, &cblock); err == nil && cblock.Type == "text" {
			text = cblock.Text
		}
	}

	update := SessionUpdate{
		SessionID: in.SessionID,
		Type:      updatePayloadData.SessionUpdate,
		Text:      text,
		Status:    updatePayloadData.Status,
		RawJSON:   json.RawMessage(in.Update),
	}
	if c.eventHandler != nil {
		_ = c.eventHandler.HandleSessionUpdate(ctx, update)
	}

	if update.Type == "agent_message_chunk" && update.Text != "" {
		c.appendText(in.SessionID, update.Text)
	}
}

func (c *Client) startCollect(sessionID string) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	c.activeText[sessionID] = &strings.Builder{}
}

func (c *Client) appendText(sessionID string, text string) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	sb, ok := c.activeText[sessionID]
	if !ok {
		return
	}
	sb.WriteString(text)
}

func (c *Client) collectedText(sessionID string) string {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	sb, ok := c.activeText[sessionID]
	if !ok {
		return ""
	}
	return sb.String()
}

func (c *Client) stopCollect(sessionID string) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	delete(c.activeText, sessionID)
}

func toACPClientCapabilities(caps ClientCapabilities) acpproto.ClientCapabilities {
	return acpproto.ClientCapabilities{
		Fs: acpproto.FileSystemCapability{
			ReadTextFile:  caps.FSRead,
			WriteTextFile: caps.FSWrite,
		},
		Terminal: caps.Terminal,
	}
}

func mergeEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func isInvalidParamsRPCError(err error) bool {
	var rpcErr *rpcError
	if !errors.As(err, &rpcErr) {
		return false
	}
	return rpcErr != nil && rpcErr.Code == -32602
}

func isProcessExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func decodeACPNotification(params json.RawMessage) (SessionUpdate, bool) {
	var notification acpproto.SessionNotification
	if err := json.Unmarshal(params, &notification); err != nil {
		return SessionUpdate{}, false
	}
	return decodeACPNotificationFromStruct(notification)
}

func decodeACPNotificationFromStruct(notification acpproto.SessionNotification) (SessionUpdate, bool) {
	sessionID := strings.TrimSpace(string(notification.SessionId))
	if sessionID == "" {
		return SessionUpdate{}, false
	}

	updateType := ""
	text := ""
	status := ""
	commands := []acpproto.AvailableCommand(nil)
	configOptions := []acpproto.SessionConfigOptionSelect(nil)
	switch {
	case notification.Update.AgentMessageChunk != nil:
		updateType = "agent_message_chunk"
		if tb := notification.Update.AgentMessageChunk.Content.Text; tb != nil {
			text = tb.Text
		}
	case notification.Update.AgentThoughtChunk != nil:
		updateType = "agent_thought_chunk"
		if tb := notification.Update.AgentThoughtChunk.Content.Text; tb != nil {
			text = tb.Text
		}
	case notification.Update.UserMessageChunk != nil:
		updateType = "user_message_chunk"
		if tb := notification.Update.UserMessageChunk.Content.Text; tb != nil {
			text = tb.Text
		}
	case notification.Update.ToolCall != nil:
		updateType = "tool_call"
		status = string(notification.Update.ToolCall.Status)
	case notification.Update.ToolCallUpdate != nil:
		updateType = "tool_call_update"
		if notification.Update.ToolCallUpdate.Status != nil {
			status = string(*notification.Update.ToolCallUpdate.Status)
		}
	case notification.Update.Plan != nil:
		updateType = "plan"
	case notification.Update.AvailableCommandsUpdate != nil:
		updateType = "available_commands_update"
		commands = make([]acpproto.AvailableCommand, len(notification.Update.AvailableCommandsUpdate.AvailableCommands))
		copy(commands, notification.Update.AvailableCommandsUpdate.AvailableCommands)
	case notification.Update.CurrentModeUpdate != nil:
		updateType = "current_mode_update"
	case notification.Update.ConfigOptionUpdate != nil:
		updateType = "config_option_update"
		configOptions = selectConfigOptions(notification.Update.ConfigOptionUpdate.ConfigOptions)
	case notification.Update.SessionInfoUpdate != nil:
		updateType = "session_info_update"
	case notification.Update.UsageUpdate != nil:
		updateType = "usage_update"
	default:
		return SessionUpdate{}, false
	}

	rawUpdate, err := json.Marshal(notification.Update)
	if err != nil {
		return SessionUpdate{}, false
	}
	var currentModeId string
	if notification.Update.CurrentModeUpdate != nil {
		currentModeId = string(notification.Update.CurrentModeUpdate.CurrentModeId)
	}

	return SessionUpdate{
		SessionID:     sessionID,
		Type:          updateType,
		Text:          text,
		Status:        status,
		RawJSON:       rawUpdate,
		Commands:      commands,
		ConfigOptions: configOptions,
		CurrentModeId: currentModeId,
	}, true
}

func selectConfigOptions(options []acpproto.SessionConfigOption) []acpproto.SessionConfigOptionSelect {
	if options == nil {
		return nil
	}
	result := make([]acpproto.SessionConfigOptionSelect, 0, len(options))
	for _, option := range options {
		if option.Select != nil {
			result = append(result, *option.Select)
		}
	}
	return result
}

func decodeTerminalCreateRequest(params json.RawMessage) (acpproto.CreateTerminalRequest, error) {
	type rawTerminalCreateRequest struct {
		SessionID string            `json:"sessionId,omitempty"`
		CWD       string            `json:"cwd,omitempty"`
		Command   json.RawMessage   `json:"command,omitempty"`
		Args      []string          `json:"args,omitempty"`
		Env       map[string]string `json:"env,omitempty"`
	}

	var raw rawTerminalCreateRequest
	if err := json.Unmarshal(params, &raw); err != nil {
		return acpproto.CreateTerminalRequest{}, err
	}

	var commandParts []string
	if len(raw.Command) > 0 {
		if err := json.Unmarshal(raw.Command, &commandParts); err != nil {
			var commandName string
			if err := json.Unmarshal(raw.Command, &commandName); err != nil {
				return acpproto.CreateTerminalRequest{}, errors.New("terminal/create command must be string or string[]")
			}
			if trimmed := strings.TrimSpace(commandName); trimmed != "" {
				commandParts = append(commandParts, trimmed)
			}
		}
	}
	if len(raw.Args) > 0 {
		commandParts = append(commandParts, raw.Args...)
	}
	if len(commandParts) == 0 {
		return acpproto.CreateTerminalRequest{}, errors.New("terminal/create command is required")
	}
	command := strings.TrimSpace(commandParts[0])
	if command == "" {
		return acpproto.CreateTerminalRequest{}, errors.New("terminal/create command is required")
	}

	req := acpproto.CreateTerminalRequest{
		SessionId: acpproto.SessionId(strings.TrimSpace(raw.SessionID)),
		Command:   command,
	}
	if len(commandParts) > 1 {
		req.Args = append(req.Args, commandParts[1:]...)
	}
	if cwd := strings.TrimSpace(raw.CWD); cwd != "" {
		req.Cwd = &cwd
	}

	if len(raw.Env) > 0 {
		keys := make([]string, 0, len(raw.Env))
		for k := range raw.Env {
			if strings.TrimSpace(k) != "" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		req.Env = make([]acpproto.EnvVariable, 0, len(keys))
		for _, key := range keys {
			req.Env = append(req.Env, acpproto.EnvVariable{
				Name:  key,
				Value: raw.Env[key],
			})
		}
	}

	return req, nil
}
