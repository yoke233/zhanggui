package acpclient

type LaunchConfig struct {
	Command string
	Args    []string
	WorkDir string
	Env     map[string]string
}

type ClientCapabilities struct {
	FSRead   bool
	FSWrite  bool
	Terminal bool
}

type NewSessionRequest struct {
	CWD        string
	MCPServers []MCPServerConfig
	Metadata   map[string]string
}

type MCPServerConfig struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

type LoadSessionRequest struct {
	SessionID string
	CWD       string
	Metadata  map[string]string
}

type SessionInfo struct {
	SessionID string `json:"sessionId"`
}

type PromptRequest struct {
	SessionID string
	Prompt    string
	Metadata  map[string]string
}

type TokenUsage struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens,omitempty"`
}

type PromptResult struct {
	RequestID  string     `json:"requestId,omitempty"`
	Text       string     `json:"text,omitempty"`
	Usage      TokenUsage `json:"usage,omitempty"`
	StopReason string     `json:"stopReason,omitempty"`
}

type CancelRequest struct {
	SessionID string
	RequestID string
}

type SessionUpdate struct {
	SessionID string `json:"sessionId"`
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Status    string `json:"status,omitempty"`
}

type NewSessionParams struct {
	CWD        string            `json:"cwd"`
	MCPServers []MCPServerConfig `json:"mcpServers,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type LoadSessionParams struct {
	SessionID string            `json:"sessionId"`
	CWD       string            `json:"cwd,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type PromptParams struct {
	SessionID string            `json:"sessionId"`
	Prompt    []PromptContent   `json:"prompt"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type PromptContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type CancelParams struct {
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId,omitempty"`
}

func (r NewSessionRequest) ToParams() NewSessionParams {
	return NewSessionParams{
		CWD:        r.CWD,
		MCPServers: r.MCPServers,
		Metadata:   r.Metadata,
	}
}

func (r LoadSessionRequest) ToParams() LoadSessionParams {
	return LoadSessionParams{
		SessionID: r.SessionID,
		CWD:       r.CWD,
		Metadata:  r.Metadata,
	}
}

func (r PromptRequest) ToParams() PromptParams {
	return PromptParams{
		SessionID: r.SessionID,
		Prompt: []PromptContent{
			{Type: "text", Text: r.Prompt},
		},
		Metadata: r.Metadata,
	}
}

func (r CancelRequest) ToParams() CancelParams {
	return CancelParams{
		SessionID: r.SessionID,
		RequestID: r.RequestID,
	}
}

type ReadFileRequest struct {
	Path string `json:"path"`
}

type ReadFileResult struct {
	Content string `json:"content"`
}

type WriteFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type WriteFileResult struct {
	BytesWritten int `json:"bytesWritten,omitempty"`
}

type PermissionRequest struct {
	Action   string            `json:"action,omitempty"`
	Reason   string            `json:"reason,omitempty"`
	Resource string            `json:"resource,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type PermissionDecision struct {
	Outcome  string `json:"outcome,omitempty"`
	OptionID string `json:"optionId,omitempty"`
}

type TerminalCreateRequest struct {
	CWD     string            `json:"cwd,omitempty"`
	Command []string          `json:"command,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type TerminalCreateResult struct {
	TerminalID string `json:"terminalId"`
}

type TerminalWriteRequest struct {
	TerminalID string `json:"terminalId"`
	Input      string `json:"input"`
}

type TerminalWriteResult struct {
	Written int `json:"written,omitempty"`
}

type TerminalReadRequest struct {
	TerminalID string `json:"terminalId"`
	MaxBytes   int    `json:"maxBytes,omitempty"`
}

type TerminalReadResult struct {
	Output string `json:"output,omitempty"`
	EOF    bool   `json:"eof,omitempty"`
}

type TerminalResizeRequest struct {
	TerminalID string `json:"terminalId"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
}

type TerminalResizeResult struct{}

type TerminalCloseRequest struct {
	TerminalID string `json:"terminalId"`
}

type TerminalCloseResult struct{}
