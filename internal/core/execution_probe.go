package core

import "time"

// ExecutionProbeTriggerSource identifies how a probe was initiated.
type ExecutionProbeTriggerSource string

const (
	ExecutionProbeTriggerWatchdog ExecutionProbeTriggerSource = "watchdog"
	ExecutionProbeTriggerManual   ExecutionProbeTriggerSource = "manual"
)

// ExecutionProbeStatus tracks the runtime lifecycle of a probe.
type ExecutionProbeStatus string

const (
	ExecutionProbePending     ExecutionProbeStatus = "pending"
	ExecutionProbeSent        ExecutionProbeStatus = "sent"
	ExecutionProbeAnswered    ExecutionProbeStatus = "answered"
	ExecutionProbeTimeout     ExecutionProbeStatus = "timeout"
	ExecutionProbeUnreachable ExecutionProbeStatus = "unreachable"
	ExecutionProbeFailed      ExecutionProbeStatus = "failed"
)

// ExecutionProbeVerdict summarizes what the probe implies about a running execution.
type ExecutionProbeVerdict string

const (
	ExecutionProbeAlive   ExecutionProbeVerdict = "alive"
	ExecutionProbeBlocked ExecutionProbeVerdict = "blocked"
	ExecutionProbeHung    ExecutionProbeVerdict = "hung"
	ExecutionProbeDead    ExecutionProbeVerdict = "dead"
	ExecutionProbeUnknown ExecutionProbeVerdict = "unknown"
)

// ExecutionProbe stores a single side-channel diagnostic interaction for a running execution.
type ExecutionProbe struct {
	ID             int64                       `json:"id"`
	ExecutionID    int64                       `json:"execution_id"`
	IssueID         int64                       `json:"flow_id"`
	StepID         int64                       `json:"step_id"`
	AgentContextID *int64                      `json:"agent_context_id,omitempty"`
	SessionID      string                      `json:"-"`
	OwnerID        string                      `json:"owner_id,omitempty"`
	TriggerSource  ExecutionProbeTriggerSource `json:"trigger_source"`
	Question       string                      `json:"question"`
	Status         ExecutionProbeStatus        `json:"status"`
	Verdict        ExecutionProbeVerdict       `json:"verdict"`
	ReplyText      string                      `json:"reply_text,omitempty"`
	Error          string                      `json:"error,omitempty"`
	SentAt         *time.Time                  `json:"sent_at,omitempty"`
	AnsweredAt     *time.Time                  `json:"answered_at,omitempty"`
	CreatedAt      time.Time                   `json:"created_at"`
}

// ExecutionProbeRoute resolves the internal runtime route for probing an execution.
type ExecutionProbeRoute struct {
	ExecutionID     int64
	IssueID          int64
	StepID          int64
	AgentContextID  *int64
	SessionID       string
	OwnerID         string
	OwnerLastSeenAt *time.Time
}
