package acpclient

import (
	"encoding/json"

	acpproto "github.com/coder/acp-go-sdk"
)

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

type PromptResult struct {
	Text       string              `json:"text,omitempty"`
	Usage      *acpproto.Usage     `json:"usage,omitempty"`
	StopReason acpproto.StopReason `json:"stopReason,omitempty"`
}

type SessionUpdate struct {
	SessionID string          `json:"sessionId"`
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Status    string          `json:"status,omitempty"`
	RawJSON   json.RawMessage `json:"raw,omitempty"`

	Commands      []acpproto.AvailableCommand          `json:"-"`
	ConfigOptions []acpproto.SessionConfigOptionSelect `json:"-"`
}
