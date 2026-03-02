package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Option func(*Client)

func WithEventHandler(h EventHandler) Option {
	return func(c *Client) {
		c.eventHandler = h
	}
}

type Client struct {
	cfg     LaunchConfig
	handler Handler

	eventHandler EventHandler

	cmd       *exec.Cmd
	transport *Transport

	waitCh chan error

	closeOnce sync.Once
	closeErr  error

	promptMu   sync.Mutex
	activeText map[string]*strings.Builder
}

func New(cfg LaunchConfig, h Handler, opts ...Option) (*Client, error) {
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

	c.transport = NewTransport(stdin, stdout)
	c.transport.SetRequestHandler(c.handleRequest)
	c.transport.SetNotificationHandler(c.handleNotification)

	go func() {
		_, _ = io.Copy(io.Discard, stderr)
	}()
	go func() {
		c.waitCh <- cmd.Wait()
	}()

	return c, nil
}

func (c *Client) Initialize(ctx context.Context, caps ClientCapabilities) error {
	params := map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"fs": map[string]any{
				"readTextFile":  caps.FSRead,
				"writeTextFile": caps.FSWrite,
			},
			"terminal": caps.Terminal,
		},
		"clientInfo": map[string]any{
			"name":    "ai-workflow",
			"title":   "AI Workflow",
			"version": "0.1.0",
		},
	}
	_, err := c.transport.Call(ctx, "initialize", params)
	return err
}

func (c *Client) NewSession(ctx context.Context, req NewSessionRequest) (SessionInfo, error) {
	raw, err := c.transport.Call(ctx, "session/new", req.ToParams())
	if err != nil {
		return SessionInfo{}, err
	}
	var out SessionInfo
	if err := json.Unmarshal(raw, &out); err != nil {
		return SessionInfo{}, fmt.Errorf("decode session/new result: %w", err)
	}
	if out.SessionID == "" {
		return SessionInfo{}, errors.New("session/new returned empty sessionId")
	}
	return out, nil
}

func (c *Client) LoadSession(ctx context.Context, req LoadSessionRequest) (SessionInfo, error) {
	raw, err := c.transport.Call(ctx, "session/load", req.ToParams())
	if err != nil {
		return SessionInfo{}, err
	}
	var out SessionInfo
	if err := json.Unmarshal(raw, &out); err != nil {
		return SessionInfo{}, fmt.Errorf("decode session/load result: %w", err)
	}
	if out.SessionID == "" {
		return SessionInfo{}, errors.New("session/load returned empty sessionId")
	}
	return out, nil
}

func (c *Client) Prompt(ctx context.Context, req PromptRequest) (*PromptResult, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return nil, errors.New("session id is required")
	}
	c.startCollect(req.SessionID)
	defer c.stopCollect(req.SessionID)

	raw, err := c.transport.Call(ctx, "session/prompt", req.ToParams())
	if err != nil {
		return nil, err
	}

	var rpcResult struct {
		RequestID  string     `json:"requestId"`
		StopReason string     `json:"stopReason"`
		Usage      TokenUsage `json:"usage"`
		Text       string     `json:"text"`
	}
	if err := json.Unmarshal(raw, &rpcResult); err != nil {
		return nil, fmt.Errorf("decode session/prompt result: %w", err)
	}

	text := c.collectedText(req.SessionID)
	if text == "" {
		text = rpcResult.Text
	}

	return &PromptResult{
		RequestID:  rpcResult.RequestID,
		Text:       text,
		Usage:      rpcResult.Usage,
		StopReason: rpcResult.StopReason,
	}, nil
}

func (c *Client) Cancel(_ context.Context, req CancelRequest) error {
	return c.transport.Notify("session/cancel", req.ToParams())
}

func (c *Client) Close(ctx context.Context) error {
	c.closeOnce.Do(func() {
		if c.transport != nil {
			_ = c.transport.Close()
		}

		if c.cmd == nil || c.cmd.Process == nil {
			return
		}

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
	})
	return c.closeErr
}

func (c *Client) handleRequest(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "fs/read_file", "fs/read_text_file":
		var req ReadFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.HandleReadFile(ctx, req)
	case "fs/write_file", "fs/write_text_file":
		var req WriteFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.HandleWriteFile(ctx, req)
	case "session/request_permission", "request_permission":
		var req PermissionRequest
		if len(params) > 0 {
			_ = json.Unmarshal(params, &req)
		}
		return c.handler.HandleRequestPermission(ctx, req)
	case "terminal/create":
		var req TerminalCreateRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.HandleTerminalCreate(ctx, req)
	case "terminal/write":
		var req TerminalWriteRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.HandleTerminalWrite(ctx, req)
	case "terminal/read":
		var req TerminalReadRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.HandleTerminalRead(ctx, req)
	case "terminal/resize":
		var req TerminalResizeRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.HandleTerminalResize(ctx, req)
	case "terminal/close":
		var req TerminalCloseRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return c.handler.HandleTerminalClose(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func (c *Client) handleNotification(ctx context.Context, method string, params json.RawMessage) {
	if method != "session/update" {
		return
	}

	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type payload struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			SessionUpdate string          `json:"sessionUpdate"`
			Content       json.RawMessage `json:"content"`
			Status        string          `json:"status"`
		} `json:"update"`
	}

	var in payload
	if err := json.Unmarshal(params, &in); err != nil {
		return
	}

	var text string
	if len(in.Update.Content) > 0 {
		var cblock contentBlock
		if err := json.Unmarshal(in.Update.Content, &cblock); err == nil && cblock.Type == "text" {
			text = cblock.Text
		}
	}

	update := SessionUpdate{
		SessionID: in.SessionID,
		Type:      in.Update.SessionUpdate,
		Text:      text,
		Status:    in.Update.Status,
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

func mergeEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func isProcessExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}
