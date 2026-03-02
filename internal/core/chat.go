package core

import (
	"crypto/rand"
	"fmt"
	"time"
)

// ChatSession stores a conversation history for one project-scoped request.
type ChatSession struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	// AgentSessionID stores provider-native session id (e.g. Claude session_id) for multi-turn resume.
	AgentSessionID string        `json:"agent_session_id,omitempty"`
	Messages       []ChatMessage `json:"messages"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// ChatMessage is one turn inside a chat session.
type ChatMessage struct {
	Role    string    `json:"role"` // "user" | "assistant"
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

// NewChatSessionID generates an ID in format: chat-YYYYMMDD-xxxxxxxx.
func NewChatSessionID() string {
	return fmt.Sprintf("chat-%s-%s", time.Now().Format("20060102"), randomHex(4))
}

func randomHex(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
