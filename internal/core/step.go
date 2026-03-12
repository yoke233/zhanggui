package core

import "time"

// StepType classifies how a Step is executed.
type StepType string

const (
	StepExec      StepType = "exec"
	StepGate      StepType = "gate"
	StepComposite StepType = "composite"
)

// StepStatus represents the lifecycle state of a Step.
type StepStatus string

const (
	StepPending     StepStatus = "pending"
	StepReady       StepStatus = "ready"
	StepRunning     StepStatus = "running"
	StepWaitingGate StepStatus = "waiting_gate"
	StepBlocked     StepStatus = "blocked"
	StepFailed      StepStatus = "failed"
	StepDone        StepStatus = "done"
	StepCancelled   StepStatus = "cancelled"
)

// Step is a single unit of work within an Issue's execution pipeline.
// Steps are ordered by Position and executed sequentially.
type Step struct {
	ID          int64      `json:"id"`
	IssueID     int64      `json:"issue_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"` // what this step should accomplish
	Type        StepType   `json:"type"`
	Status      StepStatus `json:"status"`
	Position    int        `json:"position"` // execution order within the Issue (0-based)

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
