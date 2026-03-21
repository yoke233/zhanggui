package core

import (
	"fmt"
	"strings"
	"time"
)

// ActionType classifies how an Action is executed.
type ActionType string

const (
	ActionExec      ActionType = "exec"
	ActionGate      ActionType = "gate"
	ActionPlan      ActionType = "plan"
	ActionComposite ActionType = "composite"
)

// ActionStatus represents the lifecycle state of an Action.
type ActionStatus string

const (
	ActionPending     ActionStatus = "pending"
	ActionReady       ActionStatus = "ready"
	ActionRunning     ActionStatus = "running"
	ActionWaitingGate ActionStatus = "waiting_gate"
	ActionBlocked     ActionStatus = "blocked"
	ActionFailed      ActionStatus = "failed"
	ActionDone        ActionStatus = "done"
	ActionCancelled   ActionStatus = "cancelled"
)

func (t ActionType) Valid() bool {
	switch t {
	case ActionExec, ActionGate, ActionPlan, ActionComposite:
		return true
	default:
		return false
	}
}

func ParseActionType(raw string) (ActionType, error) {
	t := ActionType(strings.TrimSpace(raw))
	if !t.Valid() {
		return "", fmt.Errorf("invalid action type %q", raw)
	}
	return t, nil
}

func (s ActionStatus) Valid() bool {
	switch s {
	case ActionPending, ActionReady, ActionRunning, ActionWaitingGate, ActionBlocked, ActionFailed, ActionDone, ActionCancelled:
		return true
	default:
		return false
	}
}

func ParseActionStatus(raw string) (ActionStatus, error) {
	s := ActionStatus(strings.TrimSpace(raw))
	if !s.Valid() {
		return "", fmt.Errorf("invalid action status %q", raw)
	}
	return s, nil
}

// Action is a single unit of work within a WorkItem's execution pipeline.
// Actions form a DAG via DependsOn and can be executed in parallel when independent.
type Action struct {
	ID          int64        `json:"id"`
	WorkItemID  int64        `json:"work_item_id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"` // what this action should accomplish
	Type        ActionType   `json:"type"`
	Status      ActionStatus `json:"status"`
	Position    int          `json:"position"` // legacy ordering hint (0-based)

	// DAG dependencies: action IDs that must complete before this action can start
	DependsOn []int64 `json:"depends_on,omitempty"`

	// Input is the assembled task briefing for the agent (replaces Briefing entity)
	Input string `json:"input,omitempty"`

	// Agent binding
	AgentRole            string   `json:"agent_role,omitempty"`            // lead | worker | gate | support
	RequiredCapabilities []string `json:"required_capabilities,omitempty"` // capability tags for agent matching
	AcceptanceCriteria   []string `json:"acceptance_criteria,omitempty"`   // what "done" looks like (gate evaluation)

	// Execution constraints
	Timeout    time.Duration  `json:"timeout,omitempty"`
	MaxRetries int            `json:"max_retries"`
	RetryCount int            `json:"retry_count"`
	Config     map[string]any `json:"config,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
