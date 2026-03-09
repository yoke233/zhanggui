package core

import (
	"crypto/sha256"
	"fmt"
	"time"
)

const (
	DecisionTypeReview    = "review"
	DecisionTypeDecompose = "decompose"
	DecisionTypeStage     = "stage"
	DecisionTypeChat      = "chat"
	DecisionTypeGateCheck = "gate_check"
)

type Decision struct {
	ID              string    `json:"id"`
	IssueID         string    `json:"issue_id"`
	RunID           string    `json:"run_id,omitempty"`
	StageID         StageID   `json:"stage_id,omitempty"`
	AgentID         string    `json:"agent_id"`
	Type            string    `json:"type"`
	PromptHash      string    `json:"prompt_hash"`
	PromptPreview   string    `json:"prompt_preview"`
	Model           string    `json:"model"`
	Template        string    `json:"template"`
	TemplateVersion string    `json:"template_version"`
	InputTokens     int       `json:"input_tokens"`
	Action          string    `json:"action"`
	Reasoning       string    `json:"reasoning"`
	Confidence      float64   `json:"confidence"`
	OutputTokens    int       `json:"output_tokens"`
	OutputData      string    `json:"output_data"`
	DurationMs      int64     `json:"duration_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

func NewDecisionID() string {
	return fmt.Sprintf("dec-%s-%s", time.Now().Format("20060102-150405"), randomHex(4))
}

func PromptHash(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", h[:8])
}

func TruncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
