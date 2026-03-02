package web

import (
	"context"
	"errors"
	"strings"
)

// ChatAssistantRequest contains one user turn for model completion.
type ChatAssistantRequest struct {
	Message        string
	Role           string
	WorkDir        string
	AgentSessionID string
}

// ChatAssistantResponse contains assistant content and provider session identity.
type ChatAssistantResponse struct {
	Reply          string
	AgentSessionID string
}

// ChatAssistant provides multi-turn chat completion for /chat APIs.
type ChatAssistant interface {
	Reply(ctx context.Context, req ChatAssistantRequest) (ChatAssistantResponse, error)
}

// ClaudeChatAssistant starts ACP sessions through role-driven resolver and returns one reply turn.
type ClaudeChatAssistant struct {
	assistant *ACPChatAssistant
}

// NewClaudeChatAssistant creates a ChatAssistant backed by ACP client launch.
func NewClaudeChatAssistant(binary string) ChatAssistant {
	trimmedBinary := strings.TrimSpace(binary)
	if trimmedBinary == "" {
		trimmedBinary = "claude"
	}
	return newClaudeChatAssistantForTest(trimmedBinary, ACPChatAssistantDeps{})
}

func newClaudeChatAssistantForTest(binary string, deps ACPChatAssistantDeps) *ClaudeChatAssistant {
	trimmedBinary := strings.TrimSpace(binary)
	if trimmedBinary == "" {
		trimmedBinary = "claude"
	}
	if deps.RoleResolver == nil {
		deps.RoleResolver = newLegacyProviderRoleResolver("claude", trimmedBinary, nil, nil)
	}
	return &ClaudeChatAssistant{
		assistant: newACPChatAssistant(deps),
	}
}

func (a *ClaudeChatAssistant) Reply(ctx context.Context, req ChatAssistantRequest) (ChatAssistantResponse, error) {
	if a == nil || a.assistant == nil {
		return ChatAssistantResponse{}, errors.New("chat assistant is nil")
	}
	return a.assistant.Reply(ctx, req)
}
