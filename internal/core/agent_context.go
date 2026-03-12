package core

import "time"

// AgentContext tracks conversational state for an agent within an Issue.
type AgentContext struct {
	ID               int64      `json:"id"`
	AgentID          string     `json:"agent_id"`
	IssueID          int64      `json:"issue_id"`
	SystemPrompt     string     `json:"system_prompt,omitempty"`
	SessionID        string     `json:"-"` // ACP session is internal routing state.
	Summary          string     `json:"summary,omitempty"`
	TurnCount        int        `json:"turn_count"`
	WorkerID         string     `json:"owner_id,omitempty"`
	WorkerLastSeenAt *time.Time `json:"owner_last_seen_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
